package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

type countingStore struct {
	inner Store
	gets  int
	puts  int
	dels  int
}

func (s *countingStore) Get(ctx context.Context, key string, dest any) (domain.CacheMeta, bool, error) {
	s.gets++
	return s.inner.Get(ctx, key, dest)
}

func (s *countingStore) Put(ctx context.Context, key string, value any, meta domain.CacheMeta) error {
	s.puts++
	return s.inner.Put(ctx, key, value, meta)
}

func (s *countingStore) Delete(ctx context.Context, key string) error {
	s.dels++
	return s.inner.Delete(ctx, key)
}

func (s *countingStore) DeleteByRepo(ctx context.Context, host, repo string) error {
	return s.inner.DeleteByRepo(ctx, host, repo)
}

func newSQLiteStore(t *testing.T) *sqlitecache.Cache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := sqlitecache.New(path, 1)
	if err != nil {
		t.Fatalf("new sqlite cache: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

func TestCoordinatorReadWriteAndPromotion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l1 := &countingStore{inner: memorycache.NewJSONStore(1024 * 1024)}
	l2 := &countingStore{inner: newSQLiteStore(t)}
	c := NewCoordinator(l1, l2, nil)

	key := "dashboard:v1:host=github.com:repo=acme/api:kind=prs"
	repo := testutil.Repo("acme/api")
	snap := testutil.DashboardSnap(repo, testutil.PR(1), testutil.PR(2))
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      "dashboard_prs",
		Version:   1,
		Host:      "github.com",
		Repo:      "acme/api",
		FetchedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
	}

	var cold domain.DashboardSnapshot
	_, _, found, err := c.StaleWhileRevalidate(ctx, key, &cold, nil)
	if err != nil {
		t.Fatalf("cold read: %v", err)
	}
	if found {
		t.Fatalf("expected cold miss")
	}

	if err := c.Write(ctx, key, snap, meta); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Should hit L1.
	var first domain.DashboardSnapshot
	_, freshness, found, err := c.StaleWhileRevalidate(ctx, key, &first, nil)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if !found {
		t.Fatalf("expected hit after write")
	}
	if freshness != domain.FreshnessFresh {
		t.Fatalf("expected fresh, got %q", freshness)
	}
	if len(first.PRs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(first.PRs))
	}

	// Clear only L1 and ensure L2 hit promotes back to L1.
	if err := l1.inner.Delete(ctx, key); err != nil {
		t.Fatalf("l1 delete: %v", err)
	}
	beforeL2Gets := l2.gets

	var promoted domain.DashboardSnapshot
	_, _, found, err = c.StaleWhileRevalidate(ctx, key, &promoted, nil)
	if err != nil {
		t.Fatalf("read after l1 delete: %v", err)
	}
	if !found {
		t.Fatalf("expected l2 hit after l1 delete")
	}
	if l2.gets <= beforeL2Gets {
		t.Fatalf("expected l2 get count to increase")
	}

	// Next read should come from L1 again (no extra L2 get).
	beforeL2Gets = l2.gets
	var second domain.DashboardSnapshot
	_, _, found, err = c.StaleWhileRevalidate(ctx, key, &second, nil)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if !found {
		t.Fatalf("expected l1 hit after promotion")
	}
	if l2.gets != beforeL2Gets {
		t.Fatalf("expected no extra l2 reads after promotion")
	}
}

func TestCoordinatorReturnsStaleAndSchedulesRefresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l1 := &countingStore{inner: memorycache.NewJSONStore(1024 * 1024)}
	l2 := &countingStore{inner: newSQLiteStore(t)}
	c := NewCoordinator(l1, l2, nil)

	key := "dashboard:v1:host=github.com:repo=acme/api:kind=dashboard_prs"
	snap := testutil.DashboardSnap(testutil.Repo("acme/api"))
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      "dashboard_prs",
		Version:   1,
		Host:      "github.com",
		Repo:      "acme/api",
		FetchedAt: time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Minute),
	}
	if err := c.Write(ctx, key, snap, meta); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	var scheduled string
	var got domain.DashboardSnapshot
	_, freshness, found, err := c.StaleWhileRevalidate(ctx, key, &got, func(k string) {
		scheduled = k
	})
	if err != nil {
		t.Fatalf("stale read: %v", err)
	}
	if !found {
		t.Fatalf("expected stale hit")
	}
	if freshness != domain.FreshnessStale {
		t.Fatalf("expected stale freshness, got %q", freshness)
	}
	if scheduled != key {
		t.Fatalf("expected refresh for key %q, got %q", key, scheduled)
	}
}

func TestCoordinatorPreviewSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l1 := &countingStore{inner: memorycache.NewJSONStore(1024 * 1024)}
	l2 := &countingStore{inner: newSQLiteStore(t)}
	c := NewCoordinator(l1, l2, nil)

	key := "preview:v4:host=github.com:repo=acme/api:pr=42"
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      "preview",
		Version:   1,
		Host:      "github.com",
		Repo:      "acme/api",
		PRNumber:  intPtr(42),
		FetchedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
	}

	ts := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	snap := domain.PRPreviewSnapshot{
		ID:     "PR_1",
		Repo:   "acme/api",
		Number: 42,
		Title:  "Test PR",
		Reviewers: []domain.PreviewReviewer{
			{Login: "bob", State: "COMMENTED", Body: "test latest", SubmittedAt: ts},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:   "thread1",
				Path: "handlers.go",
				Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "test", CreatedAt: ts},
					{ID: "c2", Login: "alice", Body: "test inline", CreatedAt: ts.Add(24 * time.Hour)},
				},
			},
		},
	}

	if err := c.Write(ctx, key, snap, meta); err != nil {
		t.Fatalf("write: %v", err)
	}

	var loaded domain.PRPreviewSnapshot
	_, _, found, err := c.StaleWhileRevalidate(ctx, key, &loaded, nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !found {
		t.Fatalf("expected cache hit after write")
	}
	if len(loaded.ReviewThreads) != 1 {
		t.Fatalf("expected 1 ReviewThread after round-trip, got %d", len(loaded.ReviewThreads))
	}
	if loaded.ReviewThreads[0].ID != "thread1" {
		t.Errorf("expected thread ID 'thread1', got %q", loaded.ReviewThreads[0].ID)
	}
	if loaded.ReviewThreads[0].Path != "handlers.go" {
		t.Errorf("expected path 'handlers.go', got %q", loaded.ReviewThreads[0].Path)
	}
	if loaded.ReviewThreads[0].Line != 103 {
		t.Errorf("expected line 103, got %d", loaded.ReviewThreads[0].Line)
	}
	if len(loaded.ReviewThreads[0].Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(loaded.ReviewThreads[0].Comments))
	}
	if loaded.ReviewThreads[0].Comments[0].Login != "bob" {
		t.Errorf("expected first comment login 'bob', got %q", loaded.ReviewThreads[0].Comments[0].Login)
	}
	if loaded.ReviewThreads[0].Comments[1].Login != "alice" {
		t.Errorf("expected second comment login 'alice', got %q", loaded.ReviewThreads[0].Comments[1].Login)
	}
	if len(loaded.Reviewers) != 1 || loaded.Reviewers[0].Login != "bob" {
		t.Errorf("expected reviewer bob, got %+v", loaded.Reviewers)
	}
}

func intPtr(i int) *int { return &i }
