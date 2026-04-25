package pr

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

type fakeGitHubClient struct {
	FetchPreviewFn        func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error)
	FetchDashboardPRsFn   func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error)
	FetchInvolvingPRsFn   func(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error)
	FetchRecentActivityFn func(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error)
	FetchViewerFn         func(ctx context.Context, host string) (string, error)
}

func (f *fakeGitHubClient) FetchViewer(ctx context.Context, host string) (string, error) {
	if f.FetchViewerFn == nil {
		return "", nil
	}
	return f.FetchViewerFn(ctx, host)
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

func (f *fakeGitHubClient) PostComment(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *fakeGitHubClient) PostReviewComment(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeGitHubClient) ApprovePullRequest(_ context.Context, _, _, _ string) error {
	return nil
}
func (f *fakeGitHubClient) SubmitReviewWithComments(_ context.Context, _, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}

func (f *fakeGitHubClient) FetchAllPRs(_ context.Context, _ domain.Repository, _ string) ([]domain.PullRequestSummary, bool, string, error) {
	return nil, false, "", nil
}

// frozenNow is the fixed time used for both service.Now and coord.Now in tests.
// Entries seeded at this time with a 2-minute TTL are fresh; entries seeded
// at 2020 are still stale, so stale-path tests remain valid.
var frozenNow = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

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
	coord := cache.NewCoordinator(l1, l2, nil)
	coord.Now = func() time.Time { return frozenNow }
	return coord
}

func testRepo() domain.Repository {
	return domain.Repository{
		Host:     "github.com",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
	}
}

type prServiceTransport interface {
	LoadDetailFromCache(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error)
	LoadDiffFromCache(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error)
}

func TestLoadDetailCacheMissFetchesGraphQL(t *testing.T) {
	t.Parallel()

	expected := domain.PRPreviewSnapshot{
		Repo:   "owner/repo",
		Number: 42,
		Title:  "Test PR",
	}

	client := &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			if number != 42 {
				return domain.PRPreviewSnapshot{}, fmt.Errorf("expected number 42, got %d", number)
			}
			return expected, nil
		},
	}

	coord := newTestCoordinator(t)
	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Owner:  "owner",
		Repo:   "repo",
		Now:    func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	detail, fromCache, err := svc.LoadDetail(context.Background(), testRepo(), 42, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fromCache {
		t.Error("expected fromCache=false on cache miss")
	}
	if detail.Title != "Test PR" {
		t.Errorf("expected title=%q, got %q", "Test PR", detail.Title)
	}
}

func TestLoadDetailCacheHitBypassesTransport(t *testing.T) {
	t.Parallel()

	// Seed the cache manually.
	seeded := domain.PRPreviewSnapshot{
		Repo:   "owner/repo",
		Number: 42,
		Title:  "Seeded PR",
	}

	coord := newTestCoordinator(t)

	// Write directly to cache.
	key := previewCacheKey("github.com", "owner/repo", 42)
	meta := previewMeta(key, testRepo(), 42, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	callCount := 0
	client := &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			callCount++
			return domain.PRPreviewSnapshot{}, fmt.Errorf("should not be called")
		},
	}

	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	detail, fromCache, err := svc.LoadDetail(context.Background(), testRepo(), 42, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fromCache {
		t.Error("expected fromCache=true on cache hit")
	}
	if detail.Title != "Seeded PR" {
		t.Errorf("expected title=%q, got %q", "Seeded PR", detail.Title)
	}
	if callCount != 0 {
		t.Errorf("expected 0 transport calls, got %d", callCount)
	}
}

func TestLoadDetailForceRefresh(t *testing.T) {
	t.Parallel()

	// Seed the cache.
	seeded := domain.PRPreviewSnapshot{Repo: "owner/repo", Number: 42, Title: "Old"}
	coord := newTestCoordinator(t)
	key := previewCacheKey("github.com", "owner/repo", 42)
	meta := previewMeta(key, testRepo(), 42, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	callCount := 0
	client := &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			callCount++
			return domain.PRPreviewSnapshot{Repo: "owner/repo", Number: 42, Title: "New"}, nil
		},
	}

	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	detail, _, err := svc.LoadDetail(context.Background(), testRepo(), 42, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 transport call on force=true, got %d", callCount)
	}
	if detail.Title != "New" {
		t.Errorf("expected title=%q after force refresh, got %q", "New", detail.Title)
	}
}

func TestLoadDetailErrorWithNoStale(t *testing.T) {
	t.Parallel()

	client := &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			return domain.PRPreviewSnapshot{}, fmt.Errorf("network error")
		},
	}

	coord := newTestCoordinator(t)
	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	_, _, err := svc.LoadDetail(context.Background(), testRepo(), 42, false)
	if err == nil {
		t.Fatal("expected error when cache miss and transport fails")
	}
}

func TestLoadDetailReturnsStaleOnBackgroundRefresh(t *testing.T) {
	t.Parallel()

	// Seed stale data.
	seeded := domain.PRPreviewSnapshot{Repo: "owner/repo", Number: 42, Title: "Stale"}
	coord := newTestCoordinator(t)
	key := previewCacheKey("github.com", "owner/repo", 42)
	// Use an expired TTL so the data is stale and triggers background refresh.
	meta := previewMeta(key, testRepo(), 42, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	// Track if background fetch was scheduled.
	backgroundScheduled := make(chan struct{}, 1)
	client := &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			return domain.PRPreviewSnapshot{}, fmt.Errorf("network error")
		},
	}

	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
		BackgroundFn: func(fn func()) {
			backgroundScheduled <- struct{}{}
			// Execute synchronously for test determinism.
			fn()
		},
	}

	detail, fromCache, err := svc.LoadDetail(context.Background(), testRepo(), 42, false)
	// Stale-while-revalidate: returns stale data with no error.
	// Background refresh is scheduled but may fail silently.
	if err != nil {
		t.Fatalf("expected no error (stale-while-revalidate), got %v", err)
	}
	if !fromCache {
		t.Error("expected fromCache=true when returning stale data")
	}
	if detail.Title != "Stale" {
		t.Errorf("expected stale title=%q, got %q", "Stale", detail.Title)
	}
	// Verify background refresh was scheduled.
	select {
	case <-backgroundScheduled:
		// Good — background refresh was scheduled.
	default:
		t.Error("expected background refresh to be scheduled for stale data")
	}
}

func TestLoadDiffParsesAndCaches(t *testing.T) {
	t.Parallel()

	rawDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	coord := newTestCoordinator(t)

	svc := newTestPRService(coord, rawDiff, nil)

	diff, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc123", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fromCache {
		t.Error("expected fromCache=false on first load")
	}
	if len(diff.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(diff.Files))
	}
	if diff.HeadSHA != "abc123" {
		t.Errorf("expected HeadSHA=%q, got %q", "abc123", diff.HeadSHA)
	}
}

func TestLoadDiffCacheHit(t *testing.T) {
	t.Parallel()

	rawDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	coord := newTestCoordinator(t)
	svc := newTestPRService(coord, rawDiff, nil)

	// First load — populates cache.
	_, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc123", false)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}

	// Second load — should hit cache.
	callCount := 0
	svc.RESTFetchFn = func(ctx context.Context, owner, repo string, number int) (string, error) {
		callCount++
		return rawDiff, nil
	}

	diff, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc123", false)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !fromCache {
		t.Error("expected fromCache=true on cache hit")
	}
	if callCount != 0 {
		t.Errorf("expected 0 REST calls on cache hit, got %d", callCount)
	}
	if len(diff.Files) != 1 {
		t.Errorf("expected 1 file from cache, got %d", len(diff.Files))
	}
}

func TestLoadDiffSHAValidation(t *testing.T) {
	t.Parallel()

	rawDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	coord := newTestCoordinator(t)
	svc := newTestPRService(coord, rawDiff, nil)

	// Use a different SHA than what the service would set from GraphQL HeadRefOID.
	// The service sets dm.HeadSHA = headSHA (the expected SHA from GraphQL).
	// This test verifies that when they match, validation passes.
	diff, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "full40charSHA", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.HeadSHA != "full40charSHA" {
		t.Errorf("expected HeadSHA=%q, got %q", "full40charSHA", diff.HeadSHA)
	}
}

func TestLoadDiffSHAValidationSkippedIfEmpty(t *testing.T) {
	t.Parallel()

	rawDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	coord := newTestCoordinator(t)
	svc := newTestPRService(coord, rawDiff, nil)

	// Empty headSHA — validation should be skipped.
	diff, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "", false)
	if err != nil {
		t.Fatalf("unexpected error with empty SHA: %v", err)
	}
	if diff.HeadSHA != "" {
		t.Errorf("expected empty HeadSHA, got %q", diff.HeadSHA)
	}
}

func newTestPRService(coord *cache.Coordinator, rawDiff string, restErr error) *testablePRService {
	svc := &testablePRService{
		PRService: PRService{
			Cache: coord,
			Host:  "github.com",
			Now:   func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
		},
		rawDiff: rawDiff,
		restErr: restErr,
		RESTFetchFn: func(ctx context.Context, owner, repo string, number int) (string, error) {
			return rawDiff, restErr
		},
	}
	svc.PRService.Client = &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			return domain.PRPreviewSnapshot{
				Repo:   repo.FullName,
				Number: number,
			}, nil
		},
	}
	return svc
}

// testablePRService embeds PRService but allows overriding REST fetch.
type testablePRService struct {
	PRService
	rawDiff     string
	restErr     error
	RESTFetchFn func(ctx context.Context, owner, repo string, number int) (string, error)
}

func (s *testablePRService) LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	key := diffCacheKey(s.Host, repoFullName(repo), number, headSHA)

	var cached model.DiffModel
	found := false
	if !force && headSHA != "" {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, func(string) {
			s.spawnBackground(func() {
				_, _, _ = s.LoadDiff(context.Background(), repo, number, headSHA, true)
			})
		})
		if found {
			return cached, true, nil
		}
	} else if force && headSHA != "" {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	rawDiff, err := s.RESTFetchFn(ctx, s.PRService.Owner, repo.Name, number)
	if err != nil {
		if found && headSHA != "" {
			return cached, true, fmt.Errorf("refresh diff %s: %w", repo.FullName, err)
		}
		return model.DiffModel{}, false, fmt.Errorf("fetch raw diff: %w", err)
	}

	dm, err := parse.Parse(rawDiff)
	if err != nil {
		if found {
			return cached, true, fmt.Errorf("parse diff: %w", err)
		}
		return model.DiffModel{}, false, fmt.Errorf("parse diff: %w", err)
	}

	dm.HeadSHA = headSHA
	dm.Repo = repoFullName(repo)
	dm.PRNumber = number

	if headSHA != "" && dm.HeadSHA != "" && dm.HeadSHA != headSHA {
		return model.DiffModel{}, false, nil
	}

	anchor.Generate(dm, headSHA)

	cumulative := 0
	for i := range dm.Files {
		dm.Files[i].StartRow = cumulative
		cumulative += dm.Files[i].DisplayRows
	}

	if headSHA != "" {
		meta := diffMeta(key, repo, number, s.Now().UTC())
		_ = s.Cache.Write(ctx, key, *dm, meta)
	}

	return *dm, false, nil
}

func TestLoadDiffErrorWithNoStale(t *testing.T) {
	t.Parallel()

	coord := newTestCoordinator(t)
	svc := &testablePRService{
		PRService: PRService{
			Cache: coord,
			Host:  "github.com",
			Now:   func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
		},
		restErr: fmt.Errorf("network error"),
		RESTFetchFn: func(ctx context.Context, owner, repo string, number int) (string, error) {
			return "", fmt.Errorf("network error")
		},
	}

	_, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc123", false)
	if err == nil {
		t.Fatal("expected error when cache miss and REST fails")
	}
}

func TestLoadDiffReturnsStaleOnBackgroundRefresh(t *testing.T) {
	t.Parallel()

	coord := newTestCoordinator(t)

	// Seed cache with stale diff.
	seeded := model.DiffModel{HeadSHA: "abc123"}
	key := diffCacheKey("github.com", "owner/repo", 42, "abc123")
	// Use expired TTL.
	meta := diffMeta(key, testRepo(), 42, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	// Track if background fetch was scheduled.
	backgroundScheduled := make(chan struct{}, 1)

	svc := &testablePRService{
		PRService: PRService{
			Cache: coord,
			Host:  "github.com",
			Now:   func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
			BackgroundFn: func(fn func()) {
				backgroundScheduled <- struct{}{}
				fn()
			},
		},
		RESTFetchFn: func(ctx context.Context, owner, repo string, number int) (string, error) {
			return "", fmt.Errorf("network error")
		},
	}

	diff, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, "abc123", false)
	// Stale-while-revalidate: returns stale data with no error.
	if err != nil {
		t.Fatalf("expected no error (stale-while-revalidate), got %v", err)
	}
	if !fromCache {
		t.Error("expected fromCache=true when returning stale")
	}
	if diff.HeadSHA != "abc123" {
		t.Errorf("expected stale HeadSHA=%q, got %q", "abc123", diff.HeadSHA)
	}
	// Verify background refresh was scheduled.
	select {
	case <-backgroundScheduled:
	default:
		t.Error("expected background refresh to be scheduled for stale data")
	}
}

func TestLoadDiffSHAValidationMismatch(t *testing.T) {
	t.Parallel()

	// Verify that the cache key includes SHA,
	// so a different SHA = different cache entry = no collision.
	coord := newTestCoordinator(t)
	rawDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	svc := newTestPRService(coord, rawDiff, nil)

	// Load with SHA "sha1".
	_, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, "sha1", false)
	if err != nil {
		t.Fatalf("load with sha1: %v", err)
	}

	// Load with SHA "sha2" — should NOT hit the sha1 cache.
	callCount := 0
	svc.RESTFetchFn = func(ctx context.Context, owner, repo string, number int) (string, error) {
		callCount++
		return rawDiff, nil
	}

	_, _, err = svc.LoadDiff(context.Background(), testRepo(), 42, "sha2", false)
	if err != nil {
		t.Fatalf("load with sha2: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 REST call for sha2 (different cache key), got %d", callCount)
	}
}

func TestLoadDetailSharedCacheKey(t *testing.T) {
	t.Parallel()

	// Seed the preview cache (simulating a dashboard hover).
	seeded := domain.PRPreviewSnapshot{
		Repo:   "owner/repo",
		Number: 42,
		Title:  "Hovered PR",
	}
	coord := newTestCoordinator(t)
	key := previewCacheKey("github.com", "owner/repo", 42)
	meta := previewMeta(key, testRepo(), 42, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	// Now LoadDetail should hit the same cache.
	client := &fakeGitHubClient{
		FetchPreviewFn: func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			return domain.PRPreviewSnapshot{}, fmt.Errorf("should not be called")
		},
	}

	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	detail, fromCache, err := svc.LoadDetail(context.Background(), testRepo(), 42, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fromCache {
		t.Error("expected fromCache=true (shared with dashboard hover)")
	}
	if detail.Title != "Hovered PR" {
		t.Errorf("expected title=%q, got %q", "Hovered PR", detail.Title)
	}
}

// Ensure testutil import is used.
var _ = testutil.Repo("")
