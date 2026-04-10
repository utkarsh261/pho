package app

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utk/git-term/internal/application/cmds"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/testutil"
	"github.com/utk/git-term/internal/ui/components/overlay"
	"github.com/utk/git-term/internal/ui/views/dashboard"
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
		Root:      ".",
		Host:      "github.com",
		Now:       fixedNow,
	}
	model := NewModel(deps)
	model.state.Session.Viewer = "octocat"
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
