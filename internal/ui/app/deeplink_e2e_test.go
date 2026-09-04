package app

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
)

// Run with -update-deeplink to regenerate golden files:
//
//	go test ./internal/ui/app/... -update-deeplink
var updateDeeplink = flag.Bool("update-deeplink", false, "overwrite deeplink e2e golden files")

var deeplinkAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return deeplinkAnsiRe.ReplaceAllString(s, "")
}

// drainCmds runs the production message loop: every command's messages are
// fed back through Model.Update until only spinner ticks remain (bounded so a
// misbehaving flow fails the test instead of hanging it).
func drainCmds(m *Model, cmd tea.Cmd) {
	queue := append([]tea.Cmd(nil), cmd)
	for steps := 0; len(queue) > 0; {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		for _, msg := range flattenCmd(next) {
			steps++
			if steps > 1000 {
				return
			}
			if _, ok := msg.(spinner.TickMsg); ok {
				continue
			}
			if _, out := m.Update(msg); out != nil {
				queue = append(queue, out)
			}
		}
	}
}

func newDeepLinkTestModel(t *testing.T, initialPR int, repos []domain.Repository, prSvc *stubPRService) *Model {
	t.Helper()
	deps := Dependencies{
		Viewer: &stubViewerService{login: "octocat"},
		Discovery: &stubDiscoveryService{
			repos: append([]domain.Repository(nil), repos...),
		},
		Dashboard: &stubDashboardService{
			dashboardByRepo: map[string]domain.DashboardSnapshot{},
			previewByPR:     map[string]domain.PRPreviewSnapshot{},
		},
		Search:          &stubSearchService{},
		PR:              prSvc,
		Root:            ".",
		Host:            "github.com",
		Now:             fixedNow,
		InitialPRNumber: initialPR,
	}
	m := NewModel(deps)
	m.state.Session.ViewerByHost["github.com"] = "octocat"
	return m
}

func deeplinkPreviewFixture() domain.PRPreviewSnapshot {
	return domain.PRPreviewSnapshot{
		Repo:        "acme/alpha",
		Number:      123,
		Title:       "Fix login flow",
		Author:      "octocat",
		State:       domain.PRStateOpen,
		CIStatus:    domain.CIStatusSuccess,
		BodyExcerpt: "The login button did nothing on Safari.\n\nThis fixes it by clearing the stale session cookie before mount.",
		CreatedAt:   fixedNow().Add(-48 * time.Hour),
		UpdatedAt:   fixedNow().Add(-2 * time.Hour),
		FileCount:   1,
		Additions:   12,
		Deletions:   3,
		Checks: []domain.PreviewCheckRow{
			{Name: "ci/build", State: "SUCCESS"},
			{Name: "ci/test", State: "SUCCESS"},
		},
	}
}

func deeplinkDiffFixture() diffmodel.DiffModel {
	return diffmodel.DiffModel{
		Repo:     "acme/alpha",
		PRNumber: 123,
		Files: []diffmodel.DiffFile{
			{
				OldPath:     "web/login.go",
				NewPath:     "web/login.go",
				Status:      "modified",
				Additions:   12,
				Deletions:   3,
				DisplayRows: 5,
				Hunks: []diffmodel.DiffHunk{
					{
						Header: "@@ -1,3 +1,4 @@",
						Lines: []diffmodel.DiffLine{
							{Kind: "context", Raw: " unchanged"},
							{Kind: "deletion", Raw: "-old line"},
							{Kind: "addition", Raw: "+new line"},
						},
					},
				},
			},
		},
		Stats: diffmodel.DiffStats{
			TotalFiles:     1,
			TotalAdditions: 12,
			TotalDeletions: 3,
		},
	}
}

// TestEndToEndDeepLinkPRDetail is the end-to-end snapshot test for
// `pho pr <number>`: model construction with a pending deep link, startup
// discovery, automatic PR-detail open, async detail/diff loads — all through
// the real Update path — then a golden snapshot of the final frame.
func TestEndToEndDeepLinkPRDetail(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prSvc := &stubPRService{
		detailResult: deeplinkPreviewFixture(),
		diffResult:   deeplinkDiffFixture(),
	}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{repo}, prSvc)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view after deep link, got %s\nview:\n%s", m.currentView(), m.View())
	}
	if m.prDetail == nil || m.prDetail.Summary.Number != 123 {
		t.Fatalf("expected prDetail for PR #123, got %+v", m.prDetail)
	}
	if m.prDetail.Repo.FullName != "acme/alpha" {
		t.Fatalf("expected repo acme/alpha, got %q", m.prDetail.Repo.FullName)
	}
	if m.prDetail.Detail == nil {
		t.Fatal("expected detail snapshot to be loaded")
	}
	if m.prDetail.Summary.Title != "Fix login flow" {
		t.Fatalf("expected header title backfilled from load, got %q", m.prDetail.Summary.Title)
	}
	if prSvc.loadDetailCalls != 1 {
		t.Fatalf("expected exactly 1 detail load, got %d", prSvc.loadDetailCalls)
	}
	if !strings.Contains(m.View(), "Fix login flow") {
		t.Fatalf("expected loaded title in rendered view, got:\n%s", m.View())
	}

	got := stripANSI(m.View())
	goldenPath := filepath.Join("testdata", "golden", "deeplink_pr_detail.txt")
	if *updateDeeplink {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update-deeplink to generate): %v", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for deeplink PR detail\ngot:\n%s\nwant:\n%s", got, string(want))
	}
}

// TestDeepLinkAmbiguousOpensRepoPicker verifies that a bare PR number with
// multiple unrelated discovered repos opens the repo picker instead of guessing.
func TestDeepLinkAmbiguousOpensRepoPicker(t *testing.T) {
	t.Parallel()

	repos := []domain.Repository{
		testutil.Repo("acme/alpha", testutil.WithLocalPath("/repos/alpha")),
		testutil.Repo("acme/beta", testutil.WithLocalPath("/repos/beta")),
	}
	m := newDeepLinkTestModel(t, 123, repos, &stubPRService{})
	m.deps.CWD = "/somewhere/else"

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	if m.pendingPRNumber != 123 {
		t.Fatalf("expected pending deep link retained for picker, got %d (overlayOpen=%v, focus=%s, discovered=%d, errors=%+v, view:\n%s)",
			m.pendingPRNumber, m.state.Search.OverlayOpen, m.focus, len(m.state.Repos.Discovered), m.state.Errors.Errors, m.View())
	}
	if !m.state.Search.OverlayOpen {
		t.Fatalf("expected repo picker overlay open, view:\n%s", m.View())
	}
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard under the picker, got %s", m.currentView())
	}
	view := m.View()
	if !strings.Contains(view, "Open PR #123 in:") {
		t.Fatalf("expected picker hint in view, got:\n%s", view)
	}
	if !strings.Contains(view, "acme/alpha") || !strings.Contains(view, "acme/beta") {
		t.Fatalf("expected both repos listed in picker, got:\n%s", view)
	}
}

// TestDeepLinkPickerSelectionOpensPR verifies choosing a repo in the picker
// opens the deep-linked PR there.
func TestDeepLinkPickerSelectionOpensPR(t *testing.T) {
	t.Parallel()

	repos := []domain.Repository{
		testutil.Repo("acme/alpha", testutil.WithLocalPath("/repos/alpha")),
		testutil.Repo("acme/beta", testutil.WithLocalPath("/repos/beta")),
	}
	m := newDeepLinkTestModel(t, 123, repos, &stubPRService{})
	m.deps.CWD = "/somewhere/else"

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	_, _ = m.Update(overlay.SelectRepo{Repo: "acme/beta"})

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view after picking a repo, got %s", m.currentView())
	}
	if m.prDetail == nil || m.prDetail.Summary.Number != 123 {
		t.Fatalf("expected prDetail for PR #123, got %+v", m.prDetail)
	}
	if m.prDetail.Repo.FullName != "acme/beta" {
		t.Fatalf("expected picked repo acme/beta, got %q", m.prDetail.Repo.FullName)
	}
	if m.pendingPRNumber != 0 {
		t.Fatalf("expected pending deep link consumed, got %d", m.pendingPRNumber)
	}
	if m.state.Search.OverlayOpen {
		t.Fatal("expected picker closed after selection")
	}
}

// TestDeepLinkPickerCancelFallsBackToDashboard verifies esc on the picker
// cancels the deep link and lands on the dashboard.
func TestDeepLinkPickerCancelFallsBackToDashboard(t *testing.T) {
	t.Parallel()

	repos := []domain.Repository{
		testutil.Repo("acme/alpha", testutil.WithLocalPath("/repos/alpha")),
		testutil.Repo("acme/beta", testutil.WithLocalPath("/repos/beta")),
	}
	m := newDeepLinkTestModel(t, 123, repos, &stubPRService{})
	m.deps.CWD = "/somewhere/else"

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	_, _ = m.Update(overlay.CloseCmdPalette{})

	if m.state.Search.OverlayOpen {
		t.Fatal("expected picker closed after cancel")
	}
	if m.pendingPRNumber != 0 {
		t.Fatalf("expected pending deep link cleared after cancel, got %d", m.pendingPRNumber)
	}
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard after cancel, got %s", m.currentView())
	}
}

// TestDeepLinkResolvesCWDRepo verifies the CWD repo wins when multiple repos
// are discovered.
func TestDeepLinkResolvesCWDRepo(t *testing.T) {
	t.Parallel()

	repos := []domain.Repository{
		testutil.Repo("acme/alpha", testutil.WithLocalPath("/repos/alpha")),
		testutil.Repo("acme/beta", testutil.WithLocalPath("/repos/beta")),
	}
	m := newDeepLinkTestModel(t, 123, repos, &stubPRService{})
	m.deps.CWD = "/repos/beta"

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	if m.prDetail == nil || m.prDetail.Repo.FullName != "acme/beta" {
		t.Fatalf("expected CWD repo acme/beta, got %+v", m.prDetail)
	}
}

// TestDeepLinkNoReposFallsBackToDashboard verifies the dashboard stays up with
// a recorded error when nothing was discovered.
func TestDeepLinkNoReposFallsBackToDashboard(t *testing.T) {
	t.Parallel()

	m := newDeepLinkTestModel(t, 123, nil, &stubPRService{})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard when no repos discovered, got %s", m.currentView())
	}
	if m.pendingPRNumber != 0 {
		t.Fatalf("expected pending deep link consumed, got %d", m.pendingPRNumber)
	}
	if len(m.state.Errors.Errors) == 0 {
		t.Fatalf("expected a recorded error, view:\n%s", m.View())
	}
	if !strings.Contains(m.state.Errors.Errors[0].Message, "PR #123") {
		t.Fatalf("expected error to mention PR #123, got %q", m.state.Errors.Errors[0].Message)
	}
}

// TestDeepLinkPRNotFoundShowsError verifies a nonexistent PR lands in the
// detail view's error state instead of spinning forever.
func TestDeepLinkPRNotFoundShowsError(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	prSvc := &stubPRService{
		detailErr: errors.New("GraphQL: Could not resolve to a PullRequest with the number 123"),
	}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{repo}, prSvc)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view (deep link opens before validity is known), got %s", m.currentView())
	}
	if m.prDetail == nil || m.prDetail.LoadErr == nil {
		t.Fatalf("expected LoadErr set on prDetail, got %+v", m.prDetail)
	}
	if !strings.Contains(m.View(), "Could not load PR #123") {
		t.Fatalf("expected load-error panel in view, got:\n%s", m.View())
	}
	if strings.Contains(stripANSI(m.View()), "Loading…") {
		t.Fatalf("expected no perpetual loading state, got:\n%s", stripANSI(m.View()))
	}
}

// TestDeepLinkCmdMessageRouting is a regression guard: the picker flow's
// pending number must not leak into a normal (non-deep-link) session.
func TestDeepLinkCmdMessageRouting(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newDeepLinkTestModel(t, 0, []domain.Repository{repo}, &stubPRService{})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())

	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("expected dashboard without deep link, got %s", m.currentView())
	}
	_, _ = m.Update(overlay.SelectRepo{Repo: repo.FullName})
	if m.currentView() != domain.PrimaryViewDashboard {
		t.Fatalf("normal SelectRepo must not open PR detail, got %s", m.currentView())
	}
	if m.pendingPRNumber != 0 {
		t.Fatalf("expected no pending deep link in a normal session, got %d", m.pendingPRNumber)
	}
}
