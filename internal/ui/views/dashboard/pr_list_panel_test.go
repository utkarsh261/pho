package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func TestPRListPanelRenderTabsAndRows(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	m.SetTabSnapshot(domain.TabMyPRs, []domain.PullRequestSummary{
		makePR(1, "Fix login", "feature/login"),
		makePR(2, "Add tests", "feature/tests"),
		makePR(3, "Refine UI", "feature/ui"),
	}, 3, false)
	m.SetTabSnapshot(domain.TabNeedsReview, []domain.PullRequestSummary{
		makePR(10, "Needs review", "review/me"),
	}, 1, false)
	m.SetTabSnapshot(domain.TabInvolving, nil, 0, false)
	m.SetTabSnapshot(domain.TabRecent, nil, 0, false)
	m.SetActiveTab(domain.TabMyPRs)
	m.SetRect(80, 14)

	view := m.View()
	if !strings.Contains(view, "My PRs(3)") || !strings.Contains(view, "Needs Review(1)") {
		t.Fatalf("expected tab bar counts, got %q", view)
	}
	if strings.Count(view, "#1") != 1 || strings.Count(view, "#2") != 1 || strings.Count(view, "#3") != 1 {
		t.Fatalf("expected three PR rows, got %q", view)
	}
	if !strings.Contains(view, "feature/login") || !strings.Contains(view, "feature/tests") {
		t.Fatalf("expected branch rows, got %q", view)
	}
}

func TestPRListPanelTabSwitchIntent(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	m.SetTabSnapshot(domain.TabMyPRs, []domain.PullRequestSummary{makePR(1, "One", "branch")}, 1, false)
	m.SetTabSnapshot(domain.TabNeedsReview, []domain.PullRequestSummary{makePR(2, "Two", "branch")}, 1, false)
	m.SetActiveTab(domain.TabMyPRs)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("expected tab switch command")
	}
	msg := cmd()
	chg, ok := msg.(ChangeTabMsg)
	if !ok {
		t.Fatalf("expected ChangeTabMsg, got %T", msg)
	}
	if chg.Tab != domain.TabNeedsReview {
		t.Fatalf("expected next tab needs_review, got %s", chg.Tab)
	}
}

func TestPRListPanelTruncationFooter(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	prs := make([]domain.PullRequestSummary, 0, 100)
	for i := 0; i < 100; i++ {
		prs = append(prs, makePR(i+1, fmt.Sprintf("PR %d", i+1), "branch"))
	}
	m.SetTabSnapshot(domain.TabMyPRs, prs[:3], 234, true)
	m.SetTabScanned(domain.TabMyPRs, 100)
	m.SetTabSnapshot(domain.TabAll, prs, 234, true)
	m.SetActiveTab(domain.TabMyPRs)
	m.SetRect(60, 12)

	// A filtered tab must not claim to show N of the repo's open PRs.
	if view := m.View(); !strings.Contains(view, "3 from newest 100 of 234 open") {
		t.Fatalf("expected filtered-tab footer, got %q", view)
	}

	m.SetActiveTab(domain.TabAll)
	cases := []struct {
		paging PageStatus
		want   string
	}{
		{PageStatus{}, "100 of 234 open · ↓ for more"},
		{PageStatus{Loading: true}, "100 of 234 open · loading…"},
		{PageStatus{Failed: true}, "couldn't load more · R to retry"},
		{PageStatus{Capped: true}, "limit reached"},
	}
	for _, tc := range cases {
		m.Paging = tc.paging
		if view := m.View(); !strings.Contains(view, tc.want) {
			t.Fatalf("paging %+v: expected %q in footer, got %q", tc.paging, tc.want, view)
		}
	}
}

func TestPRListPanelTabBarFitsWidth(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	m.SetTheme(theme.Default())
	m.SetTabSnapshot(domain.TabMyPRs, make([]domain.PullRequestSummary, 3), 3, false)
	m.SetTabSnapshot(domain.TabNeedsReview, make([]domain.PullRequestSummary, 12), 12, false)
	m.SetTabSnapshot(domain.TabInvolving, make([]domain.PullRequestSummary, 5), 5, false)
	m.SetTabSnapshot(domain.TabAll, make([]domain.PullRequestSummary, 100), 342, true)
	m.SetTabSnapshot(domain.TabRecent, make([]domain.PullRequestSummary, 8), 8, false)
	m.SetActiveTab(domain.TabAll)

	cases := []struct {
		width int
		want  []string
		not   []string
	}{
		{80, []string{"My PRs", "Needs Review", "All 342", "Recent"}, nil},
		{56, []string{"Mine", "Review", "Involved", "All 342", "Recent 8"}, []string{"Needs Review"}},
		{46, []string{"Mine", "All 342", "Recent"}, []string{"Recent 8"}},
		{30, []string{"All 342", "4/5"}, []string{"Mine"}},
	}
	for _, tc := range cases {
		m.SetRect(tc.width, 12)
		bar := m.renderTabBarThemed()
		if w := lipgloss.Width(bar); w > tc.width {
			t.Fatalf("width %d: tab bar is %d wide: %q", tc.width, w, bar)
		}
		plain := ansi.Strip(bar)
		for _, s := range tc.want {
			if !strings.Contains(plain, s) {
				t.Fatalf("width %d: expected %q in %q", tc.width, s, plain)
			}
		}
		for _, s := range tc.not {
			if strings.Contains(plain, s) {
				t.Fatalf("width %d: did not expect %q in %q", tc.width, s, plain)
			}
		}
	}
}

func makePR(number int, title, branch string) domain.PullRequestSummary {
	return domain.PullRequestSummary{
		Repo:           "org/repo",
		Number:         number,
		Title:          title,
		Author:         "alice",
		State:          domain.PRStateOpen,
		CIStatus:       domain.CIStatusSuccess,
		ReviewDecision: domain.ReviewDecisionApproved,
		HeadRefName:    branch,
		CreatedAt:      time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 9, 13, 0, 0, 0, time.UTC),
	}
}

func TestPRListPanelVimNavigation(t *testing.T) {
	t.Parallel()

	prs := make([]domain.PullRequestSummary, 15)
	for i := range prs {
		prs[i] = makePR(i+1, fmt.Sprintf("PR %d", i+1), "branch")
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	newPanel := func() *PRListPanelModel {
		m := NewPRListPanelModel()
		m.SetTabSnapshot(domain.TabMyPRs, prs, len(prs), false)
		m.SetActiveTab(domain.TabMyPRs)
		m.SetRect(80, 20)
		return m
	}

	t.Run("gg goes to top", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Cursor = 10
		m.Update(key("g"))
		m.Update(key("g"))
		if m.Cursor != 0 {
			t.Fatalf("gg: expected cursor=0, got %d", m.Cursor)
		}
	})

	t.Run("single g does not jump", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Cursor = 5
		m.Update(key("g"))
		if m.Cursor != 5 {
			t.Fatalf("single g: expected cursor unchanged at 5, got %d", m.Cursor)
		}
	})

	t.Run("G goes to bottom", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Cursor = 0
		m.Update(key("G"))
		if m.Cursor != len(prs)-1 {
			t.Fatalf("G: expected cursor=%d, got %d", len(prs)-1, m.Cursor)
		}
	})

	t.Run("ctrl+d advances cursor", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Cursor = 0
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		if m.Cursor <= 0 {
			t.Fatalf("ctrl+d: expected cursor to advance, got %d", m.Cursor)
		}
	})

	t.Run("ctrl+u retreats cursor", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Cursor = 10
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		if m.Cursor >= 10 {
			t.Fatalf("ctrl+u: expected cursor to retreat, got %d", m.Cursor)
		}
	})

	t.Run("gg emits select cmd", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Cursor = 10
		m.Update(key("g"))
		_, cmd := m.Update(key("g"))
		if cmd == nil {
			t.Fatal("gg: expected SelectPR command, got nil")
		}
	})
}

func TestPRListPanelCustomTabs(t *testing.T) {
	t.Parallel()

	custom := []domain.SearchTab{
		{ID: domain.CustomTabID("Team"), Name: "Team"},
		{ID: domain.CustomTabID("Infrastructure"), Name: "Infrastructure"},
		{ID: domain.CustomTabID("My merged"), Name: "My merged", Short: "Merged"},
		{ID: domain.CustomTabID("Docs"), Name: "Docs"},
		{ID: domain.CustomTabID("Bots"), Name: "Bots"},
	}
	m := NewPRListPanelModel()
	m.SetTheme(theme.Default())
	m.SetCustomTabs(custom)
	m.SetTabSnapshot(custom[0].ID, make([]domain.PullRequestSummary, 4), 4, false)
	m.SetActiveTab(custom[0].ID)

	for _, tc := range []struct {
		width     int
		want, not []string
	}{
		{200, []string{"My PRs", "Infrastructure", "My merged", "Team 4", "Bots"}, nil},
		{100, []string{"Mine", "Infrast…", "Merged", "Team 4", "Bots"}, []string{"Infrastructure", "My merged"}},
		{30, []string{"Team 4", "6/10"}, []string{"Mine"}},
	} {
		m.SetRect(tc.width, 12)
		bar := m.renderTabBarThemed()
		if w := lipgloss.Width(bar); w > tc.width {
			t.Fatalf("width %d: tab bar is %d wide", tc.width, w)
		}
		plain := ansi.Strip(bar)
		for _, s := range tc.want {
			if !strings.Contains(plain, s) {
				t.Errorf("width %d: expected %q in %q", tc.width, s, plain)
			}
		}
		for _, s := range tc.not {
			if strings.Contains(plain, s) {
				t.Errorf("width %d: did not expect %q in %q", tc.width, s, plain)
			}
		}
	}

	// h/l walk from the built-ins into the custom tabs and wrap around.
	m.SetActiveTab(domain.TabRecent)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.Active != custom[0].ID {
		t.Fatalf("l from Recent = %q", m.Active)
	}
	m.SetActiveTab(custom[4].ID)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.Active != domain.TabMyPRs {
		t.Fatalf("l from the last tab = %q, want wrap to My PRs", m.Active)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.Active != custom[4].ID {
		t.Fatalf("h from My PRs = %q", m.Active)
	}
}

func TestPRListPanelCustomTabFooterAndStates(t *testing.T) {
	t.Parallel()

	tab := domain.CustomTabID("Team")
	m := NewPRListPanelModel()
	m.SetCustomTabs([]domain.SearchTab{{ID: tab, Name: "Team"}})
	m.SetRect(80, 20)
	m.SetActiveTab(tab)

	m.SetTabStatus(tab, true, "")
	if !strings.Contains(m.View(), "Loading…") {
		t.Fatalf("no loading state:\n%s", m.View())
	}
	m.SetTabStatus(tab, false, "bad query")
	if !strings.Contains(m.View(), "Couldn't load: bad query · R to retry") {
		t.Fatalf("no error state:\n%s", m.View())
	}

	m.SetTabSnapshot(tab, []domain.PullRequestSummary{makePR(1, "a", "x")}, 140, true)
	if got := m.footerLine(); got != "Showing 1 of 140 PRs" {
		t.Fatalf("footer = %q", got)
	}
}

// A long configured short label is capped, and the one-tab fallback always
// fits the panel.
func TestPRListPanelLongShortLabelFits(t *testing.T) {
	t.Parallel()

	tab := domain.SearchTab{ID: domain.CustomTabID("RC"), Name: "Release candidates", Short: "release-candidates"}
	m := NewPRListPanelModel()
	m.SetTheme(theme.Default())
	m.SetCustomTabs([]domain.SearchTab{tab})
	m.SetTabSnapshot(tab.ID, make([]domain.PullRequestSummary, 12), 12, false)
	m.SetActiveTab(tab.ID)

	m.SetRect(80, 12)
	if plain := ansi.Strip(m.renderTabBarThemed()); !strings.Contains(plain, "release…") || strings.Contains(plain, "release-candidates") {
		t.Errorf("short tier = %q, want the label cut to release…", plain)
	}
	for _, width := range []int{20, 14, 10} {
		m.SetRect(width, 12)
		bar := m.renderTabBarThemed()
		if w := lipgloss.Width(bar); w > width {
			t.Errorf("width %d: tab bar is %d wide: %q", width, w, ansi.Strip(bar))
		}
		if !strings.Contains(ansi.Strip(bar), "12") {
			t.Errorf("width %d: count lost: %q", width, ansi.Strip(bar))
		}
	}
}
