package app

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

// recordingSearch records jump-index appends.
type recordingSearch struct {
	stubSearchService
	appended []int
}

func (s *recordingSearch) AppendJumpPRs(_ string, prs []domain.PullRequestSummary) {
	for _, pr := range prs {
		s.appended = append(s.appended, pr.Number)
	}
}

func openPRs(repo string, from, n int) []domain.PullRequestSummary {
	out := make([]domain.PullRequestSummary, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, pr(repo, from+i, fmt.Sprintf("PR %d", from+i)))
	}
	return out
}

// setupAllTab loads a repo whose first dashboard page has 100 of 250 open PRs
// and switches to the All tab.
func setupAllTab(t *testing.T) (*Model, *stubDashboardService, *recordingSearch) {
	t.Helper()
	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo, openPRs(repo.FullName, 1, 100)...)
	snap.TotalCount = 250
	snap.Truncated = true
	snap.EndCursor = "c1"

	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{repo.FullName: snap})
	svc := m.deps.Dashboard.(*stubDashboardService)
	search := &recordingSearch{}
	m.deps.Search = search
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))
	m.handleChangeTabMsg(dashboard.ChangeTabMsg{Tab: domain.TabAll})
	return m, svc, search
}

// moveTo puts the All-tab cursor on index and returns the load command, if any.
func moveTo(m *Model, index int) tea.Cmd {
	m.prList.Cursor = index
	return m.maybeLoadMoreOpenPRs()
}

func runPage(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected an open-PRs page load")
	}
	msg, ok := cmd().(cmds.OpenPRsPageLoaded)
	if !ok {
		t.Fatalf("expected OpenPRsPageLoaded")
	}
	m.handleOpenPRsPage(msg)
}

func TestAllTabFirstPageIsFree(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)

	if got := len(m.currentPRsForTab(domain.TabAll)); got != 100 {
		t.Fatalf("All tab should show the dashboard's 100 PRs, got %d", got)
	}
	if len(svc.loadOpenPagesCalls) != 0 {
		t.Fatalf("opening the All tab should not fetch, got %v", svc.loadOpenPagesCalls)
	}
	if cmd := moveTo(m, 10); cmd != nil {
		t.Fatal("cursor far from the end should not load a page")
	}
}

func TestAllTabLoadsNextPageNearEnd(t *testing.T) {
	t.Parallel()
	m, svc, search := setupAllTab(t)
	page2 := openPRs("acme/alpha", 101, 100)
	page2[5].Author = "someone"
	page2[0].Author = "octocat" // viewer's PR beyond the first page
	svc.openPages = map[string]stubOpenPage{"c1": {prs: page2, hasMore: true, nextCursor: "c2"}}

	cmd := moveTo(m, 98)
	if !m.prList.Paging.Loading {
		t.Fatal("footer should show loading while a page is in flight")
	}
	if again := moveTo(m, 99); again != nil {
		t.Fatal("only one page should be in flight at a time")
	}
	runPage(t, m, cmd)

	if svc.loadOpenPagesCalls[0] != "c1" {
		t.Fatalf("first extra page should use the dashboard EndCursor, got %v", svc.loadOpenPagesCalls)
	}
	if got := len(m.currentPRsForTab(domain.TabAll)); got != 200 {
		t.Fatalf("All tab should have 200 PRs, got %d", got)
	}
	if m.prList.Cursor != 99 {
		t.Fatalf("cursor should stay on the same PR, got %d", m.prList.Cursor)
	}
	if got := len(m.currentPRsForTab(domain.TabMyPRs)); got != 200-1 {
		t.Fatalf("My PRs should be classified from every loaded page, got %d", got)
	}
	if len(search.appended) != 100 {
		t.Fatalf("loaded page should feed the jump index, got %d", len(search.appended))
	}
}

func TestAllTabStopsAtCap(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)
	m.deps.MaxDashboardPRs = 200
	svc.openPages = map[string]stubOpenPage{"c1": {prs: openPRs("acme/alpha", 101, 100), hasMore: true, nextCursor: "c2"}}

	runPage(t, m, moveTo(m, 99))
	if cmd := moveTo(m, 199); cmd != nil {
		t.Fatal("should not load past dashboard.max_prs")
	}
	if !m.prList.Paging.Capped {
		t.Fatal("footer should say the limit was reached")
	}
}

func TestAllTabFailedPageWaitsForRefresh(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)
	svc.openPages = map[string]stubOpenPage{"c1": {err: errors.New("boom")}}

	runPage(t, m, moveTo(m, 99))
	if !m.prList.Paging.Failed {
		t.Fatal("footer should report the failure")
	}
	if cmd := moveTo(m, 99); cmd != nil {
		t.Fatal("a failed page should not be retried automatically")
	}

	m.refreshSelectedRepo(true)
	if m.prList.Paging.Failed {
		t.Fatal("R should clear the failure")
	}
	repo := testutil.Repo("acme/alpha")
	snap := m.currentDashboard
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))
	if !m.openPages.loading {
		t.Fatal("after R lands, the page should load again")
	}
}

func TestAllTabDropsPageFromBeforeRefresh(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)
	svc.openPages = map[string]stubOpenPage{"c1": {prs: openPRs("acme/alpha", 101, 100), hasMore: true, nextCursor: "c2"}}

	cmd := moveTo(m, 99)
	m.refreshSelectedRepo(true)
	runPage(t, m, cmd)
	if got := len(m.currentPRsForTab(domain.TabAll)); got != 100 {
		t.Fatalf("a page requested before R should be dropped, got %d PRs", got)
	}
}

func TestBackgroundRefreshKeepsPagesAndSelection(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)
	svc.openPages = map[string]stubOpenPage{"c1": {prs: openPRs("acme/alpha", 101, 100), hasMore: true, nextCursor: "c2"}}
	runPage(t, m, moveTo(m, 99))
	m.prList.SelectNumber(150)

	// A refreshed first page where PR #200 moved to the top (it was updated).
	repo := testutil.Repo("acme/alpha")
	fresh := append([]domain.PullRequestSummary{pr(repo.FullName, 200, "PR 200")}, openPRs(repo.FullName, 1, 99)...)
	snap := dashboardSnapshot(repo, fresh...)
	snap.TotalCount, snap.Truncated, snap.EndCursor = 250, true, "c1b"
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	// #200 is deduped; #100 slid onto the server's page 2, which was loaded
	// before it moved, so it is missing until the next manual refresh.
	all := m.currentPRsForTab(domain.TabAll)
	if len(all) != 199 {
		t.Fatalf("background refresh should keep loaded pages (deduped), got %d", len(all))
	}
	if sel, _ := m.currentSelectedPR(); sel.Number != 150 {
		t.Fatalf("selection should stay on #150, got #%d", sel.Number)
	}
}

func TestRepoSwitchDoesNotRestoreSelectionByForeignNumber(t *testing.T) {
	t.Parallel()
	repoA, repoB := testutil.Repo("acme/alpha"), testutil.Repo("acme/beta")
	snapA := dashboardSnapshot(repoA, pr(repoA.FullName, 7, "A7"), pr(repoA.FullName, 3, "A3"))
	snapB := dashboardSnapshot(repoB, pr(repoB.FullName, 1, "B1"), pr(repoB.FullName, 2, "B2"), pr(repoB.FullName, 7, "B7"))
	m := newTestModel([]domain.Repository{repoA, repoB}, map[string]domain.DashboardSnapshot{
		repoA.FullName: snapA, repoB.FullName: snapB,
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repoA, repoB}))
	_, _ = m.Update(cmdsDashboardLoaded(repoA.FullName, snapA, false, nil))

	m.selectRepoByFullName(repoB.FullName, false)
	_, _ = m.Update(cmdsDashboardLoaded(repoB.FullName, snapB, false, nil))
	if m.prList.Cursor != 0 {
		t.Fatalf("cursor should start at the top of the new repo, got %d", m.prList.Cursor)
	}
}

func TestAllTabWaitsForRefreshBeforePaging(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)
	svc.openPages = map[string]stubOpenPage{"c1": {prs: openPRs("acme/alpha", 101, 100), hasMore: true, nextCursor: "c2"}}

	m.refreshSelectedRepo(true)
	if cmd := moveTo(m, 99); cmd != nil {
		t.Fatal("should not page from the old cursor while a refresh is in flight")
	}
	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo, openPRs(repo.FullName, 1, 100)...)
	snap.TotalCount, snap.Truncated, snap.EndCursor = 250, true, "c1"
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))
	if !m.openPages.loading {
		t.Fatal("paging should resume once the refreshed snapshot lands")
	}
}

func TestJumpIndexKeepsExtraPagesAfterRebuild(t *testing.T) {
	t.Parallel()
	m, svc, search := setupAllTab(t)
	svc.openPages = map[string]stubOpenPage{"c1": {prs: openPRs("acme/alpha", 101, 100), hasMore: true, nextCursor: "c2"}}
	runPage(t, m, moveTo(m, 99))

	search.appended = nil
	m.Update(cmds.SearchIndexRebuilt{Repo: "acme/alpha"})
	if len(search.appended) != 100 {
		t.Fatalf("extra pages should be re-added after an index rebuild, got %d", len(search.appended))
	}
}

func TestAllTabStopsOnEmptyPage(t *testing.T) {
	t.Parallel()
	m, svc, _ := setupAllTab(t)
	svc.openPages = map[string]stubOpenPage{"c1": {hasMore: true, nextCursor: "c1"}}

	runPage(t, m, moveTo(m, 99))
	if cmd := moveTo(m, 99); cmd != nil {
		t.Fatal("an empty page should end paging instead of refetching")
	}
}
