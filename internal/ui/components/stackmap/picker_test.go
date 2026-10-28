package stackmap

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/ui/theme"
)

func TestPickerKeepsBackgroundWidth(t *testing.T) {
	t.Parallel()
	for _, w := range []int{30, 80, 160} {
		bg := make([]string, 30)
		for i := range bg {
			bg[i] = strings.Repeat("x", w-1) + "|"
		}
		var p Picker
		p.SetTheme(theme.Default())
		p.SetSize(w, 30)
		p.Open(testStack(), 2)
		out := strings.Split(p.ViewOver(strings.Join(bg, "\n")), "\n")
		if len(out) != 30 {
			t.Fatalf("width %d: %d lines", w, len(out))
		}
		for i, l := range out {
			if lw := lipgloss.Width(l); lw != w {
				t.Fatalf("width %d: line %d is %d wide: %q", w, i, lw, ansi.Strip(l))
			}
			if !strings.HasSuffix(ansi.Strip(l), "|") {
				t.Fatalf("width %d: line %d lost the right edge: %q", w, i, ansi.Strip(l))
			}
		}
	}
}

func TestPickerMovesAndSelects(t *testing.T) {
	t.Parallel()
	var p Picker
	p.Open(testStack(), 2)
	if m, _ := p.Selected(); m.Number != 2 {
		t.Fatalf("starts on #%d, want the current #2", m.Number)
	}
	p.Move(5)
	if m, _ := p.Selected(); m.Number != 1 {
		t.Fatalf("after moving past the end: #%d, want #1", m.Number)
	}
	p.Move(-10)
	if m, _ := p.Selected(); m.Number != 3 {
		t.Fatalf("after moving past the top: #%d, want #3", m.Number)
	}
}
