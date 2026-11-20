// Package customcmd runs user-defined shell commands bound to keys in the
// config ([[keybindings]]).
package customcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// Data is what a command template can use. README's "Custom commands"
// section documents these fields; keep the two in step.
type Data struct {
	RepoName    string
	RepoPath    string
	PrNumber    int
	HeadRefName string
	BaseRefName string
	HeadSHA     string
	Author      string
	URL         string
}

// Reserved holds every single-character key pho binds in any view.
// TestReservedCoversUIKeys fails when a new built-in key is missing here.
var Reserved = setOf("a b c d e g h j k l m n o q r s t v x y z " +
	"C D E G H J K L M N O R S T U W Z " +
	"1 2 3 4 / ? [ ] { }")

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for k := range strings.FieldsSeq(s) {
		m[k] = true
	}
	return m
}

type Binding struct {
	Key     string
	Name    string
	Command string

	tmpl         *template.Template
	usesRepoPath bool
}

// Label is how the keymap overlay shows the binding, on one line.
func (b Binding) Label() string {
	s := b.Name
	if strings.TrimSpace(s) == "" {
		s = b.Command
	}
	return strings.Join(strings.Fields(s), " ")
}

// UsesRepoPath reports whether the command refers to {{.RepoPath}}, so it
// can't run for a repo without a local checkout.
func (b Binding) UsesRepoPath() bool { return b.usesRepoPath }

// docsURL is where config errors point; Homebrew installs have no docs.
const docsURL = "https://github.com/utkarsh261/pho/blob/main/docs/custom-commands.md"

// Parse checks the bindings and compiles their templates. Errors name the
// offending key so a bad config fails at startup, not on a key press.
func Parse(in []Binding) ([]Binding, error) {
	out := make([]Binding, 0, len(in))
	seen := map[string]bool{}
	for _, b := range in {
		b, err := parseOne(b, seen)
		if err != nil {
			return nil, fmt.Errorf("%w (see %s)", err, docsURL)
		}
		out = append(out, b)
	}
	return out, nil
}

func parseOne(b Binding, seen map[string]bool) (Binding, error) {
	if err := checkKey(b.Key); err != nil {
		return b, err
	}
	if seen[b.Key] {
		return b, fmt.Errorf("keybinding %q is defined more than once", b.Key)
	}
	seen[b.Key] = true
	if strings.TrimSpace(b.Command) == "" {
		return b, fmt.Errorf("keybinding %q: command is empty", b.Key)
	}
	t, err := template.New(b.Key).Parse(b.Command)
	if err != nil {
		return b, fmt.Errorf("keybinding %q: %w", b.Key, err)
	}
	// Checked on the parse tree rather than by rendering, so functions
	// such as slice aren't run on made-up values.
	fields := map[string]bool{}
	var badFunc string
	if len(t.Templates()) > 1 {
		badFunc = "define"
	}
	walk(t.Tree.Root, func(n parse.Node) {
		fieldsOf(n, fields)
		if _, ok := n.(*parse.TemplateNode); ok && badFunc == "" {
			badFunc = "template"
		}
		if id, ok := n.(*parse.IdentifierNode); ok && !allowedFuncs[id.Ident] && badFunc == "" {
			badFunc = id.Ident
		}
	})
	if badFunc != "" {
		return b, fmt.Errorf("keybinding %q: function %s is not supported; use %s", b.Key, badFunc, allowedFuncList)
	}
	for f := range fields {
		if !dataFields[f] {
			return b, fmt.Errorf("keybinding %q: unknown field {{.%s}}", b.Key, f)
		}
	}
	if f, ok := quotedField(t.Tree.Root, outside); ok {
		return b, fmt.Errorf("keybinding %q: {{.%s}} is inside quotes; pho quotes values itself, so drop the quotes around it", b.Key, f)
	}
	b.tmpl = t
	b.usesRepoPath = fields["RepoPath"]
	return b, nil
}

// allowedFuncs leave values as they are. printf, html, js and urlquery are
// out: they re-print an already quoted value and can break its quotes.
const allowedFuncList = "and or not eq ne lt le gt ge len slice"

var allowedFuncs = setOf(allowedFuncList)

var dataFields = func() map[string]bool {
	m := map[string]bool{}
	t := reflect.TypeFor[Data]()
	for i := range t.NumField() {
		m[t.Field(i).Name] = true
	}
	return m
}()

func walk(n parse.Node, visit func(parse.Node)) {
	if n == nil || reflect.ValueOf(n).IsNil() {
		return
	}
	visit(n)
	switch n := n.(type) {
	case *parse.ListNode:
		for _, c := range n.Nodes {
			walk(c, visit)
		}
	case *parse.ActionNode:
		walk(n.Pipe, visit)
	case *parse.PipeNode:
		for _, c := range n.Cmds {
			walk(c, visit)
		}
	case *parse.CommandNode:
		for _, c := range n.Args {
			walk(c, visit)
		}
	case *parse.ChainNode:
		walk(n.Node, visit)
	case *parse.IfNode:
		walkBranch(&n.BranchNode, visit)
	case *parse.RangeNode:
		walkBranch(&n.BranchNode, visit)
	case *parse.WithNode:
		walkBranch(&n.BranchNode, visit)
	case *parse.TemplateNode:
		walk(n.Pipe, visit)
	}
}

func walkBranch(b *parse.BranchNode, visit func(parse.Node)) {
	walk(b.Pipe, visit)
	walk(b.List, visit)
	walk(b.ElseList, visit)
}

// fieldsOf adds the Data fields n refers to: .X, $.X, or .X.Y (which then
// fails as unknown, since no field has fields).
func fieldsOf(n parse.Node, into map[string]bool) {
	switch n := n.(type) {
	case *parse.FieldNode:
		into[strings.Join(n.Ident, ".")] = true
	case *parse.VariableNode:
		if len(n.Ident) > 1 && n.Ident[0] == "$" {
			into[strings.Join(n.Ident[1:], ".")] = true
		}
	}
}

type quoteState int

const (
	outside quoteState = iota
	inSingle
	inDouble
)

// quotedField finds a string field used inside the command's own quotes,
// where Quote's output wouldn't protect it: $(…) still runs inside "…",
// and a ' in the value ends '…'. PrNumber is only digits, so it may be.
func quotedField(list *parse.ListNode, st quoteState) (string, bool) {
	if list == nil {
		return "", false
	}
	for _, n := range list.Nodes {
		var pipe *parse.PipeNode
		var branch *parse.BranchNode
		switch n := n.(type) {
		case *parse.TextNode:
			st = advance(st, n.Text)
			continue
		case *parse.ActionNode:
			pipe = n.Pipe
		case *parse.IfNode:
			branch = &n.BranchNode
		case *parse.RangeNode:
			branch = &n.BranchNode
		case *parse.WithNode:
			branch = &n.BranchNode
		}
		if branch != nil {
			pipe = branch.Pipe
		}
		if st != outside && pipe != nil && branch == nil {
			fields := map[string]bool{}
			walk(pipe, func(n parse.Node) { fieldsOf(n, fields) })
			for f := range fields {
				if f != "PrNumber" {
					return f, true
				}
			}
		}
		if branch != nil {
			if f, ok := quotedField(branch.List, st); ok {
				return f, true
			}
			if f, ok := quotedField(branch.ElseList, st); ok {
				return f, true
			}
		}
	}
	return "", false
}

// advance tracks sh quoting through literal text.
func advance(st quoteState, text []byte) quoteState {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch st {
		case outside:
			switch c {
			case '\\':
				i++
			case '\'':
				st = inSingle
			case '"':
				st = inDouble
			}
		case inSingle:
			if c == '\'' {
				st = outside
			}
		case inDouble:
			switch c {
			case '\\':
				i++
			case '"':
				st = outside
			}
		}
	}
	return st
}

func checkKey(key string) error {
	if key == "" {
		return errors.New("keybinding key is empty")
	}
	r := []rune(key)
	if len(r) != 1 || !unicode.IsPrint(r[0]) || unicode.IsSpace(r[0]) {
		return fmt.Errorf("keybinding %q: key must be a single character, like \"w\" or \"P\"", key)
	}
	if Reserved[key] {
		return fmt.Errorf("keybinding %q: key is a built-in pho key; free keys: %s; most punctuation is free too",
			key, freeKeys())
	}
	return nil
}

func freeKeys() string {
	var free []string
	for _, rng := range []string{"abcdefghijklmnopqrstuvwxyz", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "0123456789"} {
		for _, r := range rng {
			if !Reserved[string(r)] {
				free = append(free, string(r))
			}
		}
	}
	return strings.Join(free, " ")
}

// shellWord prints shell-quoted, so a branch name from a PR author can't
// inject commands, but if, eq and slice see the raw value.
type shellWord string

func (w shellWord) String() string { return Quote(string(w)) }

// words mirrors Data with its strings as shellWords.
type words struct {
	RepoName    shellWord
	RepoPath    shellWord
	PrNumber    int
	HeadRefName shellWord
	BaseRefName shellWord
	HeadSHA     shellWord
	Author      shellWord
	URL         shellWord
}

// Render fills the template; printed string fields come out shell-quoted.
func (b Binding) Render(d Data) (string, error) {
	var buf bytes.Buffer
	err := b.tmpl.Execute(&buf, words{
		RepoName: shellWord(d.RepoName), RepoPath: shellWord(d.RepoPath), PrNumber: d.PrNumber,
		HeadRefName: shellWord(d.HeadRefName), BaseRefName: shellWord(d.BaseRefName),
		HeadSHA: shellWord(d.HeadSHA), Author: shellWord(d.Author), URL: shellWord(d.URL),
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Quote returns s as one POSIX shell word, leaving it bare when that is
// already safe so commands stay readable.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@%+:,./_-", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// FinishedMsg reports a custom command's exit.
type FinishedMsg struct {
	Key string
	Err error
}

// Exec runs command with sh in dir, handing it the terminal until it exits.
// sh rather than $SHELL: Quote's output is POSIX syntax.
func Exec(key, command, dir string) tea.Cmd {
	c := exec.Command("sh", "-c", command)
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return FinishedMsg{Key: key, Err: err} })
}

// FailureNotice describes a failed run for the status bar.
func FailureNotice(msg FinishedMsg) string {
	var exitErr *exec.ExitError
	if errors.As(msg.Err, &exitErr) {
		return fmt.Sprintf("Custom command %s failed (exit %d)", msg.Key, exitErr.ExitCode())
	}
	return fmt.Sprintf("Custom command %s failed: %v", msg.Key, msg.Err)
}
