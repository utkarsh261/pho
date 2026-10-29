package whatsnew

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/release"
)

func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func bg(w, h int) string {
	row := strings.Repeat(".", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

func open(w, h int) Model {
	m := NewModel()
	m.md = nil // plain markdown keeps the assertions readable
	m.SetSize(w, h)
	m.Open()
	return m
}

func TestStates(t *testing.T) {
	m := open(100, 30)
	if out := ansi.Strip(m.ViewOver(bg(100, 30))); !strings.Contains(out, loadingText) {
		t.Errorf("loading view missing %q:\n%s", loadingText, out)
	}

	m.SetError()
	out := ansi.Strip(m.ViewOver(bg(100, 30)))
	for _, want := range []string{errorText, "github.com/utkarsh261/pho/releases", "What's new in pho"} {
		if !strings.Contains(out, want) {
			t.Errorf("error view missing %q:\n%s", want, out)
		}
	}

	m.SetReleases([]release.Release{{Version: "1.2.0", Body: "## Changelog\n* abc1234: Add W (#9) (@u)"}})
	out = ansi.Strip(m.ViewOver(bg(100, 30)))
	for _, want := range []string{"## v1.2.0", "* Add W (#9)"} {
		if !strings.Contains(out, want) {
			t.Errorf("loaded view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, errorText) {
		t.Error("loaded view still shows the error")
	}
}

func TestHiddenLeavesBackground(t *testing.T) {
	m := NewModel()
	m.SetSize(80, 20)
	if got := m.ViewOver(bg(80, 20)); got != bg(80, 20) {
		t.Error("hidden panel changed the background")
	}
}

func TestCloseKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{runes("W"), runes("q"), {Type: tea.KeyEsc}} {
		m := open(100, 30)
		m, _ = m.Update(k)
		if m.Visible {
			t.Errorf("%q left the panel open", k.String())
		}
	}
}

func TestScroll(t *testing.T) {
	var rs []release.Release
	for i := 9; i >= 0; i-- {
		rs = append(rs, release.Release{Version: fmt.Sprintf("1.%d.0", i), Body: "* one\n* two\n* three"})
	}
	m := open(100, 24)
	m.SetReleases(rs)

	first := ansi.Strip(m.ViewOver(bg(100, 24)))
	if !strings.Contains(first, "↓") {
		t.Errorf("long notes show no more-below marker:\n%s", first)
	}
	m, _ = m.Update(runes("j"))
	if m.offset != 1 {
		t.Errorf("offset after j = %d, want 1", m.offset)
	}
	m, _ = m.Update(runes("k"))
	m, _ = m.Update(runes("k"))
	if m.offset != 0 {
		t.Errorf("offset after scrolling above the top = %d, want 0", m.offset)
	}
	m, _ = m.Update(runes("G"))
	if want := len(m.bodyLines()) - m.bodyHeight(); m.offset != want {
		t.Errorf("offset after G = %d, want %d", m.offset, want)
	}
	if out := ansi.Strip(m.ViewOver(bg(100, 24))); strings.Contains(out, "↓") || !strings.Contains(out, "## v1.0.0") {
		t.Errorf("bottom of the notes not shown:\n%s", out)
	}
	m.Open()
	if m.offset != 0 {
		t.Error("reopening didn't scroll back to the top")
	}
}

func TestFitsSmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {20, 14}, {40, 8}, {12, 6}, {2, 2}} {
		w, h := size[0], size[1]
		m := open(w, h)
		m.SetError()
		bw, bh := m.boxSize()
		if bw > w || bh > h {
			t.Errorf("%dx%d: box is %dx%d", w, h, bw, bh)
		}
		lines := strings.Split(m.ViewOver(bg(w, h)), "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d rows", w, h, len(lines))
		}
		for i, line := range lines {
			if lw := ansi.StringWidth(line); lw != w {
				t.Errorf("%dx%d: row %d is %d wide", w, h, i, lw)
			}
		}
	}
}

func TestOpenAfterFailureShowsLoading(t *testing.T) {
	m := open(100, 30)
	m.SetError()
	m.Visible = false
	m.Open()
	if out := ansi.Strip(m.ViewOver(bg(100, 30))); !strings.Contains(out, loadingText) || strings.Contains(out, errorText) {
		t.Errorf("reopened panel should be loading:\n%s", out)
	}
}
