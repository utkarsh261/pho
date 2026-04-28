package sqlitecache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
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

func TestViewedHistoryRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)
	repo := domain.Repository{Host: "github.com", FullName: "acme/api"}

	records := []domain.ViewedPRRecord{
		{Repo: repo.FullName, Number: 2, Summary: domain.PullRequestSummary{Repo: repo.FullName, Number: 2, Title: "Two"}, LastViewedAt: time.Now().UTC().Add(-time.Hour)},
		{Repo: repo.FullName, Number: 1, Summary: domain.PullRequestSummary{Repo: repo.FullName, Number: 1, Title: "One"}, LastViewedAt: time.Now().UTC()},
	}

	if err := c.SaveViewedHistory(ctx, repo, records); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := c.LoadViewedHistory(ctx, repo)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	// Should be ordered by last_viewed_at DESC.
	if got[0].Number != 1 {
		t.Fatalf("expected most recent PR #1 first, got #%d", got[0].Number)
	}
	if got[1].Number != 2 {
		t.Fatalf("expected PR #2 second, got #%d", got[1].Number)
	}
	if got[0].Summary.Title != "One" {
		t.Fatalf("expected summary title 'One', got %q", got[0].Summary.Title)
	}
}

func TestViewedHistorySaveReplacesRepoRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)
	repo := domain.Repository{Host: "github.com", FullName: "acme/api"}

	if err := c.SaveViewedHistory(ctx, repo, []domain.ViewedPRRecord{
		{Repo: repo.FullName, Number: 1, Summary: domain.PullRequestSummary{Repo: repo.FullName, Number: 1, Title: "Old"}, LastViewedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if err := c.SaveViewedHistory(ctx, repo, []domain.ViewedPRRecord{
		{Repo: repo.FullName, Number: 2, Summary: domain.PullRequestSummary{Repo: repo.FullName, Number: 2, Title: "New"}, LastViewedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("save second: %v", err)
	}

	got, err := c.LoadViewedHistory(ctx, repo)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record after replace, got %d", len(got))
	}
	if got[0].Number != 2 {
		t.Fatalf("expected PR #2, got #%d", got[0].Number)
	}
}

func TestViewedHistoryLoadEmptyRepo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)
	repo := domain.Repository{Host: "github.com", FullName: "acme/empty"}

	got, err := c.LoadViewedHistory(ctx, repo)
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
}

func TestViewedHistorySkipsCorruptRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)
	repo := domain.Repository{Host: "github.com", FullName: "acme/api"}

	// Seed one valid and one corrupt row directly.
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO viewed_history(host, repo, pr_number, summary_json, last_viewed_at)
		VALUES (?, ?, ?, ?, ?)
	`, repo.Host, repo.FullName, 1, `{"number":1,"title":"Valid"}`, toUnixMillis(time.Now().UTC())); err != nil {
		t.Fatalf("insert valid: %v", err)
	}
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO viewed_history(host, repo, pr_number, summary_json, last_viewed_at)
		VALUES (?, ?, ?, ?, ?)
	`, repo.Host, repo.FullName, 2, `{bad json`, toUnixMillis(time.Now().UTC())); err != nil {
		t.Fatalf("insert corrupt: %v", err)
	}

	got, err := c.LoadViewedHistory(ctx, repo)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 valid record, got %d", len(got))
	}
	if got[0].Number != 1 {
		t.Fatalf("expected PR #1, got #%d", got[0].Number)
	}
}

func TestViewedHistoryIsolatedByRepo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newTestCache(t, 1)
	repoA := domain.Repository{Host: "github.com", FullName: "acme/a"}
	repoB := domain.Repository{Host: "github.com", FullName: "acme/b"}

	if err := c.SaveViewedHistory(ctx, repoA, []domain.ViewedPRRecord{
		{Repo: repoA.FullName, Number: 1, Summary: domain.PullRequestSummary{Repo: repoA.FullName, Number: 1}, LastViewedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if err := c.SaveViewedHistory(ctx, repoB, []domain.ViewedPRRecord{
		{Repo: repoB.FullName, Number: 2, Summary: domain.PullRequestSummary{Repo: repoB.FullName, Number: 2}, LastViewedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("save b: %v", err)
	}

	gotA, err := c.LoadViewedHistory(ctx, repoA)
	if err != nil {
		t.Fatalf("load a: %v", err)
	}
	if len(gotA) != 1 || gotA[0].Number != 1 {
		t.Fatalf("expected repoA to have PR #1, got %+v", gotA)
	}

	gotB, err := c.LoadViewedHistory(ctx, repoB)
	if err != nil {
		t.Fatalf("load b: %v", err)
	}
	if len(gotB) != 1 || gotB[0].Number != 2 {
		t.Fatalf("expected repoB to have PR #2, got %+v", gotB)
	}
}
