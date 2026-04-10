package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	memorycache "github.com/utk/git-term/internal/cache/memory"
	sqlitecache "github.com/utk/git-term/internal/cache/sqlite"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/testutil"
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

	key := "dashboard:v1:host=github.com:repo=acme/api:kind=recent"
	snap := testutil.RecentSnap(testutil.Repo("acme/api"))
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      "dashboard_recent",
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
	var got domain.RecentSnapshot
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
