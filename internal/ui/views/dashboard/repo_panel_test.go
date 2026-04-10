package dashboard

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
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
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[3], "▶") {
		t.Fatalf("expected active marker on row 2, got %q", lines[3])
	}
	if !strings.Contains(lines[4], "▌") {
		t.Fatalf("expected cursor highlight on row 3, got %q", lines[4])
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
