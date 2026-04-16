package dashboard

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/domain"
)

func TestRepoPanelRenderActiveAndCursor(t *testing.T) {
	t.Parallel()

	repos := makeRepos(5)
	m := NewRepoPanelModel(repos)
	m.SetActiveIndex(2)
	m.Cursor = 3
	m.SetRect(32, 8)

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) < 7 {
		t.Fatalf("expected at least 7 lines, got %d", len(lines))
	}
	// Lines 0=header, 1=blank spacing, 2=underline, 3=blank. Repos start at line 4.
	// ActiveIndex=2 → third repo → line 6
	// Cursor=3 → fourth repo → line 7
	if !strings.Contains(lines[6], "▶") {
		t.Fatalf("expected active marker on row 2, got %q", lines[6])
	}
	if !strings.Contains(lines[7], "▌") {
		t.Fatalf("expected cursor highlight on row 3, got %q", lines[7])
	}
}

func TestRepoPanelEnterSelectionIntent(t *testing.T) {
	t.Parallel()

	repos := makeRepos(5)
	m := NewRepoPanelModel(repos)
	m.Cursor = 3

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from enter")
	}
	msg := cmd()
	sel, ok := msg.(SelectRepoMsg)
	if !ok {
		t.Fatalf("expected SelectRepoMsg, got %T", msg)
	}
	if sel.Index != 3 {
		t.Fatalf("expected index 3, got %d", sel.Index)
	}
	if sel.Repo.FullName != "org/repo-3" {
		t.Fatalf("expected org/repo-3, got %s", sel.Repo.FullName)
	}
}

func TestRepoPanelScrollViewport(t *testing.T) {
	t.Parallel()

	repos := makeRepos(20)
	m := NewRepoPanelModel(repos)
	m.Cursor = 15
	m.SetRect(32, 10)

	if m.Scroll == 0 {
		t.Fatalf("expected scroll offset to advance, got %d", m.Scroll)
	}
	view := m.View()
	want := m.Repos[m.Cursor].FullName
	if !strings.Contains(view, want) {
		t.Fatalf("expected viewport to include %s, got %q", want, view)
	}
}

func makeRepos(n int) []domain.Repository {
	repos := make([]domain.Repository, 0, n)
	for i := 0; i < n; i++ {
		suffix := fmt.Sprintf("%d", i)
		repos = append(repos, domain.Repository{
			Owner:     "org",
			Name:      "repo-" + suffix,
			FullName:  "org/repo-" + suffix,
			LocalPath: "/tmp/repo-" + suffix,
		})
	}
	return repos
}

func TestRepoPanelVimNavigation(t *testing.T) {
	t.Parallel()

	repos := makeRepos(20)

	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	t.Run("gg goes to top", func(t *testing.T) {
		t.Parallel()
		m := NewRepoPanelModel(repos)
		m.SetRect(32, 10)
		m.Cursor = 15
		m.Update(key("g"))
		m.Update(key("g"))
		if m.Cursor != 0 {
			t.Fatalf("gg: expected cursor=0, got %d", m.Cursor)
		}
	})

	t.Run("single g does not jump", func(t *testing.T) {
		t.Parallel()
		m := NewRepoPanelModel(repos)
		m.SetRect(32, 10)
		m.Cursor = 5
		m.Update(key("g"))
		if m.Cursor != 5 {
			t.Fatalf("single g: expected cursor unchanged at 5, got %d", m.Cursor)
		}
	})

	t.Run("G goes to bottom", func(t *testing.T) {
		t.Parallel()
		m := NewRepoPanelModel(repos)
		m.SetRect(32, 10)
		m.Cursor = 0
		m.Update(key("G"))
		if m.Cursor != len(repos)-1 {
			t.Fatalf("G: expected cursor=%d, got %d", len(repos)-1, m.Cursor)
		}
	})

	t.Run("ctrl+d scrolls down half page", func(t *testing.T) {
		t.Parallel()
		m := NewRepoPanelModel(repos)
		m.SetRect(32, 14) // visibleCount = 14-4 = 10, half = 5
		m.Cursor = 0
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		if m.Cursor <= 0 {
			t.Fatalf("ctrl+d: expected cursor to advance, got %d", m.Cursor)
		}
	})

	t.Run("ctrl+u scrolls up half page", func(t *testing.T) {
		t.Parallel()
		m := NewRepoPanelModel(repos)
		m.SetRect(32, 14)
		m.Cursor = 15
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		if m.Cursor >= 15 {
			t.Fatalf("ctrl+u: expected cursor to move up, got %d", m.Cursor)
		}
	})

	t.Run("g then other key cancels gg", func(t *testing.T) {
		t.Parallel()
		m := NewRepoPanelModel(repos)
		m.SetRect(32, 10)
		m.Cursor = 5
		m.Update(key("g"))
		m.Update(key("j")) // cancels gg, moves down
		if m.Cursor != 6 {
			t.Fatalf("g+j: expected cursor=6, got %d", m.Cursor)
		}
	})
}
