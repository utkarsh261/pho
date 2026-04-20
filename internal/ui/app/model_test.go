package app

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
	"github.com/utkarsh261/pho/internal/ui/keymap"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

type stubViewerService struct {
	login string
	err   error
}

func (s *stubViewerService) FetchViewer(ctx context.Context, host string) (string, error) {
	return s.login, s.err
}

type stubDiscoveryService struct {
	repos []domain.Repository
	err   error
}

func (s *stubDiscoveryService) Discover(ctx context.Context, root string) ([]domain.Repository, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]domain.Repository(nil), s.repos...), nil
}

type loadRepoCall struct {
	repo  domain.Repository
	force bool
}

type loadInvolvingCall struct {
	repo   domain.Repository
	viewer string
	force  bool
}

type loadRecentCall struct {
	repo  domain.Repository
	force bool
}

type loadPreviewCall struct {
	repo   string
	number int
}

type stubDashboardService struct {
	dashboardByRepo map[string]domain.DashboardSnapshot
	previewByPR     map[string]domain.PRPreviewSnapshot

	loadRepoCalls      []loadRepoCall
	loadInvolvingCalls []loadInvolvingCall
	loadRecentCalls    []loadRecentCall
	loadPreviewCalls   []loadPreviewCall
}

func (s *stubDashboardService) LoadRepo(ctx context.Context, repo domain.Repository, force bool) (domain.DashboardSnapshot, error) {
	s.loadRepoCalls = append(s.loadRepoCalls, loadRepoCall{repo: repo, force: force})
	if snap, ok := s.dashboardByRepo[repo.FullName]; ok {
		return snap, nil
	}
	return domain.DashboardSnapshot{Repo: repo, FetchedAt: fixedNow()}, nil
}

func (s *stubDashboardService) LoadInvolving(ctx context.Context, repo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error) {
	s.loadInvolvingCalls = append(s.loadInvolvingCalls, loadInvolvingCall{repo: repo, viewer: viewer, force: force})
	return domain.InvolvingSnapshot{Repo: repo, FetchedAt: fixedNow()}, nil
}

func (s *stubDashboardService) LoadRecent(ctx context.Context, repo domain.Repository, force bool) (domain.RecentSnapshot, error) {
	s.loadRecentCalls = append(s.loadRecentCalls, loadRecentCall{repo: repo, force: force})
	return domain.RecentSnapshot{Repo: repo, FetchedAt: fixedNow()}, nil
}

func (s *stubDashboardService) LoadPreview(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error) {
	s.loadPreviewCalls = append(s.loadPreviewCalls, loadPreviewCall{repo: repo, number: number})
	key := previewKey(repo, number)
	if snap, ok := s.previewByPR[key]; ok {
		return snap, nil
	}
	return domain.PRPreviewSnapshot{Repo: repo, Number: number, Title: "Preview"}, nil
}

type stubSearchService struct {
	buildPRCalls   int
	buildRepoCalls int
}

func (s *stubSearchService) BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error {
	s.buildPRCalls++
	return nil
}

func (s *stubSearchService) BuildRepoIndex(repos []domain.Repository) error {
	s.buildRepoCalls++
	return nil
}

func (s *stubSearchService) SearchPRs(query string, limit int) []domain.SearchResult {
	return nil
}

func (s *stubSearchService) SearchRepos(query string, limit int) []domain.SearchResult {
	return nil
}

// stubPRService implements cmds.PRService for testing.
type stubPRService struct {
	detailResult      domain.PRPreviewSnapshot
	detailFromCache   bool
	detailErr         error
	diffResult        model.DiffModel
	diffFromCache     bool
	diffErr           error
	loadDetailCalls   int
	loadDiffCalls     int
}

func (s *stubPRService) LoadDetail(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error) {
	s.loadDetailCalls++
	return s.detailResult, s.detailFromCache, s.detailErr
}

func (s *stubPRService) LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	s.loadDiffCalls++
	return s.diffResult, s.diffFromCache, s.diffErr
}

func TestColdStartThenDashboardLoadedPopulates(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: dashboardSnapshot(repo,
			pr(repo.FullName, 1, "Fix login"),
		),
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))

	before := m.View()
	if contains(before, "Fix login") {
		t.Fatalf("expected cold-start shell without PR rows, got:\n%s", before)
	}

	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, pr(repo.FullName, 1, "Fix login")), false, nil))

	after := m.View()
	if !contains(after, "Fix login") {
		t.Fatalf("expected dashboard row after load, got:\n%s", after)
	}
}

func TestWarmStartCachedDashboardRendersImmediately(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, nil)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, pr(repo.FullName, 7, "Warm cache PR")), true, nil))

	if got := m.View(); !contains(got, "Warm cache PR") {
		t.Fatalf("expected cached PR in view, got:\n%s", got)
	}
}

func TestSelectRepoResetsSelectionAndTriggersLoad(t *testing.T) {
	t.Parallel()

	repoA := testutil.Repo("acme/alpha")
	repoB := testutil.Repo("acme/beta")
	dashboardMap := map[string]domain.DashboardSnapshot{
		repoA.FullName: dashboardSnapshot(repoA,
			pr(repoA.FullName, 1, "Old PR"),
			pr(repoA.FullName, 2, "Older PR"),
		),
		repoB.FullName: dashboardSnapshot(repoB,
			pr(repoB.FullName, 9, "Repo switch result"),
		),
	}
	m := newTestModel([]domain.Repository{repoA, repoB}, dashboardMap)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repoA, repoB}))
	_, _ = m.Update(cmdsDashboardLoaded(repoA.FullName, dashboardMap[repoA.FullName], false, nil))
	_, _ = m.Update(dashboard.SelectPRMsg{Tab: domain.TabMyPRs, Index: 1, Repo: repoA.FullName, Number: 2, Summary: dashboardMap[repoA.FullName].PRs[1]})

	stateBefore := m.State()
	if stateBefore.Dashboard.SelectedIndex != 1 {
		t.Fatalf("expected selected index 1 before repo switch, got %d", stateBefore.Dashboard.SelectedIndex)
	}

	_, cmd := m.Update(dashboard.SelectRepoMsg{Index: 1, Repo: repoB})
	stateAfter := m.State()
	if stateAfter.Repos.SelectedIndex != 1 {
		t.Fatalf("expected selected repo index 1, got %d", stateAfter.Repos.SelectedIndex)
	}
	if stateAfter.Dashboard.SelectedIndex != 0 {
		t.Fatalf("expected PR selection reset to 0, got %d", stateAfter.Dashboard.SelectedIndex)
	}

	msgs := flattenCmd(cmd)
	if !hasDashboardLoadedForRepo(msgs, repoB.FullName) {
		t.Fatalf("expected repo switch command to include dashboard load for %s, got %#v", repoB.FullName, msgs)
	}
}

func TestStalePreviewLoadedIgnoredWhenSelectionChanges(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{
		pr(repo.FullName, 1, "First"),
		pr(repo.FullName, 2, "Second"),
	}
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: dashboardSnapshot(repo, prs...),
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, prs...), false, nil))

	_, _ = m.Update(dashboard.SelectPRMsg{Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0]})
	_, _ = m.Update(dashboard.SelectPRMsg{Tab: domain.TabMyPRs, Index: 1, Repo: repo.FullName, Number: 2, Summary: prs[1]})

	_, _ = m.Update(cmdsPreviewLoaded(repo.FullName, 1, domain.PRPreviewSnapshot{
		Repo:   repo.FullName,
		Number: 1,
		Title:  "Stale Preview",
	}, false, nil))

	state := m.State()
	if state.Dashboard.SelectedIndex != 1 {
		t.Fatalf("expected selected index 1, got %d", state.Dashboard.SelectedIndex)
	}
	if state.Dashboard.Preview == nil {
		t.Fatal("expected non-nil preview after PR selection")
	}
	if state.Dashboard.Preview.Number != 2 {
		t.Fatalf("expected preview to stay on PR #2, got #%d", state.Dashboard.Preview.Number)
	}
	if state.Dashboard.Preview.Title == "Stale Preview" {
		t.Fatalf("expected stale preview payload to be ignored, got %#v", state.Dashboard.Preview)
	}
}

func TestRefreshFailedPreservesStaleData(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, nil)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, pr(repo.FullName, 5, "Stale PR")), false, nil))

	before := m.View()
	if !contains(before, "Stale PR") {
		t.Fatalf("expected stale data in view, got:\n%s", before)
	}

	_, _ = m.Update(cmdsRefreshFailed("dashboard:acme/alpha", errors.New("boom")))

	after := m.View()
	if !contains(after, "Stale PR") {
		t.Fatalf("expected stale data preserved after refresh failure, got:\n%s", after)
	}
	if len(m.State().Errors.Errors) == 0 {
		t.Fatal("expected refresh error to be recorded")
	}
}

// TestRefreshSelectedRepoShowsLoadingState verifies that refreshSelectedRepo
// adds jobs to InFlight and does NOT clear PR data (stale data remains visible
// if refresh fails, which is better than showing nothing).
func TestRefreshSelectedRepoShowsLoadingState(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, nil)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, pr(repo.FullName, 5, "Some PR")), false, nil))

	// Verify initial data exists
	initialPRs := len(m.State().Dashboard.PRsByTab[domain.TabMyPRs])
	if initialPRs == 0 {
		t.Fatal("expected initial PR data")
	}

	// Trigger refresh - this is the action being tested
	cmd := m.refreshSelectedRepo(true)
	if cmd == nil {
		t.Fatal("expected refresh command")
	}

	// Verify: status shows loading by checking status.Loading field
	// syncStatus() updates status.Loading from Jobs.InFlight
	status := m.status
	if !status.Loading {
		t.Error("expected status.Loading = true during refresh")
	}

	// Verify: PR data is NOT cleared (stale data remains visible during refresh)
	// This is critical - if refresh fails, user should still see stale data
	if len(m.State().Dashboard.PRsByTab[domain.TabMyPRs]) != initialPRs {
		t.Error("expected PR data to NOT be cleared during refresh")
	}
}

func TestWindowSizeMsgRecalculatesLayout(t *testing.T) {
	t.Parallel()

	m := newTestModel(nil, nil)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	before := m.Layout().Current
	if before.Preview == 0 {
		t.Fatalf("expected preview pane visible at 120 cols, got %+v", before)
	}

	_, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	after := m.Layout().Current
	if after.Width != 60 || after.Height != 24 {
		t.Fatalf("expected stored size 60x24, got %dx%d", after.Width, after.Height)
	}
	if after.Preview != 0 {
		t.Fatalf("expected preview pane collapsed at 60 cols, got %+v", after)
	}
	if after.PR == 0 {
		t.Fatalf("expected PR pane visible at 60 cols, got %+v", after)
	}
}

func newTestModel(repos []domain.Repository, dashboards map[string]domain.DashboardSnapshot) *Model {
	if dashboards == nil {
		dashboards = make(map[string]domain.DashboardSnapshot)
	}
	deps := Dependencies{
		Viewer: &stubViewerService{login: "octocat"},
		Discovery: &stubDiscoveryService{
			repos: append([]domain.Repository(nil), repos...),
		},
		Dashboard: &stubDashboardService{dashboardByRepo: dashboards, previewByPR: map[string]domain.PRPreviewSnapshot{}},
		Search:    &stubSearchService{},
		PR:        &stubPRService{},
		Root:      ".",
		Host:      "github.com",
		Now:       fixedNow,
	}
	model := NewModel(deps)
	model.state.Session.ViewerByHost["github.com"] = "octocat"
	return model
}

func fixedNow() time.Time {
	return time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
}

func dashboardSnapshot(repo domain.Repository, prs ...domain.PullRequestSummary) domain.DashboardSnapshot {
	return domain.DashboardSnapshot{
		Repo:       repo,
		PRs:        append([]domain.PullRequestSummary(nil), prs...),
		TotalCount: len(prs),
		Truncated:  false,
		FetchedAt:  fixedNow(),
	}
}

func pr(repo string, number int, title string) domain.PullRequestSummary {
	return domain.PullRequestSummary{
		Repo:           repo,
		Number:         number,
		Title:          title,
		Author:         "octocat",
		State:          domain.PRStateOpen,
		CIStatus:       domain.CIStatusSuccess,
		ReviewDecision: domain.ReviewDecisionApproved,
		HeadRefName:    "feature/test",
		BaseRefName:    "main",
		CreatedAt:      fixedNow().Add(-2 * time.Hour),
		UpdatedAt:      fixedNow().Add(-1 * time.Hour),
	}
}

func previewKey(repo string, number int) string {
	return repo + "#" + itoa(number)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func flattenCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		out := make([]tea.Msg, 0, len(batchMsg))
		for _, nested := range batchMsg {
			out = append(out, flattenCmd(nested)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func hasDashboardLoadedForRepo(msgs []tea.Msg, repo string) bool {
	for _, msg := range msgs {
		if loaded, ok := msg.(cmds.DashboardLoaded); ok && loaded.Repo == repo {
			return true
		}
	}
	return false
}

func cmdsReposDiscovered(repos []domain.Repository) tea.Msg {
	return cmds.ReposDiscovered{Repos: append([]domain.Repository(nil), repos...)}
}

func cmdsDashboardLoaded(repo string, snap domain.DashboardSnapshot, fromCache bool, err error) tea.Msg {
	return cmds.DashboardLoaded{Repo: repo, Snapshot: snap, FromCache: fromCache, Err: err}
}

func cmdsPreviewLoaded(repo string, number int, snap domain.PRPreviewSnapshot, fromCache bool, err error) tea.Msg {
	return cmds.PreviewLoaded{Repo: repo, Number: number, Preview: snap, FromCache: fromCache, Err: err}
}

func cmdsRefreshFailed(key string, err error) tea.Msg {
	return cmds.RefreshFailed{Key: key, Err: err}
}

// ---------------------------------------------------------------------------
// Preview panel integration tests
// ---------------------------------------------------------------------------

// TestPreviewPanelRendersTitleAfterDashboardLoad verifies that after a dashboard
// loads, the auto-selected PR's title appears in View() (the preview panel renders
// the derived summary without waiting for the full preview fetch).
func TestPreviewPanelRendersTitleAfterDashboardLoad(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo, pr(repo.FullName, 42, "Implement preview"))
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: snap,
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	got := m.View()
	if !contains(got, "Implement preview") {
		t.Fatalf("expected PR title in preview panel, got:\n%s", got)
	}
	// Sanity: preview dimensions must be non-zero.
	if m.preview.Width == 0 || m.preview.Height == 0 {
		t.Fatalf("preview panel dimensions are zero (%dx%d)", m.preview.Width, m.preview.Height)
	}
}

// TestPreviewPanelRendersFullBodyAfterPreviewLoaded verifies that after PreviewLoaded
// arrives with a body excerpt, that text appears in View().
func TestPreviewPanelRendersFullBodyAfterPreviewLoaded(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo, pr(repo.FullName, 5, "Add tests"))
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: snap,
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	// Deliver full preview with a body short enough to fit on a single line.
	_, _ = m.Update(cmdsPreviewLoaded(repo.FullName, 5, domain.PRPreviewSnapshot{
		Repo:        repo.FullName,
		Number:      5,
		Title:       "Add tests",
		BodyExcerpt: "short body",
		Author:      "octocat",
		State:       domain.PRStateOpen,
	}, false, nil))

	got := m.View()
	if !contains(got, "Add tests") {
		t.Fatalf("expected title in view after preview load, got:\n%s", got)
	}
	if !contains(got, "short body") {
		t.Fatalf("expected body excerpt in view after preview load, got:\n%s", got)
	}
}

// TestPreviewPanelRetainsDimensionsAfterTabChange verifies that after a tab change
// (which internally recreates m.preview), the new panel's dimensions are restored
// from the layout and the PR title still appears in View().
func TestPreviewPanelRetainsDimensionsAfterTabChange(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo, pr(repo.FullName, 7, "Tab switch PR"))
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: snap,
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	before := m.View()
	if !contains(before, "Tab switch PR") {
		t.Fatalf("expected title before tab change, got:\n%s", before)
	}

	// Cycle through another tab and back.
	_, _ = m.Update(dashboard.ChangeTabMsg{Tab: domain.TabNeedsReview})
	_, _ = m.Update(dashboard.ChangeTabMsg{Tab: domain.TabMyPRs})

	after := m.View()
	if !contains(after, "Tab switch PR") {
		t.Fatalf("expected title preserved after tab cycle, got:\n%s", after)
	}
	// Confirm dimensions were restored (the bug: NewPreviewPanelModel starts at 0×0).
	if m.preview.Width == 0 || m.preview.Height == 0 {
		t.Fatalf("preview panel dimensions reset to zero after tab change: %dx%d",
			m.preview.Width, m.preview.Height)
	}
}

// TestPreviewFetchPendingFetchClearedByFetchMsg verifies that handling a
// PreviewFetchMsg always clears PendingFetch on the preview panel, regardless of
// whether the guards proceed to fetch. Without this fix the next SelectPRMsg would
// see PendingFetch=true and skip starting a new debounce timer.
func TestPreviewFetchPendingFetchClearedByFetchMsg(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{
		pr(repo.FullName, 1, "First PR"),
		pr(repo.FullName, 2, "Second PR"),
	}
	snap := dashboardSnapshot(repo, prs...)
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: snap,
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	// After dashboard load, syncCurrentSelection fires SelectPRMsg → PendingFetch=true.
	if !m.preview.PendingFetch {
		t.Fatal("expected PendingFetch=true after initial dashboard load")
	}

	// Simulate the debounce timer firing for PR #1.
	_, _ = m.Update(dashboard.PreviewFetchMsg{Repo: repo.FullName, Number: 1})

	// PendingFetch must be cleared now — the fix forwards PreviewFetchMsg to the panel.
	if m.preview.PendingFetch {
		t.Fatal("expected PendingFetch=false after PreviewFetchMsg; " +
			"without the fix PendingFetch stays true and future SelectPRMsgs are no-ops")
	}

	// Now selecting a different PR must create a new debounce timer (cmd != nil).
	_, cmd := m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 1,
		Repo: repo.FullName, Number: 2, Summary: prs[1],
	})
	if cmd == nil {
		t.Fatal("expected non-nil cmd (new debounce timer) from SelectPRMsg after PendingFetch cleared")
	}
	if !m.preview.PendingFetch {
		t.Fatal("expected PendingFetch=true after second SelectPRMsg")
	}
}

func TestE2EFullLifecycle(t *testing.T) {
	t.Parallel()

	// 1. Boot: cold start with empty dashboard
	m := newTestModel(nil, nil)

	// 2. Send WindowSizeMsg
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// 3. Send ReposDiscovered with one repo "acme/alpha"
	repo := testutil.Repo("acme/alpha")
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))

	// 4. Verify PR list is empty (no PRs visible yet)
	viewAfterDiscover := m.View()
	if contains(viewAfterDiscover, "feat: foo") || contains(viewAfterDiscover, "fix: bar") {
		t.Fatalf("expected no PRs after discovery (cold start), got:\n%s", viewAfterDiscover)
	}

	// 5. Send DashboardLoaded for "acme/alpha" with 2 PRs: #10 "feat: foo" and #11 "fix: bar"
	snap := dashboardSnapshot(repo,
		pr(repo.FullName, 10, "feat: foo"),
		pr(repo.FullName, 11, "fix: bar"),
	)
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	// 6. Verify both PR titles visible in m.View()
	viewAfterLoad := m.View()
	if !contains(viewAfterLoad, "feat: foo") {
		t.Fatalf("expected 'feat: foo' in view after dashboard load, got:\n%s", viewAfterLoad)
	}
	if !contains(viewAfterLoad, "fix: bar") {
		t.Fatalf("expected 'fix: bar' in view after dashboard load, got:\n%s", viewAfterLoad)
	}

	// 7. Switch tab: send dashboard.ChangeTabMsg{Tab: domain.TabNeedsReview}
	_, _ = m.Update(dashboard.ChangeTabMsg{Tab: domain.TabNeedsReview})

	// 8. Verify tab state is domain.TabNeedsReview
	if got := m.State().Dashboard.ActiveTab; got != domain.TabNeedsReview {
		t.Fatalf("expected active tab %q, got %q", domain.TabNeedsReview, got)
	}

	// 9. Switch tab back to domain.TabMyPRs
	_, _ = m.Update(dashboard.ChangeTabMsg{Tab: domain.TabMyPRs})
	if got := m.State().Dashboard.ActiveTab; got != domain.TabMyPRs {
		t.Fatalf("expected active tab %q after switching back, got %q", domain.TabMyPRs, got)
	}

	// 10. Open command palette: send tea.KeyMsg{Type: tea.KeyCtrlP}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})

	// 11. Verify m.State().Search.OverlayOpen == true
	if !m.State().Search.OverlayOpen {
		t.Fatalf("expected overlay to be open after Ctrl+P")
	}

	// 12. Send overlay.OpenPR directly — the returned cmd must be non-nil
	_, openCmd := m.Update(overlay.OpenPR{Repo: "acme/alpha", Number: 10})
	if openCmd == nil {
		t.Fatalf("expected non-nil cmd from overlay.OpenPR (browser-open should be triggered)")
	}

	// 13. Close palette: send overlay.CloseCmdPalette{}
	_, _ = m.Update(overlay.CloseCmdPalette{})

	// 14. Verify overlay is closed
	if m.State().Search.OverlayOpen {
		t.Fatalf("expected overlay to be closed after CloseCmdPalette")
	}
}

// ---- Chunk C: Open/Close Scaffold tests ----

// Test helpers
func setupModelWithPRs(t *testing.T, repos []domain.Repository, prs []domain.PullRequestSummary) *Model {
	t.Helper()
	dash := make(map[string]domain.DashboardSnapshot)
	for _, repo := range repos {
		dash[repo.FullName] = dashboardSnapshot(repo, prs...)
	}
	m := newTestModel(repos, dash)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered(repos))
	for _, repo := range repos {
		_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dash[repo.FullName], false, nil))
	}
	return m
}

// TestViewStackPushPop verifies basic push/pop behavior.
func TestViewStackPushPop(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, nil)
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("initial view should be dashboard, got %s", m.currentView())
	}
	m.pushView(domain.PrimaryViewPRDetail)
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("after push, view should be PR detail, got %s", m.currentView())
	}
	if len(m.viewStack) != 2 {
		t.Fatalf("stack length should be 2, got %d", len(m.viewStack))
	}

	popped := m.popView()
	if popped != domain.PrimaryViewDashboard {
		t.Fatalf("pop should return dashboard, got %s", popped)
	}
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("after pop, view should be dashboard, got %s", m.currentView())
	}
	if len(m.viewStack) != 1 {
		t.Fatalf("stack length should be 1, got %d", len(m.viewStack))
	}
}

// TestViewStackFloorGuard verifies we never pop below 1 element.
func TestViewStackFloorGuard(t *testing.T) {
	t.Parallel()

	m := newTestModel(nil, nil)
	if len(m.viewStack) != 1 {
		t.Fatalf("initial stack length should be 1, got %d", len(m.viewStack))
	}
	popped := m.popView()
	if popped != domain.PrimaryViewDashboard {
		t.Fatalf("pop on single-element stack should return dashboard, got %s", popped)
	}
	if len(m.viewStack) != 1 {
		t.Fatalf("stack length should still be 1, got %d", len(m.viewStack))
	}
}

// TestViewStackDoublePushGuard verifies pushing same view twice is a no-op.
func TestViewStackDoublePushGuard(t *testing.T) {
	t.Parallel()

	m := newTestModel(nil, nil)
	m.pushView(domain.PrimaryViewPRDetail)
	if len(m.viewStack) != 2 {
		t.Fatalf("after first push, stack should be 2, got %d", len(m.viewStack))
	}
	m.pushView(domain.PrimaryViewPRDetail)
	if len(m.viewStack) != 2 {
		t.Fatalf("double-push should be no-op, stack=%d, want 2", len(m.viewStack))
	}
}

// TestViewDispatchToPRDetail verifies that Enter on the PR list opens PR detail
// through the full dispatch chain: key → SelectPR action → openPRDetail.
func TestViewDispatchToPRDetail(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Fix auth bug")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	// Set focus on the PR list and select the PR via a SelectPRMsg (cursor + preview).
	m.focus = domain.FocusPRListPanel
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab:     domain.TabMyPRs,
		Index:   0,
		Repo:    repo.FullName,
		Number:  1,
		Summary: prs[0],
	})

	// Enter on the PR list dispatches SelectPR → handleSelectPRAndOpenDetail → openPRDetail.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view after Enter, got %s", m.currentView())
	}
	if m.prDetail == nil {
		t.Fatal("expected prDetail model to be non-nil")
	}
	if m.prDetail.Summary.Number != 1 {
		t.Errorf("expected PR number 1, got %d", m.prDetail.Summary.Number)
	}
}

// TestViewDispatchToDashboard verifies BackToDashboard message routes through
// the root model's Update and returns to dashboard view.
func TestViewDispatchToDashboard(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Fix auth")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	// Open detail.
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}

	// BackToDashboard routes through root model Update — not a direct call.
	_, _ = m.Update(prdetail.BackToDashboard{})
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard view after BackToDashboard, got %s", m.currentView())
	}
}

// TestSelectPRPushesView verifies that opening a PR from the list pushes the view.
func TestSelectPRPushesView(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	// Simulate Enter on PR list → SelectPR action → OpenPRDetail.
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail after opening PR, got %s", m.currentView())
	}
}

// TestSelectPRPushGuardNoDuplicate verifies double-open doesn't create duplicate stack entries.
func TestSelectPRPushGuardNoDuplicate(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()
	stackLen1 := len(m.viewStack)

	// Open same PR again.
	m.openPRDetail()
	stackLen2 := len(m.viewStack)

	if stackLen1 != stackLen2 {
		t.Fatalf("stack should not grow on re-open: first=%d, second=%d", stackLen1, stackLen2)
	}
}

// TestSelectSamePRReusesModel verifies that reopening the same PR reuses the model (scroll preserved).
func TestSelectSamePRReusesModel(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()

	originalModel := m.prDetail
	// Simulate scroll.
	m.prDetail.ContentScroll = 42

	// Open same PR again.
	m.openPRDetail()

	if m.prDetail != originalModel {
		t.Fatal("expected same model instance on re-open of same PR")
	}
	if m.prDetail.ContentScroll != 42 {
		t.Errorf("expected scroll position preserved (42), got %d", m.prDetail.ContentScroll)
	}
}

// TestSelectDifferentPRConstructsNew verifies opening a different PR creates a new model.
func TestSelectDifferentPRConstructsNew(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{
		pr(repo.FullName, 1, "PR 1"),
		pr(repo.FullName, 2, "PR 2"),
	}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	// Open PR 1.
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()
	model1 := m.prDetail

	// Go back.
	m.handleBackToDashboard()

	// Select PR 2.
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 1, Repo: repo.FullName, Number: 2, Summary: prs[1],
	})
	m.openPRDetail()

	if m.prDetail == model1 {
		t.Fatal("expected new model instance for different PR")
	}
	if m.prDetail.Summary.Number != 2 {
		t.Errorf("expected PR number 2, got %d", m.prDetail.Summary.Number)
	}
}

// TestEscFromPRDetailReturnsToDashboard verifies the full Esc dispatch path:
// Esc → prDetail emits BackToDashboard cmd → execute cmd → root Update handles it.
func TestEscFromPRDetailReturnsToDashboard(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	// Set narrow width so Esc goes directly to Dashboard (bypasses Content→Files cycle)
	m.prDetail.Width = 60

	// Esc in prDetail returns a BackToDashboard cmd. Execute it to get the message,
	// then feed it through root Update — the real dispatch path.
	_, escCmd := m.prDetail.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if escCmd == nil {
		t.Fatal("expected non-nil cmd from Esc key in PR detail")
	}
	backMsg := escCmd()
	if _, ok := backMsg.(prdetail.BackToDashboard); !ok {
		t.Fatalf("expected BackToDashboard message from Esc cmd, got %T", backMsg)
	}
	_, _ = m.Update(backMsg)

	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard after Esc, got %s", m.currentView())
	}
	if m.focus != domain.FocusPRListPanel {
		t.Errorf("expected focus on PR list panel after Esc, got %s", m.focus)
	}
}

// TestEscFromDashboardIsNoop verifies Esc on dashboard doesn't dispatch BackToDashboard.
// (It may dispatch Quit per existing behavior, but never BackToDashboard.)
func TestEscFromDashboardIsNoop(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard view, got %s", m.currentView())
	}

	// Esc from preview panel dispatches globally (Quit per existing behavior).
	// The key point: it does NOT dispatch BackToDashboard or OpenPRDetail.
	result := keymap.Dispatch(domain.FocusPreviewPanel, tea.KeyMsg{Type: tea.KeyEsc})
	if _, isOpenPRDetail := result.Action.(keymap.OpenPRDetail); isOpenPRDetail {
		t.Fatalf("Esc from dashboard should NOT dispatch OpenPRDetail")
	}
	// Quit is acceptable (existing behavior).
	if _, isQuit := result.Action.(keymap.Quit); isQuit {
		// Acceptable — existing global Esc behavior.
		return
	}
	// No action or other actions are also fine.
}

// TestRKeyRefiresBothCmdsWithForce verifies 'r' in PR detail refires both load
// commands with force=true and that both actually call the PR service.
func TestRKeyRefiresBothCmdsWithForce(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	// Wire a stub PR service so Init/refresh calls can be counted.
	stubPR := &stubPRService{}
	m.deps.PR = stubPR

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()

	// Reset counts after Init (which fired on openPRDetail).
	stubPR.loadDetailCalls = 0
	stubPR.loadDiffCalls = 0

	// Send 'r' key.
	_, cmd := m.prDetail.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Loading flags are set immediately by handleRefresh (before cmd fires).
	if !m.prDetail.DetailLoading || !m.prDetail.DiffLoading {
		t.Error("expected both loading flags to be true after 'r' refresh")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'r' refresh")
	}

	// Execute the cmd: it should fire both LoadPRDetailCmd and LoadDiffCmd.
	msgs := flattenCmd(cmd)
	var hasDetail, hasDiff bool
	for _, msg := range msgs {
		switch msg.(type) {
		case cmds.PRDetailLoaded:
			hasDetail = true
		case cmds.DiffLoaded:
			hasDiff = true
		}
	}
	if !hasDetail {
		t.Error("expected PRDetailLoaded in cmd output from 'r' refresh")
	}
	if !hasDiff {
		t.Error("expected DiffLoaded in cmd output from 'r' refresh")
	}
	if stubPR.loadDetailCalls != 1 {
		t.Errorf("expected 1 LoadDetail call, got %d", stubPR.loadDetailCalls)
	}
	if stubPR.loadDiffCalls != 1 {
		t.Errorf("expected 1 LoadDiff call, got %d", stubPR.loadDiffCalls)
	}
}

// TestPRDetailLoadedFromCacheSchedulesRevalidation verifies that receiving a stale
// cache hit schedules a background revalidation (force=true).
func TestPRDetailLoadedFromCacheSchedulesRevalidation(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	detail := domain.PRPreviewSnapshot{
		Repo:        repo.FullName,
		Number:      1,
		Title:       "Cached PR",
		BodyExcerpt: "Cached body",
	}
	stubPR := &stubPRService{
		detailResult:    detail,
		detailFromCache: true,
	}

	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)
	m.deps.PR = stubPR

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()

	// Send PRDetailLoaded with FromCache=true.
	_, cmd := m.prDetail.Update(cmds.PRDetailLoaded{
		Repo:      repo.FullName,
		Number:    1,
		Detail:    detail,
		FromCache: true,
	})
	if cmd == nil {
		t.Fatal("expected non-nil cmd (revalidation) when FromCache=true")
	}
}

// TestForwardKeyInPRDetailNotDashboard verifies that keys in PR detail go to prDetail, not dashboard.
func TestForwardKeyInPRDetailNotDashboard(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{pr(repo.FullName, 1, "Test PR")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 1, Summary: prs[0],
	})
	m.openPRDetail()

	// Send 'j' — this should go to prDetail, not the dashboard prList.
	beforeCursor := m.prList.Cursor
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	afterCursor := m.prList.Cursor

	if afterCursor != beforeCursor {
		t.Fatalf("prList cursor should not change when key is forwarded to prDetail: before=%d, after=%d", beforeCursor, afterCursor)
	}
}

// TestEnterOnPRListDispatchesSelectPR verifies Enter on PR list fires SelectPR action.
func TestEnterOnPRListDispatchesSelectPR(t *testing.T) {
	t.Parallel()

	result := keymap.Dispatch(domain.FocusPRListPanel, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := result.Action.(keymap.SelectPR); !ok {
		t.Fatalf("expected SelectPR action, got %T", result.Action)
	}
}

// TestEnterOnPreviewPanelOpensDetail verifies Enter on preview panel fires OpenPRDetail.
func TestEnterOnPreviewPanelOpensDetail(t *testing.T) {
	t.Parallel()

	result := keymap.Dispatch(domain.FocusPreviewPanel, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := result.Action.(keymap.OpenPRDetail); !ok {
		t.Fatalf("expected OpenPRDetail action, got %T", result.Action)
	}
}

// TestOKeyInPRDetailOpensBrowser verifies 'o' in PR detail emits OpenBrowserPR.
func TestOKeyInPRDetailOpensBrowser(t *testing.T) {
	t.Parallel()

	mdl := prdetail.NewModel(
		domain.PullRequestSummary{Repo: "acme/alpha", Number: 42, Title: "Test"},
		domain.Repository{FullName: "acme/alpha"},
		nil,
	)

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'o' key")
	}
	// Execute the cmd and verify it emits OpenBrowserPR.
	msg := cmd()
	if msg == nil {
		t.Fatal("expected cmd to return a message")
	}
	bridge, ok := msg.(prdetail.OpenBrowserPR)
	if !ok {
		t.Fatalf("expected OpenBrowserPR message, got %T", msg)
	}
	if bridge.Number != 42 {
		t.Errorf("expected PR number 42, got %d", bridge.Number)
	}
}

// TestOKeyInDashboardPreviewStillOpensBrowser verifies 'o' in dashboard preview still opens browser.
func TestOKeyInDashboardPreviewStillOpensBrowser(t *testing.T) {
	t.Parallel()

	result := keymap.Dispatch(domain.FocusPreviewPanel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if _, ok := result.Action.(keymap.OpenBrowser); !ok {
		t.Fatalf("expected OpenBrowser action from dashboard preview, got %T", result.Action)
	}
}

// escFromDetail presses Esc in PR detail and feeds the BackToDashboard message
// through the root model — the real dispatch path.
func escFromDetail(t *testing.T, m *Model) {
	t.Helper()
	_, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if escCmd == nil {
		t.Fatal("expected non-nil cmd from Esc in PR detail")
	}
	_, _ = m.Update(escCmd())
}

// TestDashboardRoundTripViaPRDetail is a regression guard for the full
// dashboard → PR detail → dashboard flow. All dispatches go through m.Update.
func TestDashboardRoundTripViaPRDetail(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prs := []domain.PullRequestSummary{
		pr(repo.FullName, 1, "First PR"),
		pr(repo.FullName, 2, "Second PR"),
		pr(repo.FullName, 3, "Third PR"),
	}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)
	m.focus = domain.FocusPRListPanel

	// Step 1: Move cursor to the second PR (index 1).
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.prList.Cursor != 1 {
		t.Fatalf("step 1: expected cursor at index 1, got %d", m.prList.Cursor)
	}

	// Step 2: Enter → PR detail opens for PR #2.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("step 2: expected PR detail view, got %s", m.currentView())
	}
	if m.prDetail == nil {
		t.Fatal("step 2: expected prDetail model to be non-nil")
	}
	if m.prDetail.Summary.Number != 2 {
		t.Errorf("step 2: expected PR number 2, got %d", m.prDetail.Summary.Number)
	}
	// Set narrow width so Esc goes directly to Dashboard
	m.prDetail.Width = 60

	// Step 3: Esc → returns to dashboard via real dispatch path.
	escFromDetail(t, m)
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("step 3: expected dashboard view after Esc, got %s", m.currentView())
	}

	// Step 4: Cursor is preserved; focus is restored to PR list.
	if m.prList.Cursor != 1 {
		t.Fatalf("step 4: expected cursor at index 1 after Esc, got %d", m.prList.Cursor)
	}
	if m.focus != domain.FocusPRListPanel {
		t.Fatalf("step 4: expected focus on PR list panel, got %s", m.focus)
	}

	// Step 5: Tab selection is preserved; view is dashboard.
	if m.prList.Active != domain.TabMyPRs {
		t.Fatalf("step 5: expected active tab %q, got %q", domain.TabMyPRs, m.prList.Active)
	}
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("step 5: expected dashboard view, got %s", m.currentView())
	}

	// Step 6: Keyboard navigation still works — j moves to index 2.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.prList.Cursor != 2 {
		t.Fatalf("step 6: expected cursor at index 2 after 'j', got %d", m.prList.Cursor)
	}

	// Step 7: k moves back to index 1.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.prList.Cursor != 1 {
		t.Fatalf("step 7: expected cursor at index 1 after 'k', got %d", m.prList.Cursor)
	}

	// Step 8: Reopen PR #2 — same-PR reuse preserves scroll.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("step 8: expected PR detail view, got %s", m.currentView())
	}
	m.prDetail.ContentScroll = 100
	escFromDetail(t, m)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("step 8 reopen: expected PR detail view, got %s", m.currentView())
	}
	if m.prDetail.ContentScroll != 100 {
		t.Errorf("step 8 reopen: expected scroll=100 (same-PR reuse), got %d", m.prDetail.ContentScroll)
	}

	// Step 9: Esc, navigate to PR #3 — preview update works correctly.
	escFromDetail(t, m)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.prList.Cursor != 2 {
		t.Fatalf("step 9: expected cursor at index 2 after 'j', got %d", m.prList.Cursor)
	}

	// Drive the preview fetch cycle for PR #3.
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab: domain.TabMyPRs, Index: 2, Repo: repo.FullName, Number: 3, Summary: prs[2],
	})
	if !m.preview.Loading {
		t.Fatal("step 9: expected Loading=true after SelectPRMsg for PR #3")
	}
	_, _ = m.Update(dashboard.PreviewFetchMsg{
		Repo: repo.FullName, Number: 3, Generation: m.preview.DebounceGeneration,
	})
	preview3 := domain.PRPreviewSnapshot{
		Repo: repo.FullName, Number: 3, Title: "Third PR",
		Author: "octocat", State: domain.PRStateOpen,
	}
	_, _ = m.Update(dashboard.PreviewLoadedMsg{
		Repo: repo.FullName, Number: 3, Preview: preview3,
	})
	if m.state.Dashboard.Preview == nil || m.state.Dashboard.Preview.Number != 3 {
		t.Fatalf("step 9: preview should be for PR #3, got %+v", m.state.Dashboard.Preview)
	}
	if m.preview.Loading {
		t.Fatal("step 9: preview still loading after PreviewLoadedMsg")
	}
}

