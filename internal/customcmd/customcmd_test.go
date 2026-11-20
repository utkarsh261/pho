package customcmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	ok := func(key string) Binding { return Binding{Key: key, Command: "echo {{.PrNumber}}"} }
	tests := []struct {
		name    string
		in      []Binding
		wantErr []string // substrings; empty means no error
	}{
		{name: "valid", in: []Binding{ok("w")}},
		{name: "several", in: []Binding{ok("w"), ok("P"), ok("!")}},
		{name: "none", in: nil},
		{name: "reserved letter", in: []Binding{ok("e")}, wantErr: []string{`"e"`, "built-in", "free keys", "f"}},
		{name: "reserved digit", in: []Binding{ok("1")}, wantErr: []string{`"1"`, "built-in"}},
		{name: "reserved question mark", in: []Binding{ok("?")}, wantErr: []string{`"?"`, "built-in"}},
		{name: "reserved slash", in: []Binding{ok("/")}, wantErr: []string{`"/"`, "built-in"}},
		{name: "duplicate", in: []Binding{ok("w"), ok("w")}, wantErr: []string{`"w"`, "more than once"}},
		{name: "modifier", in: []Binding{ok("ctrl+x")}, wantErr: []string{`"ctrl+x"`, "single character"}},
		{name: "two chars", in: []Binding{ok("ab")}, wantErr: []string{`"ab"`, "single character"}},
		{name: "empty key", in: []Binding{ok("")}, wantErr: []string{"key is empty"}},
		{name: "space", in: []Binding{ok(" ")}, wantErr: []string{`" "`, "single character"}},
		{name: "control char", in: []Binding{ok("\t")}, wantErr: []string{"single character"}},
		{name: "empty command", in: []Binding{{Key: "w", Command: "  "}}, wantErr: []string{`"w"`, "command is empty"}},
		{name: "unknown field", in: []Binding{{Key: "w", Command: "echo {{.Nope}}"}}, wantErr: []string{`"w"`, "Nope"}},
		{name: "unclosed action", in: []Binding{{Key: "w", Command: "echo {{.PrNumber"}}, wantErr: []string{`"w"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if len(got) != len(tt.in) {
					t.Fatalf("got %d bindings, want %d", len(got), len(tt.in))
				}
				return
			}
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if !strings.Contains(err.Error(), "see README: Custom commands") {
				t.Errorf("error %q doesn't point to the docs", err)
			}
			for _, s := range tt.wantErr {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("error %q lacks %q", err, s)
				}
			}
		})
	}
}

func TestParseErrorListsOnlyFreeKeys(t *testing.T) {
	_, err := Parse([]Binding{{Key: "e", Command: "true"}})
	if err == nil {
		t.Fatal("want error")
	}
	_, list, ok := strings.Cut(err.Error(), "free keys: ")
	if !ok {
		t.Fatalf("no free key list in %q", err)
	}
	list, _, _ = strings.Cut(list, ";")
	for k := range strings.FieldsSeq(list) {
		if Reserved[k] {
			t.Errorf("free key list has reserved %q", k)
		}
	}
}

func mustParse(t *testing.T, command string) Binding {
	t.Helper()
	bs, err := Parse([]Binding{{Key: "w", Command: command}})
	if err != nil {
		t.Fatal(err)
	}
	return bs[0]
}

func TestRenderFields(t *testing.T) {
	b := mustParse(t, "{{.RepoName}} {{.RepoPath}} {{.PrNumber}} {{.HeadRefName}} {{.BaseRefName}} {{.HeadSHA}} {{.Author}} {{.URL}}")
	got, err := b.Render(Data{
		RepoName: "o/r", RepoPath: "/src/r", PrNumber: 7, HeadRefName: "feat/x", BaseRefName: "main",
		HeadSHA: "abc123", Author: "octo", URL: "https://github.com/o/r/pull/7",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "o/r /src/r 7 feat/x main abc123 octo https://github.com/o/r/pull/7"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// The shell must hand each value back exactly, however hostile.
func TestRenderQuotesForShell(t *testing.T) {
	b := mustParse(t, `printf '%s' {{.HeadRefName}}`)
	for _, v := range []string{
		"plain", "has space", "it's", `say "hi"`, "a;touch PWNED", "$(touch PWNED)",
		"`touch PWNED`", "line\nbreak", "$HOME", "*", "~", "a&&b", "-n", "",
	} {
		cmd, err := b.Render(Data{HeadRefName: v})
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		c := exec.Command("sh", "-c", cmd)
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			t.Fatalf("%q: %v", cmd, err)
		}
		if string(out) != v {
			t.Errorf("value %q came back as %q (command %q)", v, out, cmd)
		}
		if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
			t.Errorf("value %q ran a command", v)
		}
	}
}

func TestUsesRepoPath(t *testing.T) {
	if !mustParse(t, "cd {{.RepoPath}} && lazygit").UsesRepoPath() {
		t.Error("RepoPath use not detected")
	}
	if mustParse(t, "gh pr view {{.PrNumber}}").UsesRepoPath() {
		t.Error("RepoPath use reported for a command without it")
	}
}

func TestLabel(t *testing.T) {
	if got := (Binding{Name: "worktree", Command: "git worktree add"}).Label(); got != "worktree" {
		t.Errorf("Label = %q", got)
	}
	if got := (Binding{Command: "  git worktree add  "}).Label(); got != "git worktree add" {
		t.Errorf("Label = %q", got)
	}
}

func TestReadmeListsFreeLetters(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	letters := strings.TrimRight(freeKeys(), " 0123456789")
	if want := "Free letters today: `" + letters + "`"; !strings.Contains(string(readme), want) {
		t.Errorf("README is missing %q; update the Custom commands section", want)
	}
}

// nonKeyLiterals are single-character case labels in the UI code that
// match text, not key presses (editor.go splits $EDITOR into words).
var nonKeyLiterals = map[string]bool{"\t": true, "\n": true, " ": true, `"`: true, "'": true, `\`: true}

// Every single-character key the UI switches on must be reserved, so a
// custom key can never shadow a built-in one.
func TestReservedCoversUIKeys(t *testing.T) {
	root := filepath.Join("..", "ui")
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				lit, ok := e.(*ast.BasicLit)
				if !ok {
					continue
				}
				var s string
				switch lit.Kind {
				case token.STRING:
					s, _ = strconv.Unquote(lit.Value)
				case token.CHAR:
					r, _, _, _ := strconv.UnquoteChar(lit.Value[1:len(lit.Value)-1], '\'')
					s = string(r)
				}
				if len([]rune(s)) == 1 && !nonKeyLiterals[s] && !Reserved[s] {
					t.Errorf("%s: key %q is handled but not in Reserved", fset.Position(lit.Pos()), s)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
