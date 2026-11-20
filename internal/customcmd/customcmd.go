// Package customcmd runs user-defined shell commands bound to keys in the
// config ([[keybindings]]).
package customcmd

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"mvdan.cc/sh/v3/syntax"
)

// Data is what a command template can use. docs/custom-commands.md
// documents these fields; keep the two in step.
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
	b.tmpl = t
	b.usesRepoPath = fields["RepoPath"]
	// A branch the samples don't take is checked by Render at key press.
	var pe *PlacementError
	if _, err := b.Render(sampleData); errors.As(err, &pe) {
		return b, fmt.Errorf("keybinding %q: %w", b.Key, err)
	}
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

// shellWord prints as a marker that Render swaps for the quoted value once
// it has checked where the marker landed; if, eq and slice see the raw value.
type shellWord string

func (w shellWord) String() string { return "'" + markTag + hex.EncodeToString([]byte(w)) + "'" }

var markTag = func() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "pho" + hex.EncodeToString(b) + "-"
}()

var markRe = regexp.MustCompile("'" + markTag + "([0-9a-f]*)'")

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
	return placeValues(buf.String(), d)
}

// sampleData names each field in its value, long enough for slice.
var sampleData = func() Data {
	var d Data
	v := reflect.ValueOf(&d).Elem()
	for i := range v.NumField() {
		if f := v.Field(i); f.Kind() == reflect.String {
			f.SetString(v.Type().Field(i).Name + strings.Repeat("0", 40))
		}
	}
	d.PrNumber = 1
	return d
}()

// PlacementError is a value printed where its quoting can't protect it.
type PlacementError struct {
	Field  string // "{{.HeadRefName}}", or "a value"
	Reason string
}

func (e *PlacementError) Error() string {
	return fmt.Sprintf("%s %s; pho quotes values itself, so use it as a bare word, like: git switch {{.HeadRefName}}",
		e.Field, e.Reason)
}

// placeValues parses the command as shell and accepts a marker only as a
// plain '…' word part. Inside "…", a heredoc or a comment, or after a
// backslash, it isn't one, and the value's own quotes wouldn't hold there.
func placeValues(cmd string, d Data) (string, error) {
	marks := markRe.FindAllStringIndex(cmd, -1)
	if len(marks) == 0 {
		return cmd, nil
	}
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return "", &PlacementError{Field: fieldAt(cmd, marks[0], d),
			Reason: "is in a command that isn't valid shell (" + err.Error() + ")"}
	}
	safe := safeMarks(f)
	for _, m := range marks {
		if !safe[uint(m[0])] {
			return "", &PlacementError{Field: fieldAt(cmd, m, d),
				Reason: "is inside quotes, a heredoc, ${…}, $((…)), [[…]] or a comment"}
		}
	}
	return markRe.ReplaceAllStringFunc(cmd, func(m string) string { return Quote(decodeMark(m)) }), nil
}

// safeMarks returns the offsets of markers that are plain '…' word parts,
// not under "…", a heredoc, backquotes, ${…}, arithmetic (which includes
// array indexes), let or [[…]].
func safeMarks(f *syntax.File) map[uint]bool {
	safe := map[uint]bool{}
	badChild := map[syntax.Node]bool{}
	var stack []bool
	unsafe := 0
	syntax.Walk(f, func(n syntax.Node) bool {
		if n == nil {
			if stack[len(stack)-1] {
				unsafe--
			}
			stack = stack[:len(stack)-1]
			return true
		}
		bad := badChild[n]
		switch n := n.(type) {
		case *syntax.Redirect:
			if n.Hdoc != nil {
				badChild[n.Hdoc] = true
			}
		case *syntax.Assign:
			if n.Index != nil {
				badChild[n.Index] = true
			}
		case *syntax.CmdSubst:
			bad = n.Backquotes
		case *syntax.DblQuoted, *syntax.ParamExp, *syntax.ArithmExp, *syntax.ArithmCmd,
			*syntax.LetClause, *syntax.TestClause:
			bad = true
		case *syntax.SglQuoted:
			if unsafe == 0 && !n.Dollar && markRe.MatchString("'"+n.Value+"'") {
				safe[n.Pos().Offset()] = true
			}
		}
		if bad {
			unsafe++
		}
		stack = append(stack, bad)
		return true
	})
	return safe
}

func decodeMark(m string) string {
	v, _ := hex.DecodeString(markRe.FindStringSubmatch(m)[1])
	return string(v)
}

// fieldAt names the field whose value (or a slice of it) the marker at m
// holds.
func fieldAt(cmd string, m []int, d Data) string {
	v := decodeMark(cmd[m[0]:m[1]])
	rv := reflect.ValueOf(d)
	name := "a value"
	for i := range rv.NumField() {
		f := rv.Field(i)
		if f.Kind() != reflect.String {
			continue
		}
		if f.String() == v {
			return "{{." + rv.Type().Field(i).Name + "}}"
		}
		if v != "" && name == "a value" && strings.Contains(f.String(), v) {
			name = "{{." + rv.Type().Field(i).Name + "}}"
		}
	}
	return name
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
