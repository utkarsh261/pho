package sqlitecache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/testutil"
)

func newTestCache(t *testing.T, version int) *Cache {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	c, err := New(dbPath, version)
	if err != nil {
		t.Fatalf("new sqlite cache: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

func TestRoundTripDashboardSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)

	repo := testutil.Repo("acme/api")
	snap := testutil.DashboardSnap(repo, testutil.PR(101), testutil.PR(102))
	key := "dashboard:v1:host=github.com:repo=acme/api:kind=prs"

	meta := domain.CacheMeta{
		Key:       key,
		Kind:      "dashboard_prs",
		Version:   1,
		Host:      "github.com",
		Repo:      "acme/api",
		FetchedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
	}
	if err := c.Put(ctx, key, snap, meta); err != nil {
		t.Fatalf("put: %v", err)
	}

	var got domain.DashboardSnapshot
	gotMeta, found, err := c.Get(ctx, key, &got)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatalf("expected cache hit")
	}
	if got.Repo.FullName != snap.Repo.FullName {
		t.Fatalf("repo mismatch: got %q want %q", got.Repo.FullName, snap.Repo.FullName)
	}
	if len(got.PRs) != len(snap.PRs) {
		t.Fatalf("pr count mismatch: got %d want %d", len(got.PRs), len(snap.PRs))
	}
	if gotMeta.Kind != "dashboard_prs" {
		t.Fatalf("kind mismatch: got %q", gotMeta.Kind)
	}
	if gotMeta.SizeBytes <= 0 {
		t.Fatalf("expected size bytes > 0, got %d", gotMeta.SizeBytes)
	}
}

func TestExpiredEntryStillReadableWithExpiredMeta(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)

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
	if err := c.Put(ctx, key, snap, meta); err != nil {
		t.Fatalf("put: %v", err)
	}

	var got domain.RecentSnapshot
	gotMeta, found, err := c.Get(ctx, key, &got)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatalf("expected cache hit")
	}
	if !gotMeta.ExpiresAt.Before(time.Now().UTC()) {
		t.Fatalf("expected expired metadata, got expires_at=%s", gotMeta.ExpiresAt)
	}
}

func TestVersionMismatchDeletesAndMisses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cache.db")

	cV1, err := New(dbPath, 1)
	if err != nil {
		t.Fatalf("new v1 cache: %v", err)
	}
	defer cV1.Close()

	key := "discovery:v1:root=/tmp/workspace"
	snap := domain.DiscoverySnapshot{
		Repos:     []domain.Repository{testutil.Repo("acme/api")},
		FetchedAt: time.Now().UTC(),
	}
	if err := cV1.Put(ctx, key, snap, domain.CacheMeta{
		Key:       key,
		Kind:      "discovery",
		Version:   1,
		Host:      "-",
		FetchedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}); err != nil {
		t.Fatalf("put v1: %v", err)
	}

	cV2, err := New(dbPath, 2)
	if err != nil {
		t.Fatalf("new v2 cache: %v", err)
	}
	defer cV2.Close()

	var got domain.DiscoverySnapshot
	_, found, err := cV2.Get(ctx, key, &got)
	if err != nil {
		t.Fatalf("get v2: %v", err)
	}
	if found {
		t.Fatalf("expected miss due to version mismatch")
	}

	// Verify stale-version row was deleted.
	var count int
	if err := cV2.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cache_entries WHERE key = ?`, key).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected row deletion, found %d rows", count)
	}
}

func TestCorruptPayloadDeletesOnRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)
	key := "preview:v1:host=github.com:repo=acme/api:pr=42"

	_, err := c.db.ExecContext(ctx, `
		INSERT INTO cache_entries(
			key, kind, version, host, repo, pr_number, fetched_at, expires_at,
			etag, last_modified, size_bytes, encoding, payload
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		key, "preview", 1, "github.com", "acme/api", 42,
		time.Now().Add(-time.Minute).UnixMilli(),
		time.Now().Add(time.Minute).UnixMilli(),
		"", "", 5, "json", []byte("{bad"),
	)
	if err != nil {
		t.Fatalf("insert corrupt row: %v", err)
	}

	var got domain.PreviewSnapshot
	_, found, err := c.Get(ctx, key, &got)
	if err != nil {
		t.Fatalf("get corrupt row: %v", err)
	}
	if found {
		t.Fatalf("expected miss for corrupt payload")
	}

	var count int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cache_entries WHERE key = ?`, key).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected corrupt row to be deleted, found %d rows", count)
	}
}
