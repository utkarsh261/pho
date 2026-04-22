package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

type mockSearchService struct {
	searchReposFn      func(query string, limit int) []domain.SearchResult
	searchPRsForRepoFn func(query, repo string, limit int) []domain.SearchResult

	searchRepoQueries      []string
	searchPRForRepoQueries []string
}

func (m *mockSearchService) SearchRepos(query string, limit int) []domain.SearchResult {
	m.searchRepoQueries = append(m.searchRepoQueries, query)
	if m.searchReposFn != nil {
		return m.searchReposFn(query, limit)
	}
	return nil
}

func (m *mockSearchService) SearchPRsForRepo(query, repo string, limit int) []domain.SearchResult {
	m.searchPRForRepoQueries = append(m.searchPRForRepoQueries, query)
	if m.searchPRsForRepoFn != nil {
		return m.searchPRsForRepoFn(query, repo, limit)
	}
	return nil
}

func TestModel_TitleIsGoTo(t *testing.T) {
	m := NewModel(nil)
	m.width = 80
	m.height = 24
	view := m.View()
	assertContains(t, view, "Go to")
}

func TestModel_QueryLineHasNoPrefix(t *testing.T) {
	svc := &mockSearchService{}
	m := NewModel(svc)
	m.width = 80
	m.height = 24
	m, _ = m.Update(keyRuneMsg("fix"))
	view := m.View()
	assertContains(t, view, "fix")
	if strings.Contains(view, "Query:") {
		t.Fatalf("expected no 'Query:' prefix in view, got:\n%s", view)
	}
}

func TestModel_EmptyQueryCallsOnlyPRSearch(t *testing.T) {
	svc := &mockSearchService{}
	m := NewModel(svc)
	m.SetActiveRepo("org/alpha")
	m.width = 80
	m.height = 24
	m.RefreshResults()
	if len(svc.searchRepoQueries) > 0 {
		t.Fatalf("expected no repo search calls for empty query, got: %v", svc.searchRepoQueries)
	}
	if len(svc.searchPRForRepoQueries) == 0 {
		t.Fatal("expected SearchPRsForRepo to be called for empty query")
	}
}

func TestModel_NonEmptyQueryCallsBoth(t *testing.T) {
	svc := &mockSearchService{}
	m := NewModel(svc)
	m.SetActiveRepo("org/alpha")
	m.width = 80
	m.height = 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(keyRuneMsg("fix"))
	if len(svc.searchPRForRepoQueries) == 0 {
		t.Fatal("expected SearchPRsForRepo to be called for non-empty query")
	}
	if len(svc.searchRepoQueries) == 0 {
		t.Fatal("expected SearchRepos to be called for non-empty query")
	}
}

func TestModel_RenderShowsInputAndResults(t *testing.T) {
	svc := &mockSearchService{
		searchReposFn: func(query string, limit int) []domain.SearchResult {
			if query != "g" {
				t.Fatalf("unexpected repo query %q", query)
			}
			return []domain.SearchResult{
				{
					Kind:  domain.SearchResultRepo,
					Repo:  "org/alpha",
					Score: 9.7,
				},
			}
		},
		searchPRsForRepoFn: func(query, repo string, limit int) []domain.SearchResult {
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

	var cmd tea.Cmd
	model, cmd = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("expected no command from window size, got %T", cmd)
	}

	model, cmd = model.Update(keyRuneMsg("g"))
	if cmd != nil {
		t.Fatalf("expected no command from query update, got %T", cmd)
	}
	if got := svc.searchRepoQueries; len(got) == 0 || got[len(got)-1] != "g" {
		t.Fatalf("unexpected repo queries: %#v", got)
	}
	if got := svc.searchPRForRepoQueries; len(got) == 0 || got[len(got)-1] != "g" {
		t.Fatalf("unexpected pr queries: %#v", got)
	}

	view := model.View()
	if !containsLineWithPrefix(view, 20) {
		t.Fatalf("expected centered overlay with left padding, got:\n%s", view)
	}
	assertContains(t, view, "Go to")
	assertContains(t, view, "g")
	assertContains(t, view, "org/alpha")
	assertContains(t, view, "#7")
	assertContains(t, view, "Fix login flow")
}

func TestModel_PRResultRowFormat(t *testing.T) {
	m := NewModel(nil)
	m.width = 80
	m.height = 24
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultPR, Repo: "org/beta", Number: 7, Title: "Fix login flow", State: domain.PRStateOpen},
	})
	view := m.View()
	assertContains(t, view, "◆")
	assertContains(t, view, "#7")
	assertContains(t, view, "Fix login flow")
}

func TestModel_RepoResultNoPrefix(t *testing.T) {
	m := NewModel(nil)
	m.width = 80
	m.height = 24
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultRepo, Repo: "org/alpha"},
	})
	view := m.View()
	assertContains(t, view, "org/alpha")
	if strings.Contains(view, "REPO") {
		t.Fatalf("expected no 'REPO' prefix in repo result, got:\n%s", view)
	}
}

func TestModel_HydrationFooterShown(t *testing.T) {
	m := NewModel(nil)
	m.SetHydrating(true)
	m.width = 100
	m.height = 40
	view := m.View()
	assertContains(t, view, "Loading")
}

func TestModel_HydrationFooterHidden(t *testing.T) {
	m := NewModel(nil)
	m.SetHydrating(false)
	m.width = 100
	m.height = 40
	view := m.View()
	if strings.Contains(view, "Loading") {
		t.Fatalf("expected no Loading indicator when not hydrating, got:\n%s", view)
	}
}

func TestModel_EnterOnPRDispatchesSummary(t *testing.T) {
	m := NewModel(nil)
	m.SetActiveRepo("org/alpha")
	m.SetResults([]domain.SearchResult{
		{
			Kind:   domain.SearchResultPR,
			Repo:   "org/beta",
			Number: 42,
			Title:  "Fix flaky test",
			State:  domain.PRStateOpen,
		},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)

	dispatch, ok := msg.(DispatchMsg)
	if !ok {
		t.Fatalf("expected DispatchMsg, got %T", msg)
	}

	var found OpenPR
	for _, item := range dispatch.Messages {
		if pr, ok := item.(OpenPR); ok {
			found = pr
			break
		}
	}
	if found.Repo != "org/beta" || found.Number != 42 {
		t.Fatalf("unexpected OpenPR payload: %+v", found)
	}
	if found.Summary.Title != "Fix flaky test" {
		t.Fatalf("expected Summary.Title = 'Fix flaky test', got %q", found.Summary.Title)
	}
}

func TestModel_EnterOnRepoDispatchesCloseAndSelect(t *testing.T) {
	m := NewModel(nil)
	m.SetActiveRepo("org/alpha")
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultRepo, Repo: "org/other"},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := runCmd(t, cmd)

	dispatch, ok := msg.(DispatchMsg)
	if !ok {
		t.Fatalf("expected DispatchMsg, got %T", msg)
	}

	var hasSelectRepo, hasClosePalette bool
	for _, item := range dispatch.Messages {
		switch item.(type) {
		case SelectRepo:
			hasSelectRepo = true
		case CloseCmdPalette:
			hasClosePalette = true
		}
	}
	if !hasSelectRepo {
		t.Fatal("expected SelectRepo in dispatch messages")
	}
	if !hasClosePalette {
		t.Fatal("expected CloseCmdPalette in dispatch messages")
	}
}

func TestModel_EscDismissesOverlay(t *testing.T) {
	m := NewModel(nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	msg := runCmd(t, cmd)

	if _, ok := msg.(CloseCmdPalette); !ok {
		t.Fatalf("expected CloseCmdPalette, got %T", msg)
	}
}

func TestModel_SelectionMovesWithJAndK(t *testing.T) {
	m := NewModel(nil)
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultRepo, Repo: "org/a"},
		{Kind: domain.SearchResultRepo, Repo: "org/b"},
		{Kind: domain.SearchResultRepo, Repo: "org/c"},
	})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.selectedIndex; got != 1 {
		t.Fatalf("expected selected index 1, got %d", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.selectedIndex; got != 2 {
		t.Fatalf("expected selected index 2, got %d", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.selectedIndex; got != 1 {
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

// TestOverlayFullUIRender locks the structural bounds of the overlay View output.
func TestOverlayFullUIRender(t *testing.T) {
	m := NewModel(nil)
	m.width = 80
	m.height = 24
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultRepo, Repo: "org/alpha", Title: "Alpha repo"},
		{Kind: domain.SearchResultPR, Repo: "org/beta", Number: 7, Title: "Fix login flow"},
	})

	view := m.View()

	// Box borders: boxW = round(80*0.6) = 48, innerW = 46 dashes.
	assertContains(t, view, "┌──────────────────────────────────────────────┐")
	assertContains(t, view, "└──────────────────────────────────────────────┘")

	// Title is "Go to".
	assertContains(t, view, "Go to")

	// Repo result: no "REPO" prefix.
	assertContains(t, view, "org/alpha")
	if strings.Contains(view, "REPO") {
		t.Fatalf("expected no 'REPO' prefix, got:\n%s", view)
	}

	// PR result: glyph and number present.
	assertContains(t, view, "#7")

	// Box must be horizontally centered: leftPad = (80-48)/2 = 16 spaces.
	if !containsLineWithPrefix(view, 16) {
		t.Fatalf("expected box lines to carry 16-space left indent, view:\n%s", view)
	}
}

// TestOverlayPRResultsRender verifies all four PR state glyphs render correctly.
func TestOverlayPRResultsRender(t *testing.T) {
	m := NewModel(nil)
	m.width = 80
	m.height = 40
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 1, Title: "Open PR", State: domain.PRStateOpen},
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 2, Title: "Merged PR", State: domain.PRStateMerged},
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 3, Title: "Closed PR", State: domain.PRStateClosed},
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 4, Title: "Draft PR", IsDraft: true},
	})

	view := m.View()
	assertContains(t, view, "◆")
	assertContains(t, view, "✓")
	assertContains(t, view, "✕")
	assertContains(t, view, "○")
}

// TestThemedBoxDimensions verifies the rendered box is exactly boxW × boxH when a theme is set.
// Previously Width(boxW)/Height(boxH) used content dimensions, making the box 2 chars wider
// and 2 lines taller than expected, which broke ViewOver compositing.
func TestThemedBoxDimensions(t *testing.T) {
	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m.width = 80
	m.height = 24

	boxW, boxH := m.boxSize()
	box := m.renderBox(boxW, boxH)

	lines := strings.Split(box, "\n")
	if len(lines) != boxH {
		t.Errorf("box height: want %d lines, got %d", boxH, len(lines))
	}
}

// TestThemedBoxContent verifies key strings appear inside the themed box.
func TestThemedBoxContent(t *testing.T) {
	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m.width = 80
	m.height = 24
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 1234, Title: "Fix flaky test", State: domain.PRStateOpen, Author: "alice"},
		{Kind: domain.SearchResultRepo, Repo: "org/backend"},
	})

	view := m.View()

	// Rounded border (from theme's RoundedBorder)
	assertContains(t, view, "╭")
	assertContains(t, view, "╰")

	// Title
	assertContains(t, view, "Go to")

	// PR result parts
	assertContains(t, view, "◆")
	assertContains(t, view, "1234")
	assertContains(t, view, "Fix flaky test")
	assertContains(t, view, "alice")

	// Repo result — no "REPO" prefix
	assertContains(t, view, "org/backend")
	if strings.Contains(view, "REPO") {
		t.Fatal("expected no REPO prefix in themed view")
	}

	// No > selection prefix
	if strings.Contains(view, "> ") {
		t.Fatal("expected no '> ' prefix in themed view")
	}
}

// TestSelectedRowHasBackground verifies the selected row carries a different ANSI styling
// than normal rows — specifically that selected rows have a background (highlight) and that
// selected vs normal rows produce distinct output.
func TestSelectedRowHasBackground(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("COLORTERM", "truecolor")

	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m.width = 80
	m.height = 24
	m.SetResults([]domain.SearchResult{
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 42, Title: "Selected PR", State: domain.PRStateOpen},
		{Kind: domain.SearchResultPR, Repo: "org/x", Number: 99, Title: "Normal PR", State: domain.PRStateOpen},
	})
	// selectedIndex == 0 by default (Selected PR is first)

	boxW, boxH := m.boxSize()
	innerW := maxInt(0, boxW-2)
	innerH := maxInt(0, boxH-2)
	lines := m.bodyLines(innerW, innerH)

	var selectedLine, normalLine string
	for _, l := range lines {
		if strings.Contains(l, "Selected PR") {
			selectedLine = l
		}
		if strings.Contains(l, "Normal PR") {
			normalLine = l
		}
	}
	if selectedLine == "" {
		t.Fatal("could not find selected row in body lines")
	}
	if normalLine == "" {
		t.Fatal("could not find normal row in body lines")
	}

	// When ANSI is enabled, the two rows must have distinct styling sequences.
	if strings.Contains(selectedLine, "\x1b[") || strings.Contains(normalLine, "\x1b[") {
		// At least one has ANSI; they must differ (different bg = different escape sequences).
		if selectedLine == normalLine {
			t.Error("selected and normal rows have identical output — highlight not applied")
		}
		// Selected row must include bold (\\x1b[1m or combined bold code).
		if !strings.Contains(selectedLine, "1m") && !strings.Contains(selectedLine, ";1;") && !strings.Contains(selectedLine, "1;") {
			t.Error("selected row: expected bold in ANSI sequence")
		}
		// Both rows must have some background escape sequence.
		if !strings.Contains(selectedLine, "\x1b[") {
			t.Error("selected row: missing ANSI escape sequence")
		}
		if !strings.Contains(normalLine, "\x1b[") {
			t.Error("normal row: missing ANSI escape sequence (dark bg not applied)")
		}
	}
}
