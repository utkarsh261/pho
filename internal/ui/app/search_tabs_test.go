package app

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

var teamTab = domain.SearchTab{ID: domain.CustomTabID("Team"), Name: "Team", Query: "author:alice"}

func setupSearchTabs(t *testing.T, defaultTab domain.DashboardTab, repos ...domain.Repository) (*Model, *stubDashboardService) {
	t.Helper()
	dash := map[string]domain.DashboardSnapshot{}
	found := map[string]map[domain.DashboardTab][]domain.PullRequestSummary{}
	for i, repo := range repos {
		dash[repo.FullName] = dashboardSnapshot(repo, pr(repo.FullName, 1, "mine"))
		found[repo.FullName] = map[domain.DashboardTab][]domain.PullRequestSummary{
			teamTab.ID: {pr(repo.FullName, 100+i, "alice's")},
		}
	}
	m := newTestModelWith(repos, dash, func(d *Dependencies) {
		d.SearchTabs = []domain.SearchTab{teamTab}
		d.DefaultTab = defaultTab
	})
	svc := m.deps.Dashboard.(*stubDashboardService)
	svc.searchTabs = found
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	runSearchTabCmds(m, upd(m, cmdsReposDiscovered(repos)))
	return m, svc
}

// runSearchTabCmds runs cmd and its batches, feeding search-tab results
// back into m; other messages are dropped.
func runSearchTabCmds(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		switch msg := msg.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				runSearchTabCmds(m, c)
			}
		case cmds.SearchTabLoaded:
			runSearchTabCmds(m, upd(m, msg))
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func upd(m *Model, msg tea.Msg) tea.Cmd {
	_, cmd := m.Update(msg)
	return cmd
}

func switchTab(m *Model, tab domain.DashboardTab) {
	runSearchTabCmds(m, upd(m, dashboard.ChangeTabMsg{Tab: tab}))
}

func TestSearchTabLoadsOnlyWhenShown(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/web")
	m, svc := setupSearchTabs(t, "", repo)
	if len(svc.loadSearchTabCalls) != 0 {
		t.Fatalf("startup loaded custom tabs: %+v", svc.loadSearchTabCalls)
	}

	switchTab(m, teamTab.ID)
	if len(svc.loadSearchTabCalls) != 1 || svc.loadSearchTabCalls[0].force {
		t.Fatalf("calls after switch = %+v, want one cached load", svc.loadSearchTabCalls)
	}
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{100}) {
		t.Fatalf("Team = %v, want [100]", got)
	}
	if !strings.Contains(m.prList.View(), "alice's") {
		t.Fatalf("list doesn't show the tab's PR:\n%s", m.prList.View())
	}

	// Switching away loads nothing; back reads through the cache.
	switchTab(m, domain.TabAll)
	switchTab(m, teamTab.ID)
	if len(svc.loadSearchTabCalls) != 2 || svc.loadSearchTabCalls[1].force {
		t.Fatalf("calls = %+v", svc.loadSearchTabCalls)
	}
}

func TestSearchTabRefresh(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/web")
	m, svc := setupSearchTabs(t, "", repo)

	runSearchTabCmds(m, m.refreshSelectedRepo(true))
	if len(svc.loadSearchTabCalls) != 0 {
		t.Fatalf("refresh on a built-in tab loaded a custom one: %+v", svc.loadSearchTabCalls)
	}
	switchTab(m, teamTab.ID)
	runSearchTabCmds(m, m.refreshSelectedRepo(true))
	if n := len(svc.loadSearchTabCalls); n != 2 || !svc.loadSearchTabCalls[1].force {
		t.Fatalf("calls = %+v, want a forced reload last", svc.loadSearchTabCalls)
	}
}

func TestSearchTabDefaultLoadsAtStartup(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/web")
	m, svc := setupSearchTabs(t, teamTab.ID, repo)
	if m.prList.Active != teamTab.ID || m.state.Dashboard.ActiveTab != teamTab.ID {
		t.Fatalf("active = %q / %q", m.prList.Active, m.state.Dashboard.ActiveTab)
	}
	if len(svc.loadSearchTabCalls) != 1 {
		t.Fatalf("calls = %+v, want the default tab loaded", svc.loadSearchTabCalls)
	}
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{100}) {
		t.Fatalf("Team = %v", got)
	}
}

func TestSearchTabRepoSwitch(t *testing.T) {
	t.Parallel()
	a, b := testutil.Repo("acme/a"), testutil.Repo("acme/b")
	m, _ := setupSearchTabs(t, teamTab.ID, a, b)
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{100}) {
		t.Fatalf("Team on a = %v", got)
	}

	// Selecting b clears a's results before b's arrive.
	cmd := m.selectRepoAt(1, b, false)
	if got := tabNumbers(m, teamTab.ID); len(got) != 0 {
		t.Fatalf("Team shows %v from the previous repo", got)
	}
	if !strings.Contains(m.prList.View(), "Loading…") {
		t.Fatalf("no loading state:\n%s", m.prList.View())
	}
	runSearchTabCmds(m, cmd)
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{101}) {
		t.Fatalf("Team on b = %v", got)
	}

	// A late result for a is dropped.
	_, _ = m.Update(cmds.SearchTabLoaded{Repo: a.FullName, Tab: teamTab.ID, Snapshot: domain.SearchTabSnapshot{
		Repo: a, PRs: []domain.PullRequestSummary{pr(a.FullName, 7, "late")}, FetchedAt: fixedNow(),
	}})
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{101}) {
		t.Fatalf("late result replaced b's: %v", got)
	}
}

func TestSearchTabError(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/web")
	m, svc := setupSearchTabs(t, "", repo)
	svc.searchTabErr = errors.New("ERROR: invalid qualifier\ndetails")
	switchTab(m, teamTab.ID)
	view := m.prList.View()
	if !strings.Contains(view, "Couldn't load: ERROR: invalid qualifier ·") {
		t.Fatalf("no error in list:\n%s", view)
	}
	if len(m.state.Jobs.InFlight) != 0 {
		t.Fatalf("in-flight jobs left: %v", m.state.Jobs.InFlight)
	}
}

// A tab loaded before the stacks are known is regrouped once they are.
func TestSearchTabRegroupsWhenStacksChange(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/web")
	m, svc := setupSearchTabs(t, "", repo)
	mk := func(n int, head, base string) domain.PullRequestSummary {
		p := pr(repo.FullName, n, "layer "+head)
		p.HeadRefName, p.BaseRefName = head, base
		return p
	}
	found := []domain.PullRequestSummary{mk(3, "c", "b"), mk(9, "x", "main"), mk(2, "b", "main")}
	svc.searchTabs[repo.FullName][teamTab.ID] = found
	m.currentDashboard = domain.DashboardSnapshot{}
	m.rebuildStacks(nil)
	switchTab(m, teamTab.ID)
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{3, 9, 2}) {
		t.Fatalf("before stacks: %v", got)
	}

	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, found...), false, nil))
	if got := tabNumbers(m, teamTab.ID); !slices.Equal(got, []int{3, 2, 9}) {
		t.Errorf("after stacks: %v, want the stack together: [3 2 9]", got)
	}
}

// Reloading a tab keeps the cursor on the same PR when new ones arrive.
func TestSearchTabReloadKeepsSelection(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/web")
	m, svc := setupSearchTabs(t, "", repo)
	svc.searchTabs[repo.FullName][teamTab.ID] = []domain.PullRequestSummary{pr(repo.FullName, 50, "a"), pr(repo.FullName, 40, "b")}
	switchTab(m, teamTab.ID)
	if !m.prList.SelectNumber(40) {
		t.Fatal("#40 not listed")
	}

	svc.searchTabs[repo.FullName][teamTab.ID] = []domain.PullRequestSummary{pr(repo.FullName, 60, "new"), pr(repo.FullName, 50, "a"), pr(repo.FullName, 40, "b")}
	runSearchTabCmds(m, m.refreshSelectedRepo(true))
	if sel, ok := m.currentSelectedPR(); !ok || sel.Number != 40 {
		t.Fatalf("selected %+v after reload, want #40", sel)
	}
}
