package pr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
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

func (f *fakeGitHubClient) PostCommentReply(_ context.Context, _, _, _, _ string) error { return nil }
func (f *fakeGitHubClient) PostReviewComment(_ context.Context, _, _, _ string) error   { return nil }
func (f *fakeGitHubClient) PostThreadReply(_ context.Context, _, _, _ string) error     { return nil }
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
func (f *fakeGitHubClient) ClosePullRequest(_ context.Context, _, _ string) error  { return nil }
func (f *fakeGitHubClient) ReopenPullRequest(_ context.Context, _, _ string) error { return nil }
func (f *fakeGitHubClient) FetchCommits(ctx context.Context, repo domain.Repository, number int) ([]domain.Commit, error) {
	if f.FetchCommitsFn == nil {
		return nil, fmt.Errorf("unexpected FetchCommits(%s,#%d)", repo.FullName, number)
	}
	return f.FetchCommitsFn(ctx, repo, number)
}

func (f *fakeGitHubClient) UpdatePullRequest(_ context.Context, _, _, _, _ string) error { return nil }
func (f *fakeGitHubClient) ResolveReviewThread(_ context.Context, _, _ string) error     { return nil }
func (f *fakeGitHubClient) UnresolveReviewThread(_ context.Context, _, _ string) error   { return nil }

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

	select {
	case <-backgroundScheduled:
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
	key := diffCacheKey(repo.Host, repoFullName(repo), number, headSHA)

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
	key := commitDiffCacheKey(repo.Host, repoFullName(repo), sha)

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

func TestFetchRepoInfoSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"default_branch": "main",
			"fork": false
		}`))
	}))
	defer server.Close()

	coord := newTestCoordinator(t)
	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Owner: "owner",
		Repo:  "repo",
		Log:   pholog.NewNop(),
	}

	info, err := svc.FetchRepoInfo(context.Background(), testRepo())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DefaultBranch != "main" {
		t.Errorf("expected default_branch=main, got %q", info.DefaultBranch)
	}
	if info.Fork {
		t.Error("expected fork=false")
	}
	if info.ParentFullName != "" {
		t.Errorf("expected empty parent, got %q", info.ParentFullName)
	}
}

func TestFetchRepoInfoForkWithParent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"default_branch": "develop",
			"fork": true,
			"parent": {
				"full_name": "upstream-org/upstream-repo"
			}
		}`))
	}))
	defer server.Close()

	coord := newTestCoordinator(t)
	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Owner: "fork-owner",
		Repo:  "fork-repo",
		Log:   pholog.NewNop(),
	}

	info, err := svc.FetchRepoInfo(context.Background(), domain.Repository{
		Host:     "github.com",
		Owner:    "fork-owner",
		Name:     "fork-repo",
		FullName: "fork-owner/fork-repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Fork {
		t.Error("expected fork=true")
	}
	if info.ParentFullName != "upstream-org/upstream-repo" {
		t.Errorf("expected parent=upstream-org/upstream-repo, got %q", info.ParentFullName)
	}
}

func TestFetchRepoInfoError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	coord := newTestCoordinator(t)
	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Owner: "owner",
		Repo:  "repo",
		Log:   pholog.NewNop(),
	}

	_, err := svc.FetchRepoInfo(context.Background(), testRepo())
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestCreatePRSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"number": 42,
			"title": "Add feature",
			"body": "Description",
			"state": "open",
			"html_url": "https://github.com/owner/repo/pull/42",
			"head": {"ref": "feature-branch"},
			"base": {"ref": "main"},
			"draft": false,
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"user": {"login": "testuser"}
		}`))
	}))
	defer server.Close()

	coord := newTestCoordinator(t)
	// Seed dashboard cache to verify invalidation.
	dashKey := "dashboard:v1:host=github.com:repo=owner/repo"
	_ = coord.Write(context.Background(), dashKey, []domain.PullRequestSummary{}, domain.CacheMeta{})

	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Owner: "owner",
		Repo:  "repo",
		Now:   func() time.Time { return frozenNow },
		Log:   pholog.NewNop(),
	}

	params := domain.CreatePRParams{
		Repo:  testRepo(),
		Title: "Add feature",
		Body:  "Description",
		Head:  "feature-branch",
		Base:  "main",
		Draft: false,
	}

	summary, err := svc.CreatePR(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Number != 42 {
		t.Errorf("expected number=42, got %d", summary.Number)
	}
	if summary.Title != "Add feature" {
		t.Errorf("expected title=%q, got %q", "Add feature", summary.Title)
	}
	if summary.HeadRefName != "feature-branch" {
		t.Errorf("expected head=%q, got %q", "feature-branch", summary.HeadRefName)
	}
	if summary.BaseRefName != "main" {
		t.Errorf("expected base=%q, got %q", "main", summary.BaseRefName)
	}
	if summary.IsDraft {
		t.Error("expected draft=false")
	}

	// Verify dashboard cache was invalidated.
	_, found, _ := coord.L2.Get(context.Background(), dashKey, &[]domain.PullRequestSummary{})
	if found {
		t.Error("expected dashboard cache to be invalidated after PR creation")
	}
}

func TestCreatePRDraft(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"number": 1,
			"title": "WIP: Feature",
			"state": "open",
			"html_url": "https://github.com/owner/repo/pull/1",
			"head": {"ref": "wip-branch"},
			"base": {"ref": "main"},
			"draft": true,
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"user": {"login": "testuser"}
		}`))
	}))
	defer server.Close()

	coord := newTestCoordinator(t)
	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Owner: "owner",
		Repo:  "repo",
		Log:   pholog.NewNop(),
	}

	params := domain.CreatePRParams{
		Repo:  testRepo(),
		Title: "WIP: Feature",
		Head:  "wip-branch",
		Base:  "main",
		Draft: true,
	}

	summary, err := svc.CreatePR(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summary.IsDraft {
		t.Error("expected draft=true")
	}
}

func TestCreatePRError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Validation Failed"}`))
	}))
	defer server.Close()

	coord := newTestCoordinator(t)
	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Owner: "owner",
		Repo:  "repo",
		Log:   pholog.NewNop(),
	}

	params := domain.CreatePRParams{
		Repo:  testRepo(),
		Title: "Test",
		Head:  "branch",
		Base:  "nonexistent",
	}

	_, err := svc.CreatePR(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for validation failure")
	}
}

func TestUpdateBranchInvalidatesPreviewCache(t *testing.T) {
	t.Parallel()

	repo := testRepo()
	coord := newTestCoordinator(t)

	// Seed the preview cache so we can confirm it's invalidated.
	seeded := domain.PRPreviewSnapshot{
		Repo:   repo.FullName,
		Number: 42,
		Title:  "Before update-branch",
	}
	key := previewCacheKey(repo.Host, repo.FullName, 42)
	meta := previewMeta(key, repo, 42, frozenNow)
	if err := coord.Write(context.Background(), key, seeded, meta); err != nil {
		t.Fatalf("cache write: %v", err)
	}

	var cached domain.PRPreviewSnapshot
	_, found, _ := coord.L2.Get(context.Background(), key, &cached)
	if !found {
		t.Fatal("expected preview cache to exist before update-branch")
	}

	// REST endpoint stub. Records the call and returns 202 Accepted.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected method=PUT, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/pulls/42/update-branch" {
			t.Errorf("expected path=/repos/owner/repo/pulls/42/update-branch, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Now:   func() time.Time { return frozenNow },
		Log:   pholog.NewNop(),
	}

	if err := svc.UpdateBranch(context.Background(), repo, 42, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cache miss after update-branch.
	_, found, _ = coord.L2.Get(context.Background(), key, &cached)
	if found {
		t.Fatal("expected preview cache to be deleted after update-branch")
	}
}

func TestUpdateBranchPropagatesRESTError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Validation Failed: branch is not behind base"}`))
	}))
	defer server.Close()

	restClient := &rest.Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	svc := &PRService{
		Cache: newTestCoordinator(t),
		REST:  restClient,
		Now:   func() time.Time { return frozenNow },
		Log:   pholog.NewNop(),
	}

	err := svc.UpdateBranch(context.Background(), testRepo(), 42, "")
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected error to contain status code 422, got %v", err)
	}
}

func TestUpdateBranchCacheDeleteErrorNonfatal(t *testing.T) {
	t.Parallel()
	repo := testRepo()
	coord := newTestCoordinator(t)
	// Replace L2 with a store that fails Delete: this drives the "log but
	// don't propagate" branch in UpdateBranch.
	coord.L2 = failingStore{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	restClient := &rest.Client{BaseURL: server.URL, Token: "test-token"}
	svc := &PRService{
		Cache: coord,
		REST:  restClient,
		Now:   func() time.Time { return frozenNow },
		Log:   pholog.NewNop(),
	}

	if err := svc.UpdateBranch(context.Background(), repo, 42, ""); err != nil {
		t.Fatalf("expected nil return despite cache delete failure, got %v", err)
	}
}

// failingStore is a cache.Store whose Delete always fails. Get/Put return
// "not found"/no-op so the service treats cache as empty but Delete errors.
type failingStore struct{}

func (failingStore) Get(context.Context, string, any) (domain.CacheMeta, bool, error) {
	return domain.CacheMeta{}, false, nil
}
func (failingStore) Put(context.Context, string, any, domain.CacheMeta) error { return nil }
func (failingStore) Delete(context.Context, string) error                     { return errors.New("disk full") }
func (failingStore) DeleteByRepo(context.Context, string, string) error {
	return errors.New("disk full")
}

func TestUpdateBranchRoutesToHostSpecificRESTClient(t *testing.T) {
	t.Parallel()

	// Two fake GitHub hosts with distinct tokens and expect distinct mutation targets.
	var ghHost, gheHost string
	var ghHits, gheHits int

	ghSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ghHits++
		if r.Header.Get("Authorization") != "token gh-token" {
			t.Errorf("gh host: expected token gh-token, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/repos/owner/repo/pulls/42/update-branch" {
			t.Errorf("gh host: unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ghSrv.Close()
	ghHost = "github.com"

	gheSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gheHits++
		if r.Header.Get("Authorization") != "token ghe-token" {
			t.Errorf("ghe host: expected token ghe-token, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/repos/owner/repo/pulls/42/update-branch" {
			t.Errorf("ghe host: unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer gheSrv.Close()
	gheHost = "github.example.com"

	ghClient := &rest.Client{BaseURL: ghSrv.URL, Token: "gh-token"}
	gheClient := &rest.Client{BaseURL: gheSrv.URL, Token: "ghe-token"}
	restByHost := map[string]*rest.Client{
		ghHost:  ghClient,
		gheHost: gheClient,
	}

	svc := &PRService{
		Cache:      newTestCoordinator(t),
		REST:       ghClient, // default fallback
		RESTByHost: restByHost,
		Now:        func() time.Time { return frozenNow },
		Log:        pholog.NewNop(),
	}

	// Update a PR whose repo.Host is the GHES instance — must hit gheSrv, NOT ghSrv.
	gheRepo := domain.Repository{
		Host: gheHost, Owner: "owner", Name: "repo", FullName: "owner/repo",
	}
	if err := svc.UpdateBranch(context.Background(), gheRepo, 42, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gheHits != 1 {
		t.Fatalf("expected exactly 1 hit on ghe host, got %d", gheHits)
	}
	if ghHits != 0 {
		t.Fatalf("expected 0 hits on github.com host for a GHES PR, got %d", ghHits)
	}
}

func TestUpdateBranchFallsBackToDefaultREST(t *testing.T) {
	t.Parallel()

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	defaultClient := &rest.Client{BaseURL: srv.URL, Token: "default-token"}
	// RESTByHost is nil → every host falls back to defaultClient.
	svc := &PRService{
		Cache: newTestCoordinator(t),
		REST:  defaultClient,
		Now:   func() time.Time { return frozenNow },
		Log:   pholog.NewNop(),
	}
	repo := domain.Repository{Host: "some-unknown-host", Owner: "owner", Name: "repo", FullName: "owner/repo"}
	if err := svc.UpdateBranch(context.Background(), repo, 1, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected fallback client hit, got %d", hits)
	}
}

func TestRESTForConfiguredHostsFailsClosed(t *testing.T) {
	t.Parallel()
	defaultClient := &rest.Client{BaseURL: "https://api.github.com", Token: "primary-token"}
	svc := &PRService{
		REST: defaultClient,
		RESTByHost: map[string]*rest.Client{
			"github.com": defaultClient,
		},
	}
	if _, err := svc.restFor("github.example.com"); err == nil || !strings.Contains(err.Error(), "github.example.com") {
		t.Fatalf("expected an unknown configured host to fail closed, got %v", err)
	}
}
