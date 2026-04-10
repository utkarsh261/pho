package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utk/git-term/internal/domain"
)

type mockSearchService struct {
	searchReposFn func(query string, limit int) []domain.SearchResult
	searchPRsFn   func(query string, limit int) []domain.SearchResult

	searchRepoQueries []string
	searchPRQueries   []string
}

func (m *mockSearchService) SearchRepos(query string, limit int) []domain.SearchResult {
	m.searchRepoQueries = append(m.searchRepoQueries, query)
	if m.searchReposFn != nil {
		return m.searchReposFn(query, limit)
	}
	return nil
}

func (m *mockSearchService) SearchPRs(query string, limit int) []domain.SearchResult {
	m.searchPRQueries = append(m.searchPRQueries, query)
	if m.searchPRsFn != nil {
		return m.searchPRsFn(query, limit)
	}
	return nil
}

func TestModel_RenderShowsInputAndResults(t *testing.T) {
	svc := &mockSearchService{
		searchReposFn: func(query string, limit int) []domain.SearchResult {
			if query != "g" {
				t.Fatalf("unexpected repo query %q", query)
			}
			return []domain.SearchResult{
				{
					Kind:   domain.SearchResultRepo,
					Repo:   "org/alpha",
					Title:  "Alpha repository",
					Score:  9.7,
					Branch: "main",
				},
			}
		},
		searchPRsFn: func(query string, limit int) []domain.SearchResult {
			if query != "g" {
				t.Fatalf("unexpected pr query %q", query)
			}
			return []domain.SearchResult{
				{
					Kind:   domain.SearchResultPR,
					Repo:   "org/beta",
					Number: 7,
					Title:  "Fix login flow",
					Score:  10,
				},
			}
		},
	}

	model := NewModel(svc)
	model.SetActiveRepo("org/alpha")
	model.SetRepoHydrationStats(5, 2)

	var cmd tea.Cmd
	model, cmd = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("expected no command from window size, got %T", cmd)
	}

	model, cmd = model.Update(keyRuneMsg("g"))
	if cmd != nil {
		t.Fatalf("expected no command from query update, got %T", cmd)
	}
	if got := svc.searchRepoQueries; len(got) != 1 || got[0] != "g" {
		t.Fatalf("unexpected repo queries: %#v", got)
	}
	if got := svc.searchPRQueries; len(got) != 1 || got[0] != "g" {
		t.Fatalf("unexpected pr queries: %#v", got)
	}

	view := model.View()
	if !containsLineWithPrefix(view, 20) {
		t.Fatalf("expected centered overlay with left padding, got:\n%s", view)
	}
	assertContains(t, view, "Command Palette")
	assertContains(t, view, "Query: g")
	assertContains(t, view, "REPO org/alpha")
	assertContains(t, view, "PR #7 org/beta - Fix login flow")
}

func TestModel_EnterDispatchesSelectRepoThenOpenPR(t *testing.T) {
	model := NewModel(nil)
	model.SetActiveRepo("org/alpha")
	model.SetResults([]domain.SearchResult{
		{
			Kind:   domain.SearchResultPR,
			Repo:   "org/beta",
			Number: 42,
			Title:  "Fix flaky test",
		},
	})

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)

	dispatch, ok := msg.(DispatchMsg)
	if !ok {
		t.Fatalf("expected DispatchMsg, got %T", msg)
	}
	if len(dispatch.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(dispatch.Messages))
	}

	selectRepo, ok := dispatch.Messages[0].(SelectRepo)
	if !ok {
		t.Fatalf("expected first message SelectRepo, got %T", dispatch.Messages[0])
	}
	if selectRepo.Repo != "org/beta" {
		t.Fatalf("unexpected repo %q", selectRepo.Repo)
	}

	openPR, ok := dispatch.Messages[1].(OpenPR)
	if !ok {
		t.Fatalf("expected second message OpenPR, got %T", dispatch.Messages[1])
	}
	if openPR.Repo != "org/beta" || openPR.Number != 42 {
		t.Fatalf("unexpected open PR payload: %+v", openPR)
	}
}

func TestModel_EscDismissesOverlay(t *testing.T) {
	model := NewModel(nil)

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	msg := runCmd(t, cmd)

	if _, ok := msg.(CloseCmdPalette); !ok {
		t.Fatalf("expected CloseCmdPalette, got %T", msg)
	}
}

func TestModel_FooterHintForUnhydratedRepos(t *testing.T) {
	model := NewModel(nil)
	model.SetRepoHydrationStats(5, 2)
	model.width = 100
	model.height = 40

	view := model.View()
	assertContains(t, view, "3 of 5 repos still hydrating; results may be incomplete")
}

func TestModel_SelectionMovesWithJAndK(t *testing.T) {
	model := NewModel(nil)
	model.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultRepo, Repo: "org/a"},
		{Kind: domain.SearchResultRepo, Repo: "org/b"},
		{Kind: domain.SearchResultRepo, Repo: "org/c"},
	})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := model.selectedIndex; got != 1 {
		t.Fatalf("expected selected index 1, got %d", got)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := model.selectedIndex; got != 2 {
		t.Fatalf("expected selected index 2, got %d", got)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := model.selectedIndex; got != 1 {
		t.Fatalf("expected selected index 1, got %d", got)
	}
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}
	return cmd()
}

func keyRuneMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q in output, got:\n%s", want, got)
	}
}

func containsLineWithPrefix(text string, spaces int) bool {
	prefix := strings.Repeat(" ", spaces)
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return strings.HasPrefix(line, prefix)
	}
	return false
}
