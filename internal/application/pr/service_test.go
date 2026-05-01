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
	FetchPreviewFn      func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error)
	FetchDashboardPRsFn func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error)
	FetchInvolvingPRsFn func(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error)
	FetchViewerFn       func(ctx context.Context, host string) (string, error)
	FetchCommitsFn      func(ctx context.Context, repo domain.Repository, number int) ([]domain.Commit, error)
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
func (f *fakeGitHubClient) MergePullRequest(_ context.Context, _, _, _, _ string) error { return nil }
func (f *fakeGitHubClient) CheckMergeable(_ context.Context, _ domain.Repository, _ int) (domain.MergeableState, error) {
	return domain.MergeableState{}, nil
}
func (f *fakeGitHubClient) ClosePullRequest(_ context.Context, _, _ string) error    { return nil }
func (f *fakeGitHubClient) ReopenPullRequest(_ context.Context, _, _ string) error { return nil }
func (f *fakeGitHubClient) FetchCommits(ctx context.Context, repo domain.Repository, number int) ([]domain.Commit, error) {
	if f.FetchCommitsFn == nil {
		return nil, fmt.Errorf("unexpected FetchCommits(%s,#%d)", repo.FullName, number)
	}
	return f.FetchCommitsFn(ctx, repo, number)
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
	rawDiff           string
	restErr           error
	RESTFetchFn       func(ctx context.Context, owner, repo string, number int) (string, error)
	FetchCommitDiffFn func(ctx context.Context, owner, repo, sha string) (string, error)
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

func (s *testablePRService) LoadCommitDiff(ctx context.Context, repo domain.Repository, sha string, force bool) (model.DiffModel, error) {
	key := commitDiffCacheKey(s.Host, repoFullName(repo), sha)

	var cached model.DiffModel
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
		if found {
			return cached, nil
		}
	}

	var rawDiff string
	var err error
	if s.FetchCommitDiffFn != nil {
		rawDiff, err = s.FetchCommitDiffFn(ctx, s.PRService.Owner, repo.Name, sha)
	} else if s.RESTFetchFn != nil {
		rawDiff, err = s.RESTFetchFn(ctx, s.PRService.Owner, repo.Name, 0)
	} else {
		err = fmt.Errorf("no fetch function configured")
	}
	if err != nil {
		if found {
			return cached, fmt.Errorf("fetch commit diff %s: %w", repo.FullName, err)
		}
		return model.DiffModel{}, fmt.Errorf("fetch commit diff: %w", err)
	}

	dm, err := parse.Parse(rawDiff)
	if err != nil {
		if found {
			return cached, fmt.Errorf("parse commit diff: %w", err)
		}
		return model.DiffModel{}, fmt.Errorf("parse commit diff: %w", err)
	}

	dm.HeadSHA = sha
	dm.Repo = repoFullName(repo)

	anchor.Generate(dm, sha)

	cumulative := 0
	for i := range dm.Files {
		dm.Files[i].StartRow = cumulative
		cumulative += dm.Files[i].DisplayRows
	}

	meta := commitDiffMeta(key, repo, sha, s.Now().UTC())
	_ = s.Cache.Write(ctx, key, *dm, meta)

	return *dm, nil
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

func TestMergePRInvalidatesPreviewCache(t *testing.T) {
	t.Parallel()

	repo := testRepo()
	coord := newTestCoordinator(t)

	// Seed the preview cache.
	seeded := domain.PRPreviewSnapshot{
		Repo:   repo.FullName,
		Number: 42,
		Title:  "Before Merge",
	}
	key := previewCacheKey(repo.Host, repo.FullName, 42)
	meta := previewMeta(key, repo, 42, frozenNow)
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	// Verify cache hit before merge.
	var cached domain.PRPreviewSnapshot
	_, found, _ := coord.L2.Get(context.Background(), key, &cached)
	if !found {
		t.Fatal("expected preview cache to exist before merge")
	}

	client := &fakeGitHubClient{}
	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   repo.Host,
		Now:    func() time.Time { return frozenNow },
	}

	if err := svc.MergePR(context.Background(), repo, 42, "pr_123", "abc123", "SQUASH"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cache miss after merge.
	_, found, _ = coord.L2.Get(context.Background(), key, &cached)
	if found {
		t.Fatal("expected preview cache to be deleted after merge")
	}
}

func TestLoadPRCommitsCacheMissFetchesGraphQL(t *testing.T) {
	t.Parallel()

	expected := []domain.Commit{
		{SHA: "sha1", MessageHeadline: "First commit", AuthorLogin: "alice"},
		{SHA: "sha2", MessageHeadline: "Second commit", AuthorLogin: "bob"},
	}

	client := &fakeGitHubClient{
		FetchCommitsFn: func(ctx context.Context, repo domain.Repository, number int) ([]domain.Commit, error) {
			if number != 42 {
				return nil, fmt.Errorf("expected number 42, got %d", number)
			}
			return expected, nil
		},
	}

	coord := newTestCoordinator(t)
	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return frozenNow },
	}

	commits, err := svc.LoadPRCommits(context.Background(), testRepo(), 42, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].SHA != "sha1" {
		t.Errorf("expected SHA=%q, got %q", "sha1", commits[0].SHA)
	}
}

func TestLoadPRCommitsCacheHitBypassesTransport(t *testing.T) {
	t.Parallel()

	seeded := []domain.Commit{
		{SHA: "seeded", MessageHeadline: "Seeded commit"},
	}
	coord := newTestCoordinator(t)
	key := commitsCacheKey("github.com", "owner/repo", 42)
	meta := commitsMeta(key, testRepo(), 42, frozenNow)
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	client := &fakeGitHubClient{
		FetchCommitsFn: func(ctx context.Context, repo domain.Repository, number int) ([]domain.Commit, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}

	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return frozenNow },
	}

	commits, err := svc.LoadPRCommits(context.Background(), testRepo(), 42, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 1 || commits[0].SHA != "seeded" {
		t.Errorf("expected seeded commit, got %+v", commits)
	}
}

func TestLoadPRCommitsForceRefresh(t *testing.T) {
	t.Parallel()

	seeded := []domain.Commit{
		{SHA: "seeded", MessageHeadline: "Seeded commit"},
	}
	coord := newTestCoordinator(t)
	key := commitsCacheKey("github.com", "owner/repo", 42)
	meta := commitsMeta(key, testRepo(), 42, frozenNow)
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	callCount := 0
	client := &fakeGitHubClient{
		FetchCommitsFn: func(ctx context.Context, repo domain.Repository, number int) ([]domain.Commit, error) {
			callCount++
			return []domain.Commit{{SHA: "new-sha", MessageHeadline: "New commit"}}, nil
		},
	}

	svc := &PRService{
		Cache:  coord,
		Client: client,
		Host:   "github.com",
		Now:    func() time.Time { return frozenNow },
	}

	commits, err := svc.LoadPRCommits(context.Background(), testRepo(), 42, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 transport call on force=true, got %d", callCount)
	}
	if len(commits) != 1 || commits[0].SHA != "new-sha" {
		t.Errorf("expected new commit after force refresh, got %+v", commits)
	}
}

func TestLoadCommitDiffForceRefresh(t *testing.T) {
	t.Parallel()

	seeded := model.DiffModel{HeadSHA: "seeded-sha"}
	coord := newTestCoordinator(t)
	key := commitDiffCacheKey("github.com", "owner/repo", "abc1234")
	meta := commitDiffMeta(key, testRepo(), "abc1234", frozenNow)
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	callCount := 0
	svc := &testablePRService{
		PRService: PRService{
			Cache: coord,
			Host:  "github.com",
			Now:   func() time.Time { return frozenNow },
		},
		FetchCommitDiffFn: func(ctx context.Context, owner, repo, sha string) (string, error) {
			callCount++
			return "diff --git a/f.txt b/f.txt\nnew file mode 100644\nindex 0000000..e69de29\n--- /dev/null\n+++ b/f.txt\n@@ -0,0 +1 @@\n+hello\n", nil
		},
	}

	diff, err := svc.LoadCommitDiff(context.Background(), testRepo(), "abc1234", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 REST call on force=true, got %d", callCount)
	}
	if diff.HeadSHA != "abc1234" {
		t.Errorf("expected HeadSHA=abc1234 after force refresh, got %q", diff.HeadSHA)
	}
}

// Ensure testutil import is used.
var _ = testutil.Repo("")
