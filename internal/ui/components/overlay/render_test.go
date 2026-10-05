package overlay

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func prResults(n int) []domain.SearchResult {
	out := make([]domain.SearchResult, n)
	for i := range out {
		out[i] = domain.SearchResult{Kind: domain.SearchResultPR, Repo: "org/x", Number: i + 1, Title: fmt.Sprintf("PR %d", i+1)}
	}
	return out
}

// The box grows with the result list (capped) and always fits the terminal,
// with every row exactly the box width.
func TestBoxSizesToResultsAndFits(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {60, 12}, {40, 10}} {
		for _, n := range []int{0, 1, 5, 30} {
			m := NewModel(nil)
			m.SetTheme(theme.Default())
			m.width, m.height = size[0], size[1]
			m.SetResults(prResults(n))

			boxW, boxH := m.boxSize()
			wantRows := max(min(n, defaultSearchLimit, size[1]-boxChrome-2), 1)
			if boxH != boxChrome+wantRows {
				t.Errorf("%v n=%d: boxH = %d, want %d", size, n, boxH, boxChrome+wantRows)
			}
			lines := strings.Split(m.ViewOver(strings.Repeat(strings.Repeat(".", size[0])+"\n", size[1]-1)+strings.Repeat(".", size[0])), "\n")
			if len(lines) != size[1] {
				t.Fatalf("%v n=%d: %d lines, want %d", size, n, len(lines), size[1])
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != size[0] {
					t.Errorf("%v n=%d: line %d width %d, want %d", size, n, i, w, size[0])
				}
			}
			box := strings.Split(m.renderBox(boxW, boxH), "\n")
			if !strings.HasPrefix(ansi.Strip(box[len(box)-1]), "╰") {
				t.Errorf("%v n=%d: bottom border missing", size, n)
			}
		}
	}
}

// Scrolling keeps the selected row inside the (content-sized) box.
func TestSelectionStaysVisibleWhenScrolling(t *testing.T) {
	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m.width, m.height = 80, 24
	m.SetResults(prResults(30))
	for range 20 {
		m.moveSelection(1)
	}
	boxW, boxH := m.boxSize()
	if !strings.Contains(m.renderBox(boxW, boxH), "PR 21") {
		t.Fatalf("selected result not rendered:\n%s", ansi.Strip(m.renderBox(boxW, boxH)))
	}
}

func TestViewOverStatusLeavesStatusBarUndimmed(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	th := theme.Default()
	m := NewModel(nil)
	m.SetTheme(th)
	m.width, m.height = 60, 12

	status := th.HintKey.Render("Esc") + " close"
	body := strings.TrimSuffix(strings.Repeat(th.Header.Render(strings.Repeat("x", 60))+"\n", 11), "\n")
	lines := strings.Split(m.ViewOverStatus(body, status), "\n")
	if lines[0] == ansi.Strip(lines[0]) {
		t.Fatal("expected styled output; colour profile not applied")
	}
	if got := lines[len(lines)-1]; got != status {
		t.Errorf("status bar changed: %q, want %q", got, status)
	}
	if got, want := lines[0], theme.Backdrop(th, ansi.Strip(body[:strings.Index(body, "\n")])); got != want {
		t.Errorf("body row not dimmed: %q, want %q", got, want)
	}
}

func TestEmptyStates(t *testing.T) {
	m := NewModel(nil)
	m.width, m.height = 80, 24
	m.query, m.cursor = "zzz", 3
	m.SetResults(nil)
	assertContains(t, m.View(), "No matches for “zzz”")

	m = NewModel(nil)
	m.width, m.height = 80, 24
	assertContains(t, m.View(), "Search pull requests and repositories")
}

func TestHighlightMatch(t *testing.T) {
	base := lipgloss.NewStyle()
	mark := func(s string) string { return "[" + s + "]" }
	cases := []struct{ text, query, want string }{
		{"Fix login flow", "LOGIN", "Fix [login] flow"},
		{"Fix login flow", "", "Fix login flow"},
		{"Fix login flow", "nope", "Fix login flow"},
		{"日本 login", "login", "日本 [login]"},
		{"İstanbul login", "login", "İstanbul login"}, // lowercasing changes byte length: no highlight, no panic
	}
	for _, c := range cases {
		got := highlightMatchWith(c.text, c.query, func(s string) string { return base.Render(s) }, mark)
		if got != c.want {
			t.Errorf("highlightMatch(%q, %q) = %q, want %q", c.text, c.query, got, c.want)
		}
	}
}

// Backspace, Delete and the arrow keys move by character, so multi-byte
// input never leaves invalid UTF-8 in the query.
func TestQueryEditingIsRuneSafe(t *testing.T) {
	m := NewModel(&mockSearchService{})
	m.width, m.height = 80, 24
	press := func(k tea.KeyType) { m, _ = m.Update(tea.KeyMsg{Type: k}) }

	m, _ = m.Update(keyRuneMsg("é日"))
	press(tea.KeyLeft)
	m, _ = m.Update(keyRuneMsg("x"))
	if m.query != "éx日" {
		t.Fatalf("insert after Left: %q", m.query)
	}
	press(tea.KeyBackspace)
	if m.query != "é日" {
		t.Fatalf("Backspace: %q", m.query)
	}
	press(tea.KeyDelete)
	if m.query != "é" {
		t.Fatalf("Delete removes the character after the cursor: %q", m.query)
	}
	press(tea.KeyRight) // at the end: no-op
	press(tea.KeyBackspace)
	press(tea.KeyBackspace) // empty: no-op
	if m.query != "" || m.cursor != 0 {
		t.Fatalf("query %q cursor %d, want empty", m.query, m.cursor)
	}
}

// Terminals too small for the box render no box rather than a broken one.
func TestTinyTerminalHidesBox(t *testing.T) {
	for _, size := range [][2]int{{80, 4}, {3, 24}, {1, 1}} {
		m := NewModel(nil)
		m.SetTheme(theme.Default())
		m.width, m.height = size[0], size[1]
		m.SetResults(prResults(3))
		bg := strings.TrimSuffix(strings.Repeat(strings.Repeat(".", size[0])+"\n", size[1]), "\n")
		if got := m.ViewOverStatus(bg, ""); ansi.Strip(got) != bg {
			t.Errorf("%v: box drawn on a too-small terminal:\n%s", size, ansi.Strip(got))
		}
	}
}

// Repo results show the repo name and a "repository" tag; PRs from another
// repo name it in their metadata.
func TestResultMetadata(t *testing.T) {
	m := NewModel(nil)
	m.SetActiveRepo("org/x")
	m.width, m.height = 100, 24
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 1, Title: "Same repo", Author: "alice"},
		{Kind: domain.SearchResultPR, Repo: "org/y", Number: 2, Title: "Other repo", Author: "bob"},
		{Kind: domain.SearchResultRepo, Repo: "org/z"},
	})
	view := m.View()
	assertContains(t, view, "alice")
	assertContains(t, view, "org/y · bob")
	assertContains(t, view, "org/z")
	assertContains(t, view, "repository")
	if strings.Contains(view, "org/x · alice") {
		t.Error("active repo should not be repeated in PR metadata")
	}
}
