package theme

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		s = reapplyAfterResets(s, seq)
	}
	return lipgloss.NewStyle().Background(bg).Width(width).Render(s)
}

const sgrReset = "\x1b[0m"

// reapplyAfterResets writes seq again after every SGR reset inside s, so a
// background or attribute set around s survives its styled segments.
func reapplyAfterResets(s, seq string) string {
	return strings.ReplaceAll(s, sgrReset, sgrReset+seq)
}

// PaintRow sets background bg under a whole rendered row, padding it with
// spaces to width. It is a cheaper FillBg for full-screen painting: rows
// wider than width are left as they are rather than truncated.
func PaintRow(bg lipgloss.Color, width int, s string) string {
	seq := bgSequence(bg)
	if seq == "" {
		return s
	}
	pad := max(width-rowWidth(s), 0)
	return seq + reapplyAfterResets(s, seq) + strings.Repeat(" ", pad) + sgrReset
}

// rowWidth is ansi.StringWidth with a fast path for rows of ASCII, box
// drawing and other narrow symbols, which is nearly every rendered row. It
// falls back to the full measure on combining marks or possibly wide runes.
func rowWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c == 0x1b { // skip a CSI sequence: ESC [ params final
			if i+1 >= len(s) || s[i+1] != '[' {
				return ansi.StringWidth(s) // OSC and others: full parser
			}
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
			i++
			continue
		}
		if c < 0x80 {
			if c >= 0x20 && c != 0x7f {
				w++
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if !narrowRune(r) {
			return ansi.StringWidth(s)
		}
		w++
		i += size
	}
	return w
}

// narrowRune reports runes known to take one cell: Latin-1 and Latin
// Extended (below the combining marks at U+0300), and the punctuation,
// arrows, maths, box drawing, shapes and check marks the UI draws with.
// Scripts with combining marks, emoji-width shapes and anything else go to
// the full measure.
func narrowRune(r rune) bool {
	switch {
	case r == utf8.RuneError:
		return false
	case r < 0xA0:
		return r < 0x80 // C1 control characters take no cell
	case r < 0x300:
		return r != 0xAD // soft hyphen takes no cell
	case r >= 0x2010 && r <= 0x2027, r >= 0x2190 && r <= 0x21ff, r >= 0x2200 && r <= 0x22ff:
		return true
	case r >= 0x2500 && r <= 0x25fc:
		return true
	case r == 0x2713 || r == 0x2714 || r == 0x2717 || r == 0x2718:
		return true
	}
	return false
}

// FillRow is FillBg for cursor and selection rows. On 16-colour terminals a
// palette with ANSIFallback marks the row with reverse video instead, since
// its tints there land on the user's own colours and can vanish.
func (t *Theme) FillRow(bg lipgloss.Color, width int, s string) string {
	if t.FallbackActive() {
		return fillReverse(width, s, "")
	}
	return FillBg(bg, width, s)
}

// FillCursorInSelection is FillRow for the cursor row inside a range
// selection. With the 16-colour fallback both are reverse video, so the
// cursor is also underlined to stay visible.
func (t *Theme) FillCursorInSelection(width int, s string) string {
	if t.FallbackActive() {
		return fillReverse(width, s, "\x1b[4m")
	}
	return FillBg(t.Selection, width, s)
}

// FallbackActive reports whether the 16-colour fallback is in use: the
// palette asks for it and the terminal has only 16 colours.
func (t *Theme) FallbackActive() bool {
	return t.ANSIFallback && lipgloss.ColorProfile() == termenv.ANSI
}

// fillReverse renders s padded/truncated to width cells in reverse video,
// plus any extra SGR sequence, re-applied after every reset inside s like
// FillBg's background.
func fillReverse(width int, s, extra string) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) > width {
		s = lipgloss.NewStyle().MaxWidth(width).Render(s)
	}
	reverse := "\x1b[7m" + extra
	pad := max(width-lipgloss.Width(s), 0)
	return reverse + reapplyAfterResets(s, reverse) + strings.Repeat(" ", pad) + sgrReset
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

// Backdrop renders line stripped of its colours in the faint foreground, for
// content behind a modal box.
func Backdrop(t *Theme, line string) string {
	plain := ansi.Strip(line)
	if t == nil || strings.TrimSpace(plain) == "" {
		return plain
	}
	return lipgloss.NewStyle().Foreground(t.Faint).Render(plain)
}
