package theme

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// RenderHints styles a hint string of the form "Key: Desc | Key: Desc"
// (segments may also be separated by runs of 3+ spaces) as
// "key desc · key desc", with keys emphasised and descriptions muted.
//
// Segments that are not key hints, such as a prompt ("Close #9? (y/n)") or
// the sentence in "2 drafts belong to the previous head | D: Discard", keep
// the brighter StatusHelp colour. A string with no key hint at all (an error
// such as "Failed: GraphQL: ...") is returned unchanged in StatusHelp, so
// messages are never split apart or dimmed.
func (t *Theme) RenderHints(hint string) string {
	segs := splitHints(hint)
	if len(segs) == 0 {
		return ""
	}
	out := make([]string, len(segs))
	anyKey := false
	for i, s := range segs {
		if key, desc, ok := strings.Cut(s, ": "); ok && isKeyName(key) {
			out[i] = t.HintKey.Render(key) + " " + t.HintDesc.Render(desc)
			anyKey = true
		} else if isKeyName(s) && len(s) <= 3 {
			out[i] = t.HintKey.Render(s) // a bare key such as "?"
		} else {
			out[i] = t.StatusHelp.Render(s)
		}
	}
	if !anyKey {
		return t.StatusHelp.Render(strings.TrimSpace(hint))
	}
	return strings.Join(out, lipgloss.NewStyle().Foreground(t.Faint).Render("  ·  "))
}

// namedKeys are the multi-letter key names used in hints.
var namedKeys = map[string]bool{
	"tab": true, "enter": true, "esc": true, "space": true, "backspace": true,
	"up": true, "down": true, "left": true, "right": true, "pgup": true, "pgdn": true,
}

// isKeyName reports whether s names a key or key combination ("?", "j/k",
// "1/2/3/4", "Ctrl+E", "Esc / q", "Tab") rather than starting a sentence.
func isKeyName(s string) bool {
	if s == "" || len(s) > 12 {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		part = strings.TrimSpace(part)
		for _, k := range strings.Split(part, "+") {
			if k == "" {
				continue
			}
			if utf8.RuneCountInString(k) > 2 && !namedKeys[strings.ToLower(k)] &&
				!strings.EqualFold(k, "ctrl") && !strings.EqualFold(k, "shift") && !strings.EqualFold(k, "alt") {
				return false
			}
		}
	}
	return true
}

// splitHints splits a hint string on " | " or runs of 3+ spaces.
func splitHints(hint string) []string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return nil
	}
	var segs []string
	for part := range strings.SplitSeq(hint, " | ") {
		for p := range strings.SplitSeq(part, "   ") {
			if p = strings.TrimSpace(p); p != "" {
				segs = append(segs, p)
			}
		}
	}
	return segs
}

// FillBg renders s padded/truncated to exactly width cells on background bg.
// Unlike Style.Background, the background survives ANSI resets emitted by
// already-styled segments inside s, so the whole row is tinted.
func FillBg(bg lipgloss.Color, width int, s string) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) > width {
		s = lipgloss.NewStyle().MaxWidth(width).Render(s)
	}
	if seq := bgSequence(bg); seq != "" {
		s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+seq)
	}
	return lipgloss.NewStyle().Background(bg).Width(width).Render(s)
}

// bgSeqCache memoises the escape sequence that sets each background colour,
// keyed by colour and colour profile (the sequence is "" without colour support).
var bgSeqCache sync.Map // bgSeqKey -> string

type bgSeqKey struct {
	bg      lipgloss.Color
	profile termenv.Profile
}

// bgSequence returns the escape sequence lipgloss emits to set background bg.
func bgSequence(bg lipgloss.Color) string {
	key := bgSeqKey{bg, lipgloss.ColorProfile()}
	if v, ok := bgSeqCache.Load(key); ok {
		return v.(string)
	}
	probe := lipgloss.NewStyle().Background(bg).Render("x")
	seq := ""
	if i := strings.Index(probe, "x"); i > 0 {
		seq = probe[:i]
	}
	bgSeqCache.Store(key, seq)
	return seq
}
