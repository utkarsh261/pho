package dashboard

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

type fakeGitHubClient struct {
	FetchDashboardPRsFn   func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error)
	FetchInvolvingPRsFn   func(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error)
	FetchRecentActivityFn func(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error)
	FetchPreviewFn        func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error)
}

func (f *fakeGitHubClient) FetchViewer(ctx context.Context, host string) (string, error) {
	return "", fmt.Errorf("unexpected FetchViewer(%s)", host)
}

func (f *fakeGitHubClient) FetchDashboardPRs(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
	if f.FetchDashboardPRsFn == nil {
		return nil, 0, false, "", fmt.Errorf("unexpected FetchDashboardPRs(%s)", repo.FullName)
	}
	return f.FetchDashboardPRsFn(ctx, repo)
}

func (f *fakeGitHubClient) FetchInvolvingPRs(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error) {
	if f.FetchInvolvingPRsFn == nil {
		return nil, 0, false, fmt.Errorf("unexpected FetchInvolvingPRs(%s,%s)", repo.FullName, viewer)
	}
	return f.FetchInvolvingPRsFn(ctx, repo, viewer)
}

func (f *fakeGitHubClient) FetchRecentActivity(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error) {
	if f.FetchRecentActivityFn == nil {
		return nil, fmt.Errorf("unexpected FetchRecentActivity(%s)", repo.FullName)
	}
	return f.FetchRecentActivityFn(ctx, repo)
}

func (f *fakeGitHubClient) FetchPreview(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
	if f.FetchPreviewFn == nil {
		return domain.PRPreviewSnapshot{}, fmt.Errorf("unexpected FetchPreview(%s,#%d)", repo.FullName, number)
	}
	return f.FetchPreviewFn(ctx, repo, number)
}

func (f *fakeGitHubClient) PostComment(_ context.Context, _, _, _ string) error       { return nil }
func (f *fakeGitHubClient) PostReviewComment(_ context.Context, _, _, _ string) error  { return nil }
func (f *fakeGitHubClient) ApprovePullRequest(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeGitHubClient) SubmitReviewWithComments(_ context.Context, _, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}

func (f *fakeGitHubClient) FetchAllPRs(_ context.Context, _ domain.Repository, _ string) ([]domain.PullRequestSummary, bool, string, error) {
	return nil, false, "", nil
}

func newTestCoordinator(t *testing.T) *cache.Coordinator {
	t.Helper()

	l1 := memorycache.NewJSONStore(1024 * 1024)
	l2, err := sqlitecache.New(filepath.Join(t.TempDir(), "cache.db"), 1)
	if err != nil {
		t.Fatalf("new sqlite cache: %v", err)
	}
	t.Cleanup(func() {
		_ = l2.Close()
	})
	return cache.NewCoordinator(l1, l2, nil)
}

func TestSelectInitialRepo(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	repos := []domain.Repository{
		testutil.Repo("acme/alpha", testutil.WithLocalPath("/workspace/alpha")),
		testutil.Repo("acme/beta", testutil.WithLocalPath("/workspace/beta")),
	}

	got, err := svc.SelectInitialRepo(repos, "/workspace/beta")
	if err != nil {
		t.Fatalf("select cwd repo: %v", err)
	}
	if got == nil || got.FullName != "acme/beta" {
		t.Fatalf("got %#v, want acme/beta", got)
	}

	got, err = svc.SelectInitialRepo(repos, "/workspace/does-not-exist")
	if err != nil {
		t.Fatalf("select fallback repo: %v", err)
	}
	if got == nil || got.FullName != "acme/alpha" {
		t.Fatalf("got %#v, want acme/alpha", got)
	}
}

func TestSelectInitialRepoEmpty(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	got, err := svc.SelectInitialRepo(nil, "/workspace/alpha")
	if err == nil {
		t.Fatalf("expected error, got repo %#v", got)
	}
}

func TestLoadRepoWarmCacheBypassesTransport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	coord := newTestCoordinator(t)
	repo := testutil.Repo("acme/api", testutil.WithLocalPath("/workspace/api"))
	cached := testutil.DashboardSnap(repo, testutil.PR(1), testutil.PR(2))
	key := dashboardCacheKey(repo, "prs")
	if err := coord.Write(ctx, key, cached, dashboardMeta(key, repo, cacheKindDashboardPRs, nil, time.Now().UTC())); err != nil {
		t.Fatalf("seed dashboard cache: %v", err)
	}

	clientCalls := 0
	svc := &Service{
		Cache: coord,
		Client: &fakeGitHubClient{
			FetchDashboardPRsFn: func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
				clientCalls++
				return nil, 0, false, "", fmt.Errorf("unexpected transport call")
			},
		},
		Now: time.Now,
	}

	got, err := svc.LoadRepo(ctx, repo, false)
	if err != nil {
		t.Fatalf("load warm cache: %v", err)
	}
	if clientCalls != 0 {
		t.Fatalf("expected no transport calls, got %d", clientCalls)
	}
	if len(got.PRs) != len(cached.PRs) {
		t.Fatalf("expected %d prs, got %d", len(cached.PRs), len(got.PRs))
	}
}

func TestLoadRepoForceRefreshUpdatesCache(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	coord := newTestCoordinator(t)
	repo := testutil.Repo("acme/api", testutil.WithLocalPath("/workspace/api"))
	stale := testutil.DashboardSnap(repo, testutil.PR(1))
	key := dashboardCacheKey(repo, "prs")
	if err := coord.Write(ctx, key, stale, dashboardMeta(key, repo, cacheKindDashboardPRs, nil, time.Now().UTC())); err != nil {
		t.Fatalf("seed dashboard cache: %v", err)
	}

	fresh := testutil.DashboardSnap(repo, testutil.PR(10), testutil.PR(11))
	clientCalls := 0
	svc := &Service{
		Cache: coord,
		Client: &fakeGitHubClient{
			FetchDashboardPRsFn: func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
				clientCalls++
				return fresh.PRs, fresh.TotalCount, fresh.Truncated, fresh.EndCursor, nil
			},
		},
		Now: time.Now,
	}

	got, err := svc.LoadRepo(ctx, repo, true)
	if err != nil {
		t.Fatalf("force refresh: %v", err)
	}
	if clientCalls != 1 {
		t.Fatalf("expected 1 transport call, got %d", clientCalls)
	}
	if len(got.PRs) != len(fresh.PRs) {
		t.Fatalf("expected %d prs, got %d", len(fresh.PRs), len(got.PRs))
	}

	var fromCache domain.DashboardSnapshot
	_, _, found, err := coord.StaleWhileRevalidate(ctx, key, &fromCache, nil)
	if err != nil {
		t.Fatalf("read refreshed cache: %v", err)
	}
	if !found {
		t.Fatalf("expected refreshed cache entry")
	}
	if len(fromCache.PRs) != len(fresh.PRs) {
		t.Fatalf("expected cache to contain %d prs, got %d", len(fresh.PRs), len(fromCache.PRs))
	}

	_, err = svc.LoadRepo(ctx, repo, false)
	if err != nil {
		t.Fatalf("warm read after refresh: %v", err)
	}
	if clientCalls != 1 {
		t.Fatalf("expected cached read after refresh, transport calls=%d", clientCalls)
	}
}

func TestStaleWhileRevalidateDoesNotCascade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	coord := newTestCoordinator(t)
	repo := testutil.Repo("acme/api", testutil.WithLocalPath("/workspace/api"))

	// Seed a stale cache entry: fetched 3 minutes ago, TTL is 2 minutes.
	staleAt := time.Now().Add(-3 * time.Minute)
	stale := testutil.DashboardSnap(repo, testutil.PR(1))
	key := dashboardCacheKey(repo, "prs")
	if err := coord.Write(ctx, key, stale, dashboardMeta(key, repo, cacheKindDashboardPRs, nil, staleAt)); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	clientCalls := 0
	fresh := testutil.DashboardSnap(repo, testutil.PR(10))
	svc := &Service{
		Cache: coord,
		Client: &fakeGitHubClient{
			FetchDashboardPRsFn: func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
				clientCalls++
				return fresh.PRs, fresh.TotalCount, fresh.Truncated, fresh.EndCursor, nil
			},
		},
		Now: time.Now,
	}

	// Capture background spawns instead of running goroutines.
	var spawnCount int
	var capturedFns []func()
	svc.BackgroundFn = func(fn func()) {
		spawnCount++
		capturedFns = append(capturedFns, fn)
	}

	// LoadRepo(force=false) on a stale entry: returns stale data immediately
	// and schedules exactly one background refresh.
	got, err := svc.LoadRepo(ctx, repo, false)
	if err != nil {
		t.Fatalf("load stale: %v", err)
	}
	if len(got.PRs) != len(stale.PRs) {
		t.Fatalf("expected stale PRs to be returned, got %d", len(got.PRs))
	}
	if clientCalls != 0 {
		t.Fatalf("stale hit must not call transport synchronously, got %d calls", clientCalls)
	}
	if spawnCount != 1 {
		t.Fatalf("expected 1 background spawn from stale hit, got %d", spawnCount)
	}

	// Run the spawned background refresh synchronously.
	for _, fn := range capturedFns {
		fn()
	}

	// The background refresh (force=true) must make exactly one network call
	// and must NOT schedule another background refresh.
	if clientCalls != 1 {
		t.Fatalf("background refresh must call transport exactly once, got %d", clientCalls)
	}
	if spawnCount != 1 {
		t.Fatalf("background refresh must not cascade into another spawn, got %d total spawns", spawnCount)
	}
}

func TestForceRefreshNeverSpawnsBackground(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	coord := newTestCoordinator(t)
	repo := testutil.Repo("acme/api", testutil.WithLocalPath("/workspace/api"))

	// Seed a stale cache entry.
	staleAt := time.Now().Add(-3 * time.Minute)
	stale := testutil.DashboardSnap(repo, testutil.PR(1))
	key := dashboardCacheKey(repo, "prs")
	if err := coord.Write(ctx, key, stale, dashboardMeta(key, repo, cacheKindDashboardPRs, nil, staleAt)); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	clientCalls := 0
	svc := &Service{
		Cache: coord,
		Client: &fakeGitHubClient{
			FetchDashboardPRsFn: func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
				clientCalls++
				return nil, 0, false, "", nil
			},
		},
		Now: time.Now,
		BackgroundFn: func(fn func()) {
			t.Errorf("force=true must never spawn a background goroutine")
		},
	}

	if _, err := svc.LoadRepo(ctx, repo, true); err != nil {
		t.Fatalf("force refresh: %v", err)
	}
	if clientCalls != 1 {
		t.Fatalf("expected exactly 1 transport call, got %d", clientCalls)
	}
}

func TestLoadPreviewWarmCacheBypassesTransport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	coord := newTestCoordinator(t)
	repo := "acme/api"
	parsedRepo := domain.Repository{Host: defaultPreviewHost, Owner: "acme", Name: "api", FullName: repo}
	key := previewCacheKey(parsedRepo, 42)
	cached := domain.PRPreviewSnapshot{
		Repo:   repo,
		Number: 42,
		Title:  "cached",
	}
	if err := coord.Write(ctx, key, cached, dashboardMeta(key, parsedRepo, cacheKindPreview, &cached.Number, time.Now().UTC())); err != nil {
		t.Fatalf("seed preview cache: %v", err)
	}

	calls := 0
	svc := &Service{
		Cache: coord,
		Client: &fakeGitHubClient{
			FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
				calls++
				return domain.PRPreviewSnapshot{}, fmt.Errorf("unexpected preview transport call")
			},
		},
		DefaultHost: defaultPreviewHost,
		Now:         time.Now,
	}

	got, err := svc.LoadPreview(ctx, repo, 42)
	if err != nil {
		t.Fatalf("warm preview load: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no preview transport calls, got %d", calls)
	}
	if got.Title != cached.Title {
		t.Fatalf("expected cached preview, got %#v", got)
	}
}
