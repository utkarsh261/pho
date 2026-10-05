package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

const defaultMaxDashboardPRs = 300

// openPRPages tracks open PRs loaded beyond the dashboard's first page. The
// first page comes from the cached dashboard snapshot; later pages are
// fetched on demand as the All tab scrolls and live in memory only.
type openPRPages struct {
	repo       string
	generation int // bumped on reset so in-flight pages from before are dropped
	extra      []domain.PullRequestSummary
	nextCursor string
	hasMore    bool
	fetched    bool // at least one extra page arrived; nextCursor/hasMore are valid
	loading    bool
	failed     bool
}

func (p *openPRPages) reset(repo string) {
	*p = openPRPages{repo: repo, generation: p.generation + 1}
}

// allOpenPRs is the dashboard's first page followed by any extra pages,
// deduped by number with the first page winning (it is refreshed more often).
func (m *Model) allOpenPRs() []domain.PullRequestSummary {
	first := m.currentDashboard.PRs
	if len(m.openPages.extra) == 0 || !sameRepo(m.openPages.repo, m.currentDashboard.Repo.FullName) {
		return append([]domain.PullRequestSummary(nil), first...)
	}
	out := make([]domain.PullRequestSummary, 0, len(first)+len(m.openPages.extra))
	seen := make(map[int]struct{}, cap(out))
	for _, list := range [][]domain.PullRequestSummary{first, m.openPages.extra} {
		for _, pr := range list {
			if _, ok := seen[pr.Number]; ok {
				continue
			}
			seen[pr.Number] = struct{}{}
			out = append(out, pr)
		}
	}
	return out
}

// nextOpenPRsPage returns the cursor of the next open-PR page, if any.
func (m *Model) nextOpenPRsPage() (string, bool) {
	if m.openPages.fetched {
		return m.openPages.nextCursor, m.openPages.hasMore && m.openPages.nextCursor != ""
	}
	return m.currentDashboard.EndCursor, m.currentDashboard.Truncated && m.currentDashboard.EndCursor != ""
}

func (m *Model) maxDashboardPRs() int {
	if m.deps.MaxDashboardPRs > 0 {
		return m.deps.MaxDashboardPRs
	}
	return defaultMaxDashboardPRs
}

func (m *Model) syncOpenPRsPaging() {
	_, hasMore := m.nextOpenPRsPage()
	m.prList.Paging = dashboard.PageStatus{
		Loading: m.openPages.loading,
		Failed:  m.openPages.failed,
		Capped:  hasMore && len(m.state.Dashboard.PRsByTab[domain.TabAll]) >= m.maxDashboardPRs(),
	}
}

// maybeLoadMoreOpenPRs fetches the next open-PR page when the All tab's
// cursor is within a screen of the end. One page is in flight at a time, and
// a failed page is only retried after a manual refresh (R).
func (m *Model) maybeLoadMoreOpenPRs() tea.Cmd {
	if m.prList.Active != domain.TabAll || m.deps.Dashboard == nil {
		return nil
	}
	repo, ok := m.selectedRepo()
	if !ok || !sameRepo(repo.FullName, m.currentDashboard.Repo.FullName) {
		return nil
	}
	if !sameRepo(m.openPages.repo, repo.FullName) {
		m.openPages.reset(repo.FullName)
	}
	// While a refresh is in flight the page-1 cursor is about to change, so
	// wait for the new snapshot rather than page from the old one.
	if m.openPages.loading || m.openPages.failed || m.state.Jobs.InFlight[jobKey(repo.FullName, "dashboard")] {
		return nil
	}
	cursor, hasMore := m.nextOpenPRsPage()
	loaded := len(m.state.Dashboard.PRsByTab[domain.TabAll])
	if !hasMore || loaded >= m.maxDashboardPRs() {
		return nil
	}
	if m.prList.Cursor < loaded-max(m.prList.VisibleItemCount(), 1) {
		return nil
	}
	m.openPages.loading = true
	m.syncOpenPRsPaging()
	m.logDebug("loading open PRs page", "repo", repo.FullName, "loaded", loaded)
	return cmds.FetchOpenPRsPageCmd(m.deps.Dashboard, repo, cursor, m.openPages.generation)
}

func (m *Model) handleOpenPRsPage(msg cmds.OpenPRsPageLoaded) tea.Cmd {
	if msg.Generation != m.openPages.generation || !sameRepo(msg.Repo, m.openPages.repo) {
		return nil
	}
	m.openPages.loading = false
	if msg.Err != nil {
		m.logError("open PRs page failed", "repo", msg.Repo, "err", msg.Err)
		m.openPages.failed = true
		m.syncOpenPRsPaging()
		return nil
	}
	m.openPages.extra = append(m.openPages.extra, msg.Entries...)
	m.openPages.nextCursor = msg.NextCursor
	// An empty page or a cursor that didn't advance would refetch forever.
	m.openPages.hasMore = msg.HasMore && len(msg.Entries) > 0 && msg.NextCursor != msg.Cursor
	m.openPages.fetched = true
	m.rebuildDashboardTabs()
	if m.deps.Search != nil && len(msg.Entries) > 0 {
		m.deps.Search.AppendJumpPRs(msg.Repo, msg.Entries)
	}
	m.syncStatus()
	return m.maybeLoadMoreOpenPRs()
}

// reappendOpenPRPages restores the All tab's extra pages to the jump index
// after a dashboard refresh rebuilt it from the first page only.
func (m *Model) reappendOpenPRPages(repo string) {
	if m.deps.Search == nil || len(m.openPages.extra) == 0 || !sameRepo(repo, m.openPages.repo) {
		return
	}
	m.deps.Search.AppendJumpPRs(m.openPages.repo, m.openPages.extra)
}

// restoreSelection keeps the cursor on the same PR after a list is rebuilt,
// falling back to clamping the old index when that PR is gone. The previous
// PR only counts if it belongs to the selected repo: after a repo switch the
// old repo's list is still showing, and its numbers mean nothing here.
func (m *Model) restoreSelection(prev domain.PullRequestSummary, hadPrev bool) {
	if hadPrev && !sameRepo(prev.Repo, m.selectedRepoName()) {
		hadPrev = false
	}
	if !hadPrev || !m.prList.SelectNumber(prev.Number) {
		m.prList.Cursor = clampIndex(m.prList.Cursor, len(m.currentPRsForTab(m.prList.Active)))
	}
	m.state.Dashboard.SelectedIndex = m.prList.Cursor
}
