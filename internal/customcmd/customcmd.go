// Package customcmd runs user-defined shell commands bound to keys in the
// config ([[keybindings]]).
package customcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
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

// Label is how the keymap overlay shows the binding.
func (b Binding) Label() string {
	if b.Name != "" {
		return b.Name
	}
	return strings.TrimSpace(b.Command)
}

// UsesRepoPath reports whether the command refers to {{.RepoPath}}, so it
// can't run for a repo without a local checkout.
func (b Binding) UsesRepoPath() bool { return b.usesRepoPath }

// Parse checks the bindings and compiles their templates. Errors name the
// offending key so a bad config fails at startup, not on a key press.
func Parse(in []Binding) ([]Binding, error) {
	out := make([]Binding, 0, len(in))
	seen := map[string]bool{}
	for _, b := range in {
		b, err := parseOne(b, seen)
		if err != nil {
			return nil, fmt.Errorf("%w (see README: Custom commands)", err)
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
	t, err := template.New(b.Key).Option("missingkey=error").Parse(b.Command)
	if err != nil {
		return b, fmt.Errorf("keybinding %q: %w", b.Key, err)
	}
	b.tmpl = t
	// A zero Data catches unknown fields such as {{.PRNumber}}.
	if _, err := b.Render(Data{}); err != nil {
		return b, fmt.Errorf("keybinding %q: %w", b.Key, err)
	}
	const probe = "\x00repo-path\x00"
	var buf bytes.Buffer
	if err := t.Execute(&buf, Data{RepoPath: probe}); err == nil {
		b.usesRepoPath = strings.Contains(buf.String(), probe)
	}
	return b, nil
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

// Render fills the template. String fields are shell-quoted, so a branch
// name from a PR author can't inject commands.
func (b Binding) Render(d Data) (string, error) {
	q := d
	q.RepoName = Quote(d.RepoName)
	q.RepoPath = Quote(d.RepoPath)
	q.HeadRefName = Quote(d.HeadRefName)
	q.BaseRefName = Quote(d.BaseRefName)
	q.HeadSHA = Quote(d.HeadSHA)
	q.Author = Quote(d.Author)
	q.URL = Quote(d.URL)
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, q); err != nil {
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
