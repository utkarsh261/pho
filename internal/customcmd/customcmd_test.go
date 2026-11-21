package customcmd

import (
	"errors"
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
		// Inside the user's own quotes, $(…) or a ' in a value would escape.
		{name: "field in double quotes", in: []Binding{{Key: "w", Command: `echo "{{.HeadRefName}}"`}}, wantErr: []string{`"w"`, "HeadRefName", "inside quotes"}},
		{name: "field in single quotes", in: []Binding{{Key: "w", Command: `echo '{{.Author}}'`}}, wantErr: []string{"Author", "inside quotes"}},
		{name: "field in quotes in an if", in: []Binding{{Key: "w", Command: `echo "{{if .BaseRefName}}{{.BaseRefName}}{{end}}"`}}, wantErr: []string{"inside quotes"}},
		{name: "piped field in quotes", in: []Binding{{Key: "w", Command: `echo "{{slice .HeadSHA 0 7}}"`}}, wantErr: []string{"HeadSHA", "inside quotes"}},
		{name: "quotes closed before field", in: []Binding{{Key: "w", Command: `echo "a" 'b' {{.HeadRefName}}`}}},
		{name: "escaped quote is not a quote", in: []Binding{{Key: "w", Command: `echo \"{{.HeadRefName}}\"`}}},
		{name: "number in quotes is fine", in: []Binding{{Key: "w", Command: `tmux new-window -n "pr-{{.PrNumber}}"`}}},
		// Ways around a check of the template's text; the rendered command
		// is what gets checked.
		{name: "with dot in quotes", in: []Binding{{Key: "w", Command: `echo "{{with .HeadRefName}}{{.}}{{end}}"`}}, wantErr: []string{"HeadRefName", "inside quotes"}},
		{name: "variable in quotes", in: []Binding{{Key: "w", Command: `{{$b := .HeadRefName}}echo "{{$b}}"`}}, wantErr: []string{"HeadRefName", "inside quotes"}},
		{name: "heredoc", in: []Binding{{Key: "w", Command: "cat <<EOF\n{{.HeadRefName}}\nEOF"}}, wantErr: []string{"HeadRefName", "heredoc"}},
		{name: "quoted heredoc", in: []Binding{{Key: "w", Command: "cat <<'EOF'\n{{.HeadRefName}}\nEOF"}}, wantErr: []string{"HeadRefName"}},
		{name: "quote opened in an if", in: []Binding{{Key: "w", Command: `echo {{if .URL}}"{{end}}{{.HeadRefName}}{{if .URL}}"{{end}}`}}, wantErr: []string{"HeadRefName", "inside quotes"}},
		{name: "after a backslash", in: []Binding{{Key: "w", Command: `echo \{{.HeadRefName}}`}}, wantErr: []string{"HeadRefName"}},
		{name: "after a dollar", in: []Binding{{Key: "w", Command: `echo ${{.HeadRefName}}`}}, wantErr: []string{"HeadRefName"}},
		{name: "in a comment", in: []Binding{{Key: "w", Command: `echo hi # {{.HeadRefName}}`}}, wantErr: []string{"HeadRefName"}},
		{name: "in backquotes", in: []Binding{{Key: "w", Command: "echo `echo {{.HeadRefName}}`"}}, wantErr: []string{"HeadRefName"}},
		{name: "in quoted command substitution", in: []Binding{{Key: "w", Command: `echo "$(echo {{.HeadRefName}})"`}}, wantErr: []string{"HeadRefName"}},
		{name: "in a parameter default", in: []Binding{{Key: "w", Command: `echo ${X:-{{.HeadRefName}}}`}}, wantErr: []string{"HeadRefName"}},
		{name: "in arithmetic", in: []Binding{{Key: "w", Command: `echo $(( {{.HeadRefName}} ))`}}, wantErr: []string{"HeadRefName"}},
		{name: "in [[ ]]", in: []Binding{{Key: "w", Command: `[[ {{.HeadRefName}} -eq 1 ]]`}}, wantErr: []string{"HeadRefName"}},
		{name: "in let", in: []Binding{{Key: "w", Command: `let x={{.HeadRefName}}`}}, wantErr: []string{"HeadRefName"}},
		{name: "in a C-style for", in: []Binding{{Key: "w", Command: `for ((i={{.HeadRefName}}; i<1; i++)); do :; done`}}, wantErr: []string{"HeadRefName"}},
		{name: "in an array index", in: []Binding{{Key: "w", Command: `a[{{.HeadRefName}}]=1`}}, wantErr: []string{"HeadRefName"}},
		{name: "invalid shell", in: []Binding{{Key: "w", Command: `echo {{.HeadRefName}} )`}}, wantErr: []string{"HeadRefName", "isn't valid shell"}},
		{name: "invalid shell without fields is left to sh", in: []Binding{{Key: "w", Command: `echo {{.PrNumber}} )`}}},
		{name: "unquoted command substitution", in: []Binding{{Key: "w", Command: `cd $(git rev-parse --show-toplevel) && echo {{.HeadRefName}} $(echo {{.Author}})`}}},
		{name: "assignment and pipe", in: []Binding{{Key: "w", Command: `B={{.HeadRefName}}; echo "$B" {{.Author}} | cat`}}},
		{name: "env handoff from the docs", in: []Binding{{Key: "w", Command: `tmux new-window -e BRANCH={{.HeadRefName}} 'git switch "$BRANCH"'`}}},
		{name: "glued to text", in: []Binding{{Key: "w", Command: `git worktree add ../wt-{{.HeadRefName}} {{.HeadSHA}}`}}},
		// These re-print a quoted value, which can undo the quoting.
		{name: "html", in: []Binding{{Key: "w", Command: `echo {{html .HeadRefName}}`}}, wantErr: []string{"html", "not supported"}},
		{name: "printf", in: []Binding{{Key: "w", Command: `echo {{printf "%.3s" .HeadRefName}}`}}, wantErr: []string{"printf", "not supported"}},
		{name: "piped urlquery", in: []Binding{{Key: "w", Command: `echo {{.HeadRefName | urlquery}}`}}, wantErr: []string{"urlquery", "not supported"}},
		// A defined template would hide a field from the quote check.
		{name: "define", in: []Binding{{Key: "w", Command: `{{define "x"}}{{.HeadRefName}}{{end}}echo "{{template "x" .}}"`}}, wantErr: []string{"not supported"}},
		{name: "allowed functions", in: []Binding{{Key: "w", Command: `{{if and (ne .BaseRefName "main") (gt (len .HeadSHA) 7)}}{{slice .HeadSHA 0 7}}{{end}}`}}},
		{name: "slice and if load", in: []Binding{{Key: "w", Command: `git show {{slice .HeadSHA 0 7}} {{if .BaseRefName}}{{.BaseRefName}}{{end}}`}}},
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
			if !strings.Contains(err.Error(), docsURL) {
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

// A branch the startup samples don't take is still checked on the key press.
func TestRenderChecksPlacementWithRealValues(t *testing.T) {
	b := mustParse(t, `{{if eq .BaseRefName "main"}}echo "{{.HeadRefName}}"{{else}}echo {{.HeadRefName}}{{end}}`)
	if _, err := b.Render(Data{BaseRefName: "dev", HeadRefName: "x"}); err != nil {
		t.Fatalf("safe branch: %v", err)
	}
	_, err := b.Render(Data{BaseRefName: "main", HeadRefName: "x$(id)"})
	var pe *PlacementError
	if !errors.As(err, &pe) || pe.Field != "{{.HeadRefName}}" {
		t.Fatalf("got %v, want a PlacementError naming HeadRefName", err)
	}
}

// Whatever shape a command takes, a hostile value must not run: pho either
// refuses the command or the shell gets the value as plain text.
func TestHostileValuesNeverRun(t *testing.T) {
	shapes := []string{
		`echo {{.HeadRefName}}`,
		`echo "{{with .HeadRefName}}{{.}}{{end}}"`,
		`{{$b := .HeadRefName}}echo "{{$b}}"`,
		"cat <<EOF\n{{.HeadRefName}}\nEOF",
		`echo {{if .URL}}"{{end}}{{.HeadRefName}}{{if .URL}}"{{end}}`,
		`echo \{{.HeadRefName}}`,
		`echo ${{.HeadRefName}}`,
		`echo '{{.HeadRefName}}'`,
		`echo $(echo {{.HeadRefName}})`,
		"echo `echo {{.HeadRefName}}`",
		`echo "$(echo {{.HeadRefName}})"`,
		`echo ${X:-{{.HeadRefName}}}`,
		`echo $(( {{.HeadRefName}} ))`,
		`a[{{.HeadRefName}}]=1; echo`,
		`for ((i={{.HeadRefName}}; i<1; i++)); do :; done`,
		`for ((i=0; i<{{.HeadRefName}}; i++)); do :; done`,
		`(( {{.HeadRefName}} )); echo`,
		`let {{.HeadRefName}}; echo`,
		`[[ {{.HeadRefName}} -eq 1 ]]; echo`,
		`echo ${X[{{.HeadRefName}}]}`,
		`echo $[ {{.HeadRefName}} ]`,
		`B={{.HeadRefName}}; echo "$B"`,
		`for x in {{.HeadRefName}}; do echo "$x"; done`,
		`case {{.HeadRefName}} in *) echo;; esac`,
		`f() { echo {{.HeadRefName}}; }; f | cat`,
		`echo a{{.HeadRefName}}b{{slice .HeadRefName 1}}c`,
		`test {{.HeadRefName}} = x; echo`,
	}
	values := []string{
		"$(touch PWNED)", "`touch PWNED`", "';touch PWNED;'", `";touch PWNED;"`,
		"a[$(touch PWNED)]", "x\ntouch PWNED", `\';touch PWNED;#`, "x$(touch PWNED)'\"`",
	}
	shells := []string{"sh"}
	if _, err := exec.LookPath("bash"); err == nil {
		shells = append(shells, "bash")
	}
	for _, shape := range shapes {
		bs, err := Parse([]Binding{{Key: "w", Command: shape}})
		if err != nil {
			continue // refused at startup
		}
		for _, v := range values {
			cmd, err := bs[0].Render(Data{HeadRefName: v, URL: "u"})
			if err != nil {
				continue // refused on the key press
			}
			for _, sh := range shells {
				dir := t.TempDir()
				c := exec.Command(sh, "-c", cmd)
				c.Dir = dir
				_ = c.Run()
				if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
					t.Errorf("%s ran %q from shape %q (command %q)", sh, v, shape, cmd)
				}
			}
		}
	}
}

// Template functions see the raw values; only what is printed is quoted.
func TestRenderFunctionsSeeRawValues(t *testing.T) {
	b := mustParse(t, `{{slice .HeadSHA 0 7}} {{if .BaseRefName}}base={{.BaseRefName}}{{else}}nobase{{end}} {{if eq .Author "octo"}}me{{end}} {{slice .HeadRefName 0 4}}`)
	got, err := b.Render(Data{HeadSHA: "0123456789abcdef", Author: "octo", HeadRefName: "it's x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `0123456 nobase me 'it'\''s'`; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	got, err = b.Render(Data{HeadSHA: "0123456789abcdef", BaseRefName: "main", HeadRefName: "feat"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "0123456 base=main  feat"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// A value too short for the template's slice fails on the key press, not
// at startup.
func TestRenderErrorAtRunTime(t *testing.T) {
	b := mustParse(t, `git show {{slice .HeadSHA 0 7}}`)
	if _, err := b.Render(Data{HeadSHA: "abc"}); err == nil {
		t.Fatal("want an error slicing a short value")
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
	// The overlay draws one row per binding.
	if got := (Binding{Command: "git fetch\n\tgit checkout {{.HeadRefName}}\n"}).Label(); got != "git fetch git checkout {{.HeadRefName}}" {
		t.Errorf("Label = %q", got)
	}
	if got := (Binding{Name: "two\nlines", Command: "true"}).Label(); got != "two lines" {
		t.Errorf("Label = %q", got)
	}
}

// The docs list the free keys and allowed functions; keep them true.
func TestDocsMatchCode(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "custom-commands.md"))
	if err != nil {
		t.Fatal(err)
	}
	letters := strings.TrimRight(freeKeys(), " 0123456789")
	for _, want := range []string{
		"Free letters today: `" + letters + "`",
		"The functions allowed are `" + allowedFuncList + "`",
	} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("docs/custom-commands.md is missing %q", want)
		}
	}
	for f := range dataFields {
		if !strings.Contains(string(doc), "| `"+f+"` |") {
			t.Errorf("docs field table is missing %s", f)
		}
	}
	if !strings.HasSuffix(docsURL, "/blob/main/docs/custom-commands.md") {
		t.Errorf("errors link to %s, not the docs file", docsURL)
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
