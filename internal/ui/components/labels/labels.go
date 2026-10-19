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

// minDotContrast is the contrast a label's colour needs against the theme
// background to be drawn as-is; below it (a white label on a light theme) the
// dot falls back to the muted colour so it doesn't vanish.
const minDotContrast = 1.5

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

	// Keep the longest prefix of chips that fits along with the "+N" for the
	// rest; prefix[n] is the width of the first n chips joined.
	gapW := lipgloss.Width(chipGap)
	prefix := make([]int, len(chips)+1)
	for i, c := range chips {
		prefix[i+1] = prefix[i] + lipgloss.Width(c)
		if i > 0 {
			prefix[i+1] += gapW
		}
	}
	for n := len(chips); n > 0; n-- {
		w := prefix[n]
		more := ""
		if rest := total - n; rest > 0 {
			more = fmt.Sprintf("+%d", rest)
			w += gapW + lipgloss.Width(more)
		}
		if w <= width {
			s := strings.Join(chips[:n], chipGap)
			if more != "" {
				s += chipGap + faint(th, more)
			}
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
	if lipgloss.Width(name) == 0 {
		return "", false
	}
	if th == nil {
		return "● " + name, true
	}
	dot := th.MutedTxt.Render("●")
	if c, ok := dotColor(th, l.Color); ok {
		dot = lipgloss.NewStyle().Foreground(c).Render("●")
	}
	return dot + " " + th.MutedTxt.Render(name), true
}

// dotColor is the label's own colour when it's valid hex that stands out from
// the theme background; otherwise ok is false and the dot stays muted.
func dotColor(th *theme.Theme, color string) (lipgloss.Color, bool) {
	c, ok := hexColor(color)
	if !ok || !th.StandsOutFromBase(string(c), minDotContrast) {
		return "", false
	}
	return c, true
}

func faint(th *theme.Theme, s string) string {
	if th == nil {
		return s
	}
	return th.FaintTxt.Render(s)
}

// cleanName makes a label name safe to draw inline. Control characters (an
// escape sequence would otherwise reach the terminal) become spaces; format
// characters such as bidi overrides (U+202E would mirror the border) and
// zero-width spaces are dropped, except the zero-width joiner that emoji
// sequences need. Whitespace runs fold to one space.
func cleanName(s string) string {
	const zwj = '\u200d'
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r):
			return ' '
		case unicode.Is(unicode.Cf, r) && r != zwj:
			return -1
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
