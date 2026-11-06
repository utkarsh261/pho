package prdetail

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
)

func TestSplitShellWords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{in: "vim", want: []string{"vim"}},
		{in: "  code   --wait ", want: []string{"code", "--wait"}},
		{in: `"/Applications/My Editor/bin/ed" -w`, want: []string{"/Applications/My Editor/bin/ed", "-w"}},
		{in: `'/opt/my ed/ed' --flag='a b'`, want: []string{"/opt/my ed/ed", "--flag=a b"}},
		{in: `/opt/my\ ed/ed`, want: []string{"/opt/my ed/ed"}},
		{in: `"say \"hi\""`, want: []string{`say "hi"`}},
		{in: `""`, want: []string{""}},
		{in: "", wantErr: true},
		{in: "   \t", wantErr: true},
		{in: `"vim`, wantErr: true},
		{in: `'vim`, wantErr: true},
		{in: `vim\`, wantErr: true},
	}
	for _, c := range cases {
		got, err := splitShellWords(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("splitShellWords(%q) = %q, want error", c.in, got)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitShellWords(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestResolveEditor(t *testing.T) {
	t.Parallel()
	env := func(vals map[string]string) func(string) string {
		return func(k string) string { return vals[k] }
	}
	cases := []struct {
		name    string
		env     map[string]string
		want    []string
		wantErr bool
	}{
		{"visual wins", map[string]string{"VISUAL": "nvim", "EDITOR": "nano"}, []string{"nvim"}, false},
		{"editor", map[string]string{"EDITOR": "code -w"}, []string{"code", "-w"}, false},
		{"blank visual falls through", map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, []string{"nano"}, false},
		{"unset", nil, []string{"vi"}, false},
		{"bad quoting", map[string]string{"EDITOR": `"vim`}, nil, true},
	}
	for _, c := range cases {
		got, err := resolveEditor(env(c.env))
		if c.wantErr {
			if err == nil || !strings.Contains(err.Error(), "$EDITOR") {
				t.Errorf("%s: err = %v, want $EDITOR parse error", c.name, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestEditorArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		argv []string
		want []string
	}{
		{[]string{"vim"}, []string{"vim", "+12", "/r/a.go"}},
		{[]string{"/usr/local/bin/nvim"}, []string{"/usr/local/bin/nvim", "+12", "/r/a.go"}},
		{[]string{"nano"}, []string{"nano", "+12", "/r/a.go"}},
		{[]string{"code", "-w"}, []string{"code", "-w", "--goto", "/r/a.go:12"}},
		{[]string{"cursor"}, []string{"cursor", "--goto", "/r/a.go:12"}},
		{[]string{"subl"}, []string{"subl", "/r/a.go:12"}},
		{[]string{"hx"}, []string{"hx", "/r/a.go:12"}},
	}
	for _, c := range cases {
		if got := editorArgs(c.argv, "/r/a.go", 12); !reflect.DeepEqual(got, c.want) {
			t.Errorf("editorArgs(%q) = %q, want %q", c.argv, got, c.want)
		}
	}
	if got := editorArgs([]string{"vim"}, "/r/a.go", 0); got[1] != "+1" {
		t.Errorf("line 0 should clamp to 1, got %q", got)
	}
}

func TestNewLineNear(t *testing.T) {
	t.Parallel()
	n := func(v int) *int { return &v }
	h := &diffmodel.DiffHunk{NewStart: 7, Lines: []diffmodel.DiffLine{
		{Kind: "context", NewLine: n(7)},
		{Kind: "deletion", OldLine: n(8)},
		{Kind: "addition", NewLine: n(8)},
		{Kind: "deletion", OldLine: n(9)},
	}}
	for li, want := range map[int]int{0: 7, 1: 8, 2: 8, 3: 8} {
		if got := newLineNear(h, li); got != want {
			t.Errorf("newLineNear(%d) = %d, want %d", li, got, want)
		}
	}
	empty := &diffmodel.DiffHunk{NewStart: 3, Lines: []diffmodel.DiffLine{{Kind: "deletion", OldLine: n(3)}}}
	if got := newLineNear(empty, 0); got != 3 {
		t.Errorf("all-deleted hunk: got %d, want NewStart 3", got)
	}
}

// makeEditorRepo creates a git repo containing a.go and b.go and returns its
// path and HEAD SHA.
func makeEditorRepo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, f := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	sha, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return dir, sha
}

func makeEditorModel(t *testing.T, headSHA string) *PRDetailModel {
	t.Helper()
	m := makeDiffCursorModel(100, 40)
	m.Summary.HeadRefOID = headSHA
	m.Summary.HeadRefName = "feature/foo"
	return m
}

func pressE(m *PRDetailModel) tea.Cmd {
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	return cmd
}

func runLookup(t *testing.T, cmd tea.Cmd) localHeadMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a HEAD lookup command")
	}
	msg, ok := cmd().(localHeadMsg)
	if !ok {
		t.Fatalf("expected localHeadMsg, got %T", msg)
	}
	return msg
}

func TestOpenInEditorHeadMatchLaunches(t *testing.T) {
	t.Parallel()
	dir, sha := makeEditorRepo(t)
	m := makeEditorModel(t, sha)
	m.Repo.LocalPath = dir

	msg := runLookup(t, pressE(m))
	if msg.target.relPath != "a.go" || msg.target.line != 1 {
		t.Fatalf("target = %+v, want a.go:1", msg.target)
	}
	if pressE(m) != nil {
		t.Fatal("a second e while the lookup is in flight must be ignored")
	}
	if cmd := m.handleLocalHead(msg); cmd == nil || !m.editorLaunching {
		t.Fatal("matching HEAD should launch the editor")
	}
	if pressE(m) != nil {
		t.Fatal("e while the editor is open must be ignored")
	}
	m.handleEditorExited(editorExitedMsg{})
	if m.editorLaunching {
		t.Fatal("editorLaunching should clear when the editor exits")
	}
}

func TestOpenInEditorDeletedLineUsesNextNewLine(t *testing.T) {
	t.Parallel()
	m := makeEditorModel(t, "abc")
	m.Repo.LocalPath = "/repo"
	m.setDiffCursor(diffCursorLine{FileIdx: 0, HunkIdx: 0, LineIdx: 2}) // deletion at end of hunk
	tgt, _, ok := m.editorTargetAtCursor()
	if !ok || tgt.line != 2 {
		t.Fatalf("target = %+v, ok=%v; want line 2 (preceding new line)", tgt, ok)
	}
}

func TestOpenInEditorRemovedFile(t *testing.T) {
	t.Parallel()
	m := makeEditorModel(t, "abc")
	m.Repo.LocalPath = "/repo"
	m.Diff.Files[0].Status = "removed"
	if pressE(m) != nil {
		t.Fatal("removed file should not start a lookup")
	}
	if !strings.Contains(m.StatusHint(), "deleted") {
		t.Fatalf("hint = %q, want deleted notice", m.StatusHint())
	}
}

func TestOpenInEditorNoLocalPath(t *testing.T) {
	t.Parallel()
	m := makeEditorModel(t, "abc")
	if pressE(m) != nil {
		t.Fatal("no local repo should not start a lookup")
	}
	if !strings.Contains(m.StatusHint(), "Not a local repo") {
		t.Fatalf("hint = %q", m.StatusHint())
	}
}

func TestOpenInEditorOnlyInDiff(t *testing.T) {
	t.Parallel()
	m := makeEditorModel(t, "abc")
	m.Repo.LocalPath = "/repo"
	m.switchTab(TabDescription)
	if pressE(m) != nil || m.editPrompt != "" {
		t.Fatal("e outside the diff should do nothing")
	}
}

func TestOpenInEditorStaleResultsIgnored(t *testing.T) {
	t.Parallel()
	dir, sha := makeEditorRepo(t)
	m := makeEditorModel(t, sha)
	m.Repo.LocalPath = dir

	first := runLookup(t, pressE(m))
	m.switchTab(TabDescription) // cancels the lookup
	if cmd := m.handleLocalHead(first); cmd != nil || m.editorLaunching || m.editorPrompt != "" {
		t.Fatal("result after a tab switch must be ignored")
	}

	m.switchTab(TabDiff)
	second := runLookup(t, pressE(m))
	if cmd := m.handleLocalHead(first); cmd != nil || m.editorReq == 0 {
		t.Fatal("an older request's result must not apply to a newer one")
	}

	m.Summary.HeadRefOID = "0000000" // head reloaded under the lookup
	if cmd := m.handleLocalHead(second); cmd != nil || m.editorLaunching || m.editorPrompt != "" {
		t.Fatal("result for an outdated head must be ignored")
	}
}

func TestOpenInEditorMismatchPrompt(t *testing.T) {
	t.Parallel()
	dir, _ := makeEditorRepo(t)
	m := makeEditorModel(t, "ffffffffffffffffffffffffffffffffffffffff")
	m.Repo.LocalPath = dir

	if cmd := m.handleLocalHead(runLookup(t, pressE(m))); cmd != nil {
		t.Fatal("mismatch should prompt, not launch")
	}
	hint := m.StatusHint()
	if !strings.Contains(hint, "Local HEAD (main) differs from PR head") || !strings.Contains(hint, "b: checkout") {
		t.Fatalf("hint = %q", hint)
	}
	if strings.Contains(hint, "behind") {
		t.Fatalf("mismatch must not claim the branch is behind: %q", hint)
	}

	m = pressKey(m, "esc")
	if m.editorPrompt != "" || m.editorReq != 0 {
		t.Fatal("esc should clear the prompt")
	}

	m.handleLocalHead(runLookup(t, pressE(m)))
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil || !m.editorLaunching || m.editorPrompt != "" {
		t.Fatal("o should open anyway")
	}
}

func TestOpenInEditorCommitModeMismatchHasNoCheckout(t *testing.T) {
	t.Parallel()
	dir, _ := makeEditorRepo(t)
	m := makeEditorModel(t, "")
	m.CommitMode = true
	m.Commit.SHA = "ffffffffffffffffffffffffffffffffffffffff"
	m.Repo.LocalPath = dir

	m.handleLocalHead(runLookup(t, pressE(m)))
	if !strings.Contains(m.editorPrompt, "differs from this commit") || strings.Contains(m.editorPrompt, "b:") {
		t.Fatalf("prompt = %q", m.editorPrompt)
	}
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd != nil || m.checkoutInFlight || m.pendingEditor != nil {
		t.Fatal("b must not check out in commit mode")
	}
}

func TestOpenInEditorCheckoutFromPrompt(t *testing.T) {
	t.Parallel()
	dir, _ := makeEditorRepo(t)
	want := "ffffffffffffffffffffffffffffffffffffffff"
	m := makeEditorModel(t, want)
	m.Repo.LocalPath = dir
	m.handleLocalHead(runLookup(t, pressE(m)))

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd == nil || !m.checkoutInFlight {
		t.Fatal("b from the prompt should start a checkout, not be blocked by the prompt")
	}
	if m.editorPrompt != "" || m.pendingEditor == nil || m.pendingEditor.relPath != "a.go" {
		t.Fatalf("prompt=%q pending=%+v", m.editorPrompt, m.pendingEditor)
	}

	// Checkout fails: the pending open is dropped.
	m.Update(checkoutResultMsg{err: errors.New("boom")})
	if m.pendingEditor != nil || m.checkoutErr != "boom" {
		t.Fatal("failed checkout should drop the pending editor")
	}

	// Retry; checkout succeeds but HEAD still differs.
	m.handleLocalHead(runLookup(t, pressE(m)))
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m.checkoutInFlight = false
	lookup := m.resumeEditorAfterCheckout(true)
	if m.pendingEditor != nil {
		t.Fatal("pending editor should be consumed")
	}
	msg := runLookup(t, lookup)
	if !msg.afterCheckout {
		t.Fatal("lookup after checkout should be marked")
	}
	if cmd := m.handleLocalHead(msg); cmd == nil || !m.editorLaunching {
		t.Fatal("after checkout the editor opens even if HEAD differs")
	}
	m.Update(checkoutClearMsg{})
	m.handleEditorExited(editorExitedMsg{notice: "Opened, but local HEAD still differs from PR head"})
	if !strings.Contains(m.StatusHint(), "still differs") {
		t.Fatalf("hint = %q", m.StatusHint())
	}
}

func TestOpenInEditorCheckoutSuccessViaUpdate(t *testing.T) {
	t.Parallel()
	dir, sha := makeEditorRepo(t)
	m := makeEditorModel(t, sha)
	m.Repo.LocalPath = dir
	tgt, _, _ := m.editorTargetAtCursor()
	m.pendingEditor = &tgt
	m.checkoutInFlight = true

	_, cmd := m.Update(cmds.CheckoutResult{Branch: "feature/foo"})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	m.Update(checkoutResultMsg{branch: "feature/foo"})
	if m.pendingEditor != nil || m.editorReq == 0 {
		t.Fatal("successful checkout should start the HEAD re-check")
	}
}

func TestOpenInEditorMissingFile(t *testing.T) {
	t.Parallel()
	dir, sha := makeEditorRepo(t)
	m := makeEditorModel(t, sha)
	m.Repo.LocalPath = dir
	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}
	if cmd := m.handleLocalHead(runLookup(t, pressE(m))); cmd != nil || m.editorLaunching {
		t.Fatal("a file missing locally must not open an empty buffer")
	}
	if !strings.Contains(m.StatusHint(), "a.go not in local checkout") {
		t.Fatalf("hint = %q", m.StatusHint())
	}
}
