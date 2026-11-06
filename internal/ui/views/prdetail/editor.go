package prdetail

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/diff/model"
)

// editorTarget is a file and line to open, plus the commit it was read from.
type editorTarget struct {
	repoPath string
	wantSHA  string
	relPath  string
	line     int
}

type localHeadMsg struct {
	seq           uint64
	target        editorTarget
	sha           string
	branch        string
	err           error
	afterCheckout bool
}

type editorExitedMsg struct {
	err    error
	notice string
}

func (m *PRDetailModel) editorWantSHA() string {
	if m.CommitMode {
		return m.Commit.SHA
	}
	return m.Summary.HeadRefOID
}

// handleOpenInEditor starts opening the diff line under the cursor in $EDITOR.
// It first checks that the local checkout is on the commit being viewed.
func (m *PRDetailModel) handleOpenInEditor() tea.Cmd {
	if m.editorReq != 0 || m.editorLaunching || m.pendingEditor != nil || m.isAnyActionInProgress() {
		return nil
	}
	if m.Repo.LocalPath == "" {
		m.diffNotice = "Not a local repo · can't open in editor"
		return nil
	}
	t, notice, ok := m.editorTargetAtCursor()
	if !ok {
		m.diffNotice = notice
		return nil
	}
	return m.lookupLocalHead(t, false)
}

func (m *PRDetailModel) editorTargetAtCursor() (editorTarget, string, bool) {
	if m.Diff == nil || len(m.navigableLines) == 0 {
		return editorTarget{}, "No diff line to open", false
	}
	m.ensureDiffCursor()
	if !m.validDiffCursor() {
		return editorTarget{}, "No diff line to open", false
	}
	c := m.diffCursor
	if c.FileIdx < 0 || c.FileIdx >= len(m.Diff.Files) {
		return editorTarget{}, "No diff line to open", false
	}
	f := &m.Diff.Files[c.FileIdx]
	if f.Status == "removed" {
		return editorTarget{}, "File was deleted · nothing to open", false
	}
	path := f.NewPath
	if path == "" {
		path = f.OldPath
	}
	line := 1
	if !c.isPlaceholder() && c.HunkIdx >= 0 && c.HunkIdx < len(f.Hunks) {
		line = newLineNear(&f.Hunks[c.HunkIdx], c.LineIdx)
	}
	return editorTarget{
		repoPath: m.Repo.LocalPath,
		wantSHA:  m.editorWantSHA(),
		relPath:  path,
		line:     line,
	}, "", true
}

// newLineNear returns the new-side line number for lines[li]. A deleted line
// has none, so it takes the next line that has one (the line now in its
// place), else the previous one.
func newLineNear(h *model.DiffHunk, li int) int {
	for i := max(li, 0); i < len(h.Lines); i++ {
		if n := h.Lines[i].NewLine; n != nil {
			return max(*n, 1)
		}
	}
	for i := min(li, len(h.Lines)) - 1; i >= 0; i-- {
		if n := h.Lines[i].NewLine; n != nil {
			return max(*n, 1)
		}
	}
	return max(h.NewStart, 1)
}

func (m *PRDetailModel) lookupLocalHead(t editorTarget, afterCheckout bool) tea.Cmd {
	m.editorSeq++
	m.editorReq = m.editorSeq
	m.editorTarget = t
	seq := m.editorSeq
	return func() tea.Msg {
		sha, err := gitOutput(t.repoPath, "rev-parse", "HEAD")
		if err != nil {
			return localHeadMsg{seq: seq, target: t, err: err, afterCheckout: afterCheckout}
		}
		branch, _ := gitOutput(t.repoPath, "branch", "--show-current")
		return localHeadMsg{seq: seq, target: t, sha: sha, branch: branch, afterCheckout: afterCheckout}
	}
}

func (m *PRDetailModel) handleLocalHead(msg localHeadMsg) tea.Cmd {
	// A newer press, a tab switch or a reloaded head makes this result stale.
	if msg.seq != m.editorReq || msg.target != m.editorTarget ||
		msg.target.repoPath != m.Repo.LocalPath || msg.target.wantSHA != m.editorWantSHA() {
		return nil
	}
	m.editorReq = 0
	if msg.err != nil {
		m.diffNotice = "Not a git repo · can't open in editor"
		return nil
	}
	// Without a known head SHA there is nothing to compare against.
	if msg.target.wantSHA == "" || msg.sha == msg.target.wantSHA {
		return m.launchEditor(msg.target, "")
	}
	if msg.afterCheckout {
		return m.launchEditor(msg.target, "Opened, but local HEAD still differs from PR head")
	}
	if m.CommitMode {
		m.editorPrompt = "Local HEAD differs from this commit · o: open anyway · esc"
		return nil
	}
	at := msg.branch
	if at == "" {
		at = shortSHA(msg.sha)
	}
	m.editorPrompt = fmt.Sprintf("Local HEAD (%s) differs from PR head · b: checkout & open · o: open anyway · esc", at)
	return nil
}

func (m *PRDetailModel) handleEditorPromptKey(msg tea.KeyMsg) tea.Cmd {
	t := m.editorTarget
	m.editorPrompt = ""
	switch msg.String() {
	case "b":
		if m.CommitMode {
			return nil
		}
		// The prompt is cleared first: it counts as an action in progress,
		// which would make handleCheckout refuse.
		cmd := m.handleCheckout()
		if m.checkoutInFlight {
			m.pendingEditor = &t
		}
		return cmd
	case "o":
		return m.launchEditor(t, "")
	}
	return nil
}

func (m *PRDetailModel) resumeEditorAfterCheckout(ok bool) tea.Cmd {
	if m.pendingEditor == nil {
		return nil
	}
	t := *m.pendingEditor
	m.pendingEditor = nil
	if !ok {
		return nil
	}
	return m.lookupLocalHead(t, true)
}

func (m *PRDetailModel) cancelEditorLookup() {
	m.editorReq = 0
	m.editorPrompt = ""
}

func (m *PRDetailModel) launchEditor(t editorTarget, notice string) tea.Cmd {
	abs := filepath.Join(t.repoPath, t.relPath)
	if _, err := os.Stat(abs); err != nil {
		m.diffNotice = t.relPath + " not in local checkout"
		return nil
	}
	argv, err := resolveEditor(os.Getenv)
	if err != nil {
		m.diffNotice = "Can't parse " + err.Error()
		return nil
	}
	argv = editorArgs(argv, abs, t.line)
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = t.repoPath
	m.editorLaunching = true
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorExitedMsg{err: err, notice: notice}
	})
}

func (m *PRDetailModel) handleEditorExited(msg editorExitedMsg) {
	m.editorLaunching = false
	switch {
	case msg.err != nil:
		m.diffNotice = "Editor: " + msg.err.Error()
	case msg.notice != "":
		m.diffNotice = msg.notice
	}
}

// resolveEditor returns $VISUAL, else $EDITOR, else vi, split into argv.
func resolveEditor(getenv func(string) string) ([]string, error) {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		v := getenv(k)
		if strings.TrimSpace(v) == "" {
			continue
		}
		argv, err := splitShellWords(v)
		if err != nil {
			return nil, fmt.Errorf("$%s: %w", k, err)
		}
		return argv, nil
	}
	return []string{"vi"}, nil
}

// editorArgs appends path and line in the form the editor understands.
func editorArgs(argv []string, path string, line int) []string {
	line = max(line, 1)
	out := append([]string(nil), argv...)
	switch filepath.Base(argv[0]) {
	case "code", "code-insiders", "cursor", "windsurf", "codium":
		return append(out, "--goto", path+":"+strconv.Itoa(line))
	case "subl", "zed", "hx", "helix":
		return append(out, path+":"+strconv.Itoa(line))
	}
	return append(out, "+"+strconv.Itoa(line), path)
}

// splitShellWords splits s like a POSIX shell would split a simple command
// (quotes and backslashes, no expansion) so it can be run without a shell.
func splitShellWords(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\':
			if i+1 >= len(s) {
				return nil, errors.New("trailing backslash")
			}
			i++
			cur.WriteByte(s[i])
			inWord = true
		case '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, errors.New("unterminated single quote")
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
			inWord = true
		case '"':
			closed := false
			for i++; i < len(s); i++ {
				c := s[i]
				if c == '"' {
					closed = true
					break
				}
				if c == '\\' && i+1 < len(s) && strings.IndexByte("\"\\$`", s[i+1]) >= 0 {
					i++
					c = s[i]
				}
				cur.WriteByte(c)
			}
			if !closed {
				return nil, errors.New("unterminated double quote")
			}
			inWord = true
		case ' ', '\t', '\n':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(ch)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	if len(words) == 0 {
		return nil, errors.New("empty command")
	}
	return words, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}
