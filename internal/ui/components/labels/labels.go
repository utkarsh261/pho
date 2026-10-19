// Package labels renders a pull request's labels as a single-line strip of
// "● name" chips: the dot in the label's GitHub colour, the name muted.
package labels

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// maxNameWidth caps one label's name so a single long label can't push every
// other label out of the strip.
const maxNameWidth = 24

const chipGap = "  "

// Strip renders labels left to right within width columns. Labels that don't
// fit are folded into a trailing "+N", where N also counts labels beyond the
// fetched page (total is the PR's full label count; values below len(labels),
// such as 0 from an older cache entry, fall back to len(labels)). When not even
// one chip fits it degrades to "N labels", then to "". A nil theme renders
// without colour.
func Strip(th *theme.Theme, labels []domain.Label, total, width int) string {
	total = max(total, len(labels))
	if total == 0 || width <= 0 {
		return ""
	}

	chips := make([]string, 0, len(labels))
	for _, l := range labels {
		if c, ok := chip(th, l); ok {
			chips = append(chips, c)
		}
	}

	// Try the most chips first; each step drops one more into the "+N".
	for n := len(chips); n > 0; n-- {
		s := strings.Join(chips[:n], chipGap)
		if rest := total - n; rest > 0 {
			s += chipGap + faint(th, fmt.Sprintf("+%d", rest))
		}
		if lipgloss.Width(s) <= width {
			return s
		}
	}

	noun := "labels"
	if total == 1 {
		noun = "label"
	}
	if s := faint(th, fmt.Sprintf("%d %s", total, noun)); lipgloss.Width(s) <= width {
		return s
	}
	return ""
}

// chip renders one label; ok is false for a label with no visible name,
// which is skipped but still counted in the "+N".
func chip(th *theme.Theme, l domain.Label) (string, bool) {
	name := ansi.Truncate(cleanName(l.Name), maxNameWidth, "…")
	if name == "" {
		return "", false
	}
	if th == nil {
		return "● " + name, true
	}
	dot := th.MutedTxt.Render("●")
	if c, ok := hexColor(l.Color); ok {
		dot = lipgloss.NewStyle().Foreground(c).Render("●")
	}
	return dot + " " + th.MutedTxt.Render(name), true
}

func faint(th *theme.Theme, s string) string {
	if th == nil {
		return s
	}
	return th.FaintTxt.Render(s)
}

// cleanName drops control characters (an escape sequence in a label name
// would otherwise reach the terminal) and folds whitespace runs to one space.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// hexColor turns GitHub's label colour ("d73a4a", no leading #) into a
// lipgloss colour, rejecting anything that isn't six hex digits.
func hexColor(s string) (lipgloss.Color, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return "", false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "", false
		}
	}
	return lipgloss.Color("#" + s), true
}
