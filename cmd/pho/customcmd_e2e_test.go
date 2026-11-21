package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil/fakegh"
)

// These run the real program (config, wiring, Bubble Tea, sh) against an
// in-process GitHub and press keys in it. Bound commands write what they
// received to a file, so the asserts read a real process's output.

// fieldsCmd prints every field and the working directory, one per line,
// then moves the file into place so a reader never sees half of it.
const fieldsCmd = `{ printf '%s\n' {{.RepoName}} {{.RepoPath}} {{.PrNumber}} {{.HeadRefName}} {{.BaseRefName}} {{.HeadSHA}} {{.Author}} {{.URL}}; pwd; } > OUT.tmp && mv OUT.tmp OUT`

type customEnv struct {
	t    *testing.T
	srv  *fakegh.Server
	dir  string
	root string // discovery root, one dir per repo
	out  string // where fieldsCmd writes
	p    *tea.Program
	scr  *screen
	done chan struct{}
	tty  *os.File // write end of pho's stdin, for a command that reads
}

type customSetup struct {
	toml     string // [[keybindings]]; OUT is replaced with the output path
	prNumber int    // > 0 opens that PR's detail, like `pho pr N`
	editPRs  func(prs []domain.PullRequestSummary)
	noPRs    bool
}

func startCustom(t *testing.T, s customSetup) *customEnv {
	t.Helper()
	opts := fakegh.Options{Repos: 5, PRsPerRepo: 5, Seed: 1, FilesJSON: []byte("[]")}
	if s.prNumber > 0 {
		opts.Repos = 1
		opts.Diff = bench.SynthSmall(5, 48)
	}
	if s.noPRs {
		opts.PRsPerRepo = 0
	}
	srv := fakegh.New(opts)
	t.Cleanup(srv.Close)
	if s.editPRs != nil {
		for _, r := range srv.Repos() {
			srv.EditPRs(r.FullName, s.editPRs)
		}
	}

	dir := t.TempDir()
	isolate(t, dir)
	root := filepath.Join(dir, "src")
	if err := srv.WriteRoot(root); err != nil {
		t.Fatal(err)
	}
	e := &customEnv{t: t, srv: srv, dir: dir, root: root, out: filepath.Join(dir, "out")}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(strings.ReplaceAll(s.toml, "OUT", e.out)), 0o600); err != nil {
		t.Fatal(err)
	}

	runRoot := root
	marker := "feature/"
	if s.prNumber > 0 {
		runRoot = filepath.Join(root, srv.Repos()[0].Name)
		marker = "pkg/mod4/file4.go"
	}
	if s.noPRs {
		marker = srv.Repos()[0].Name
	}
	in, tty, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { in.Close(); tty.Close() })
	e.tty = tty
	run := launchRun{srv: srv, dir: dir, prNumber: s.prNumber, root: runRoot, input: in}
	e.p, e.scr, e.done = run.start(t, marker)
	t.Cleanup(func() {
		e.p.Quit()
		select {
		case <-e.done:
		case <-time.After(5 * time.Second):
			t.Error("pho did not exit")
		}
	})
	e.scr.wait(t, 30*time.Second)
	quiesce(srv, 200*time.Millisecond, 10*time.Second)
	return e
}

func (e *customEnv) press(keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "ctrl+p":
			msg = tea.KeyMsg{Type: tea.KeyCtrlP}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		e.p.Send(msg)
		time.Sleep(30 * time.Millisecond)
	}
}

// mark returns the screen's length, so a later waitText only looks at
// what is drawn after this point.
func (e *customEnv) mark() int {
	e.scr.mu.Lock()
	defer e.scr.mu.Unlock()
	return len(e.scr.text)
}

func (e *customEnv) waitText(from int, text string) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		e.scr.mu.Lock()
		found := bytes.Contains(e.scr.text[min(from, len(e.scr.text)):], []byte(text))
		e.scr.mu.Unlock()
		if found {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.scr.mu.Lock()
	tail := string(e.scr.text[max(from, len(e.scr.text)-3000):])
	e.scr.mu.Unlock()
	e.t.Fatalf("%q not drawn; output since mark:\n%s", text, tail)
}

func (e *customEnv) drawnSince(from int, text string) bool {
	e.scr.mu.Lock()
	defer e.scr.mu.Unlock()
	return bytes.Contains(e.scr.text[min(from, len(e.scr.text)):], []byte(text))
}

// lastLineWith is the most recently drawn line containing text: the
// current state of that row.
func (e *customEnv) lastLineWith(from int, text string) string {
	e.scr.mu.Lock()
	defer e.scr.mu.Unlock()
	lines := strings.Split(string(e.scr.text[min(from, len(e.scr.text)):]), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], text) {
			return lines[i]
		}
	}
	return ""
}

func (e *customEnv) waitFile(path string) string {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			return string(b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatalf("%s never written", path)
	return ""
}

// noFile checks path stays absent for a while: proof a command didn't run.
func (e *customEnv) noFile(path string) {
	e.t.Helper()
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(path); err == nil {
		b, _ := os.ReadFile(path)
		e.t.Fatalf("%s was written, so the command ran:\n%s", path, b)
	}
}

type fields struct {
	repoName, repoPath, number, head, base, sha, author, url, pwd string
}

// readFields parses fieldsCmd's output and checks it describes one real PR.
func (e *customEnv) readFields() (fields, domain.PullRequestSummary) {
	e.t.Helper()
	lines := strings.Split(strings.TrimSuffix(e.waitFile(e.out), "\n"), "\n")
	if len(lines) != 9 {
		e.t.Fatalf("got %d lines, want 9:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	f := fields{lines[0], lines[1], lines[2], lines[3], lines[4], lines[5], lines[6], lines[7], lines[8]}
	n, err := strconv.Atoi(f.number)
	if err != nil {
		e.t.Fatalf("PrNumber %q", f.number)
	}
	for _, pr := range e.srv.PRs(f.repoName) {
		if pr.Number == n {
			return f, pr
		}
	}
	e.t.Fatalf("no PR %s#%d on the server", f.repoName, n)
	return f, domain.PullRequestSummary{}
}

func (e *customEnv) checkFields(f fields, pr domain.PullRequestSummary) {
	e.t.Helper()
	repoDir := filepath.Join(e.root, strings.SplitN(pr.Repo, "/", 2)[1])
	want := fields{
		repoName: pr.Repo, repoPath: repoDir, number: strconv.Itoa(pr.Number),
		head: pr.HeadRefName, base: pr.BaseRefName, sha: pr.HeadRefOID, author: pr.Author,
		url: fmt.Sprintf("https://github.com/%s/pull/%d", pr.Repo, pr.Number), pwd: repoDir,
	}
	// macOS temp dirs live under a /var -> /private/var symlink.
	if r, err := filepath.EvalSymlinks(f.pwd); err == nil {
		f.pwd = r
	}
	if r, err := filepath.EvalSymlinks(want.pwd); err == nil {
		want.pwd = r
	}
	if f != want {
		e.t.Errorf("command saw\n%+v\nwant\n%+v", f, want)
	}
}

const wBinding = "[[keybindings]]\nkey = \"w\"\nname = \"dump fields\"\ncommand = \"\"\"" + fieldsCmd + "\"\"\"\n"

// E1: a key on the dashboard runs with the selected PR's fields, from its
// checkout, and pho redraws once the command exits.
func TestE2ECustomKeyDashboard(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding})
	from := e.mark()
	e.press("w")
	f, pr := e.readFields()
	e.checkFields(f, pr)
	e.waitText(from, "feature/")
}

// E2: in PR detail the key gets that PR, and the view is intact after.
func TestE2ECustomKeyPRDetail(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding, prNumber: 3})
	from := e.mark()
	e.press("w")
	f, pr := e.readFields()
	if pr.Number != 3 {
		t.Fatalf("ran for #%d, want #3", pr.Number)
	}
	e.checkFields(f, pr)
	e.waitText(from, "pkg/mod4/file4.go")
}

// Template functions work on the raw values in a real run.
func TestE2ECustomKeyTemplateFunctions(t *testing.T) {
	e := startCustom(t, customSetup{toml: "[[keybindings]]\nkey = \"w\"\ncommand = '''" +
		`{ echo {{slice .HeadSHA 0 7}}; echo {{if eq .BaseRefName "main"}}onmain{{else}}stacked{{end}}; echo {{.PrNumber}}; } > OUT.tmp && mv OUT.tmp OUT` +
		"'''\n"})
	e.press("w")
	lines := strings.Split(strings.TrimSpace(e.waitFile(e.out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("output %q", lines)
	}
	n, _ := strconv.Atoi(lines[2])
	for _, r := range e.srv.Repos() {
		for _, pr := range e.srv.PRs(r.FullName) {
			if pr.Number != n || !strings.HasPrefix(pr.HeadRefOID, lines[0]) {
				continue
			}
			want := "stacked"
			if pr.BaseRefName == "main" {
				want = "onmain"
			}
			if len(lines[0]) != 7 || lines[1] != want {
				t.Fatalf("got %q, want sha prefix of 7 and %q", lines, want)
			}
			return
		}
	}
	t.Fatalf("no PR #%d with head %s…", n, lines[0])
}

// E3: each key runs its own command and nothing else.
func TestE2ECustomKeyTwoBindings(t *testing.T) {
	e := startCustom(t, customSetup{toml: "[[keybindings]]\nkey = \"w\"\ncommand = \"echo w > OUT-w\"\n" +
		"[[keybindings]]\nkey = \"P\"\ncommand = \"echo P > OUT-P\"\n"})
	e.press("P")
	if got := e.waitFile(e.out + "-P"); got != "P\n" {
		t.Fatalf("OUT-P = %q", got)
	}
	e.noFile(e.out + "-w")
}

// E4: branch names come from PR authors; the shell must see them as data.
func TestE2ECustomKeyHostileBranch(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding, editPRs: func(prs []domain.PullRequestSummary) {
		for i := range prs {
			prs[i].HeadRefName = fmt.Sprintf("feature/x%d;touch PWNED $(touch PWNED2) `touch PWNED3` it's \"q\" $HOME", i)
		}
	}})
	e.press("w")
	f, pr := e.readFields()
	e.checkFields(f, pr)
	for _, name := range []string{"PWNED", "PWNED2", "PWNED3"} {
		if _, err := os.Stat(filepath.Join(f.pwd, name)); err == nil {
			t.Errorf("branch name ran a command: %s exists", name)
		}
	}
}

// A quoted field on a path the startup check doesn't take is refused on the
// key press, and nothing runs.
func TestE2ECustomKeyRefusedOnKeyPress(t *testing.T) {
	e := startCustom(t, customSetup{
		toml: "[[keybindings]]\nkey = \"w\"\ncommand = '''" +
			`{{if eq .BaseRefName "trap"}}echo "{{.HeadRefName}}" > OUT{{else}}echo {{.HeadRefName}} > OUT{{end}}` + "'''\n",
		editPRs: func(prs []domain.PullRequestSummary) {
			for i := range prs {
				prs[i].BaseRefName = "trap"
				prs[i].HeadRefName = "feature/x$(touch PWNED)"
			}
		},
	})
	from := e.mark()
	e.press("w")
	e.waitText(from, "Custom command w: {{.HeadRefName}} is inside quotes")
	e.noFile(e.out)
}

// E5, E6: a failing command leaves pho running and says so.
func TestE2ECustomKeyFailure(t *testing.T) {
	for _, tc := range []struct{ name, command, want string }{
		{"exit code", "exit 3", "Custom command w failed (exit 3)"},
		{"not found", "definitely-not-a-command-pho", "Custom command w failed (exit 127)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := startCustom(t, customSetup{toml: "[[keybindings]]\nkey = \"w\"\ncommand = \"" + tc.command + "\"\n"})
			from := e.mark()
			e.press("w")
			e.waitText(from, tc.want)
			// Still alive and taking keys: the next key redraws the status
			// line without the notice.
			from = e.mark()
			e.press("j")
			e.waitText(from, "j/k Move")
			if line := e.lastLineWith(from, "j/k Move"); strings.Contains(line, "failed") {
				t.Errorf("failure notice outlived the next key: %q", line)
			}
		})
	}
}

// E9: no PR under the cursor, nothing to run.
func TestE2ECustomKeyNoPR(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding, noPRs: true})
	from := e.mark()
	e.press("w")
	e.waitText(from, "No PR selected")
	e.noFile(e.out)
}

// E10: in the palette the key is text.
func TestE2ECustomKeyPalette(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding})
	e.press("ctrl+p")
	from := e.mark()
	e.press("w")
	e.waitText(from, "w")
	e.noFile(e.out)
}

// E11: search, compose and visual mode in PR detail take the key as their
// own; none runs the command.
func TestE2ECustomKeyPRDetailInputModes(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding, prNumber: 3})

	// Search works on the Diff tab.
	e.press("2")
	from := e.mark()
	e.press("/")
	e.press("w")
	e.waitText(from, "/ w")
	e.noFile(e.out)
	e.press("esc")
	e.press("1")

	from = e.mark()
	e.press("C")
	e.waitText(from, "New comment")
	from = e.mark()
	e.press("w")
	e.noFile(e.out)
	if !e.drawnSince(from, "w") {
		t.Error("w was not typed into the composer")
	}
	e.press("esc")

	e.press("2", "l")
	from = e.mark()
	e.press("space")
	e.waitText(from, "Exit visual")
	e.press("w")
	e.noFile(e.out)
	e.press("esc")

	// Back in normal mode, the key works.
	e.press("w")
	e.readFields()
}

// E14: the key keeps working after a run returns.
func TestE2ECustomKeyRepeat(t *testing.T) {
	// Run n writes OUT-n.
	e := startCustom(t, customSetup{toml: "[[keybindings]]\nkey = \"w\"\ncommand = \"echo run >> OUT-log; cp OUT-log OUT-$(wc -l < OUT-log | tr -d ' ')\"\n"})
	e.press("w")
	e.waitFile(e.out + "-1")
	e.press("w")
	if got := e.waitFile(e.out + "-2"); got != "run\nrun\n" {
		t.Fatalf("second run saw %q", got)
	}
}

// The command gets the terminal: it can prompt and read input, and pho
// comes back once it exits.
func TestE2ECustomKeyInteractive(t *testing.T) {
	e := startCustom(t, customSetup{toml: "[[keybindings]]\nkey = \"w\"\ncommand = \"echo CUSTOM-RUN; read x; echo got-$x > OUT\"\n"})
	from := e.mark()
	e.press("w")
	e.waitText(from, "CUSTOM-RUN")
	from = e.mark()
	if _, err := e.tty.WriteString("hello\n"); err != nil {
		t.Fatal(err)
	}
	if got := e.waitFile(e.out); got != "got-hello\n" {
		t.Fatalf("command read %q", got)
	}
	e.waitText(from, "feature/")
}

// The ? overlay lists custom keys by name; with none configured it shows
// no Custom group.
func TestE2ECustomKeyOverlay(t *testing.T) {
	e := startCustom(t, customSetup{toml: wBinding})
	from := e.mark()
	e.press("?")
	e.waitText(from, "dump fields")
	e.waitText(from, "Custom")
	e.press("esc")

	d := startCustom(t, customSetup{toml: wBinding, prNumber: 3})
	from = d.mark()
	d.press("?")
	d.waitText(from, "dump fields")
	d.press("esc")
}

// E13: without [[keybindings]] nothing changes.
func TestE2ECustomKeyNoneConfigured(t *testing.T) {
	e := startCustom(t, customSetup{toml: ""})
	from := e.mark()
	e.press("?")
	e.waitText(from, "Keybindings")
	if e.drawnSince(from, "Custom") {
		t.Error("overlay shows a Custom group with no bindings")
	}
	e.press("esc")
	// An unbound key is ignored and pho keeps taking keys.
	from = e.mark()
	e.press("w")
	time.Sleep(200 * time.Millisecond)
	if e.drawnSince(from, "Custom command") || e.drawnSince(from, "No PR") {
		t.Error("unbound key produced a notice")
	}
	from = e.mark()
	e.press("j") // moves the cursor, so rows redraw
	deadline := time.Now().Add(5 * time.Second)
	for e.mark() == from {
		if time.Now().After(deadline) {
			t.Fatal("pho stopped redrawing after an unbound key")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// E12: a bad binding stops pho at config load, naming what is wrong.
func TestE2ECustomKeyBadConfig(t *testing.T) {
	for _, tc := range []struct{ name, toml, want string }{
		{"reserved", "[[keybindings]]\nkey = \"e\"\ncommand = \"true\"\n", `"e": key is a built-in pho key; free keys: f i p u w`},
		{"duplicate", "[[keybindings]]\nkey = \"w\"\ncommand = \"true\"\n[[keybindings]]\nkey = \"w\"\ncommand = \"true\"\n", `"w" is defined more than once`},
		{"modifier", "[[keybindings]]\nkey = \"ctrl+x\"\ncommand = \"true\"\n", `"ctrl+x": key must be a single character`},
		{"unknown field", "[[keybindings]]\nkey = \"w\"\ncommand = \"echo {{.PRNumber}}\"\n", "PRNumber"},
		{"no command", "[[keybindings]]\nkey = \"w\"\n", "command is empty"},
		{"quoted field", "[[keybindings]]\nkey = \"w\"\ncommand = 'echo \"{{.HeadRefName}}\"'\n", "{{.HeadRefName}} is inside quotes"},
		{"variable in quotes", "[[keybindings]]\nkey = \"w\"\ncommand = '{{$b := .HeadRefName}}echo \"{{$b}}\"'\n", "{{.HeadRefName}} is inside quotes"},
		{"heredoc", "[[keybindings]]\nkey = \"w\"\ncommand = \"\"\"\ncat <<EOF\n{{.HeadRefName}}\nEOF\"\"\"\n", "{{.HeadRefName}} is inside quotes, a heredoc"},
		{"printf", "[[keybindings]]\nkey = \"w\"\ncommand = 'echo {{printf \"%.3s\" .HeadRefName}}'\n", "function printf is not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
