package app

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/components/stackmap"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

func setupStack(t *testing.T) *Model {
	t.Helper()
	repo := testutil.Repo("acme/stack")
	mk := func(n int, head, base string) domain.PullRequestSummary {
		p := pr(repo.FullName, n, "layer "+head)
		p.HeadRefName, p.BaseRefName = head, base
		return p
	}
	// Recency order scatters the stack: #2, an unrelated #9, #1, #3.
	prs := []domain.PullRequestSummary{mk(2, "b", "a"), mk(9, "x", "main"), mk(1, "a", "main"), mk(3, "c", "b")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)
	m.focus = domain.FocusPRListPanel
	m.syncStatus()
	return m
}

func tabNumbers(m *Model, tab domain.DashboardTab) []int {
	var out []int
	for _, p := range m.state.Dashboard.PRsByTab[tab] {
		out = append(out, p.Number)
	}
	return out
}

func TestDashboardGroupsStacks(t *testing.T) {
	t.Parallel()
	m := setupStack(t)
	for _, tab := range []domain.DashboardTab{domain.TabMyPRs, domain.TabAll} {
		if got := tabNumbers(m, tab); !slices.Equal(got, []int{3, 2, 1, 9}) {
			t.Errorf("%s = %v, want the stack together top-first: [3 2 1 9]", tab, got)
		}
	}
	if !strings.Contains(m.status.View(), "S: Stack") {
		t.Error("status bar should offer S on a stacked PR")
	}

	plain := setupStack(t)
	plain.deps.NoStackGrouping = true
	plain.rebuildDashboardTabs()
	if got := tabNumbers(plain, domain.TabAll); !slices.Equal(got, []int{2, 9, 1, 3}) {
		t.Errorf("ungrouped = %v", got)
	}
}

func TestInvolvingRegroupsEveryTab(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/stack")
	mk := func(n int, head, base string) domain.PullRequestSummary {
		p := pr(repo.FullName, n, "layer "+head)
		p.HeadRefName, p.BaseRefName = head, base
		return p
	}
	// #2, the middle of 1 <- 2 <- 3, is only known to the Involving search.
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{mk(3, "c", "b"), mk(9, "x", "main"), mk(1, "a", "main")})
	if got := tabNumbers(m, domain.TabAll); !slices.Equal(got, []int{3, 9, 1}) {
		t.Fatalf("before Involving: %v", got)
	}
	_, _ = m.Update(cmds.InvolvingLoaded{Repo: repo.FullName, Snapshot: domain.InvolvingSnapshot{
		Repo: repo, PRs: []domain.PullRequestSummary{mk(2, "b", "a")}, TotalCount: 1, FetchedAt: fixedNow(),
	}})
	if got := tabNumbers(m, domain.TabAll); !slices.Equal(got, []int{3, 1, 9}) {
		t.Errorf("All after Involving = %v, want the completed stack together: [3 1 9]", got)
	}
	if got := tabNumbers(m, domain.TabInvolving); !slices.Equal(got, []int{2}) {
		t.Errorf("Involving = %v", got)
	}
}

func TestInvolvingWithoutRepoNameStillFillsTab(t *testing.T) {
	t.Parallel()
	m := setupStack(t)
	repo := m.selectedRepoName()
	// A result for the selected repo whose snapshot carries no repo name.
	_, _ = m.Update(cmds.InvolvingLoaded{Repo: repo, Snapshot: domain.InvolvingSnapshot{
		PRs: []domain.PullRequestSummary{pr(repo, 9, "x")}, TotalCount: 1, FetchedAt: fixedNow(),
	}})
	if got := tabNumbers(m, domain.TabInvolving); !slices.Equal(got, []int{9}) {
		t.Errorf("Involving = %v, want [9]", got)
	}
}

func stackKey(s string) tea.KeyMsg {
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestStackHopKeepsTabAndCursor(t *testing.T) {
	t.Parallel()
	m := setupStack(t)
	m.prList.SelectNumber(1)
	_, cmd := m.Update(stackKey("enter"))
	drainCmds(m, cmd)
	if m.currentView() != domain.PrimaryViewPRDetail || m.prDetail.Summary.Number != 1 {
		t.Fatalf("expected #1 open, view %s", m.currentView())
	}
	if m.prDetail.Stack == nil {
		t.Fatal("detail should know its stack")
	}
	_, cmd = m.Update(stackKey("2"))
	drainCmds(m, cmd)

	_, cmd = m.Update(stackKey("}"))
	drainCmds(m, cmd)
	_, cmd = m.Update(stackKey("}"))
	drainCmds(m, cmd)
	if m.prDetail.Summary.Number != 3 {
		t.Fatalf("after two hops up, open PR = #%d, want #3", m.prDetail.Summary.Number)
	}
	if m.prDetail.ActiveTab() != prdetail.TabDiff || m.prDetail.PanelFocus() != prdetail.FocusContent {
		t.Errorf("hop should keep the Diff tab and content focus, got %v / %v", m.prDetail.ActiveTab(), m.prDetail.PanelFocus())
	}
	if len(m.viewStack) != 2 {
		t.Errorf("hops should replace the detail view, stack = %v", m.viewStack)
	}

	_, cmd = m.Update(stackKey("q"))
	drainCmds(m, cmd)
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("esc should return to the dashboard, got %s", m.currentView())
	}
	if sel, _ := m.currentSelectedPR(); sel.Number != 3 {
		t.Errorf("cursor on #%d, want #3 (the last PR read)", sel.Number)
	}
}

func TestStackPickerOpensMember(t *testing.T) {
	t.Parallel()
	m := setupStack(t)
	m.prList.SelectNumber(2)
	_, _ = m.Update(stackKey("S"))
	if !m.stackPicker.Visible {
		t.Fatal("S should open the stack picker")
	}
	view := m.View()
	for _, want := range []string{"Stack · 3 PRs onto main", "#3", "#1", "╰ main"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker missing %q", want)
		}
	}
	_, _ = m.Update(stackKey("j")) // down to #1
	_, cmd := m.Update(stackKey("enter"))
	drainCmds(m, cmd)
	if m.stackPicker.Visible || m.currentView() != domain.PrimaryViewPRDetail || m.prDetail.Summary.Number != 1 {
		t.Fatalf("enter should open #1; view %s", m.currentView())
	}

	// On an unstacked PR, S does nothing.
	m = setupStack(t)
	m.prList.SelectNumber(9)
	_, _ = m.Update(stackKey("S"))
	if m.stackPicker.Visible {
		t.Error("S on an unstacked PR should not open the picker")
	}
}

func TestPreviewKeepsStackMapAcrossTabs(t *testing.T) {
	t.Parallel()
	m := setupStack(t)
	// The stub echoes the host-qualified repo; real previews carry owner/name.
	svc := m.deps.Dashboard.(*stubDashboardService)
	svc.previewByPR = map[string]domain.PRPreviewSnapshot{
		previewKey("github.com/acme/stack", 3): {Repo: "acme/stack", Number: 3, Title: "layer c"},
	}
	_, cmd := m.Update(dashboard.ChangeTabMsg{Tab: domain.TabAll})
	drainCmds(m, cmd)
	if sel, _ := m.currentSelectedPR(); sel.Number != 3 {
		t.Fatalf("cursor on #%d, want #3", sel.Number)
	}
	header := stackmap.Header(m.stacks.Lookup("acme/stack", 3))
	if !strings.Contains(m.preview.View(), header) {
		t.Errorf("preview lost the stack map (%q) after switching tabs:\n%s", header, m.preview.View())
	}
}
