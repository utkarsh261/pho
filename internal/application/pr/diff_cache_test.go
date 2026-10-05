package pr

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/domain"
)

func TestDiffCacheWriteHappensInBackground(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	svc := contractService(contractCache(t, filepath.Join(t.TempDir(), "cache.db")), srv)
	var pending []func()
	svc.BackgroundFn = func(fn func()) { pending = append(pending, fn) }

	if _, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected the cache write to be deferred, got %d background jobs", len(pending))
	}
	// Until the write runs, the next open refetches.
	if _, fromCache, _ := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false); fromCache {
		t.Fatal("diff was cached before the background write ran")
	}
	pending[0]()
	if _, fromCache, _ := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false); !fromCache {
		t.Fatal("diff not cached after the background write")
	}
}

func TestDiffCacheRealGoroutineWrite(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, difftest.RawDiffs()[1].Raw)
	svc := contractService(contractCache(t, filepath.Join(t.TempDir(), "cache.db")), srv)
	svc.BackgroundFn = nil // real goroutine
	if _, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
		if err != nil {
			t.Fatal(err)
		}
		if fromCache {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("diff never reached the cache")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// seedEntry stores a raw payload under key with the given metadata.
func seedEntry(t *testing.T, s cache.Store, key, kind, host, repo string, pr *int) {
	t.Helper()
	meta := domain.CacheMeta{Kind: kind, Host: host, Repo: repo, PRNumber: pr,
		FetchedAt: frozenNow, ExpiresAt: frozenNow.Add(time.Hour)}
	if err := s.Put(context.Background(), key, map[string]string{"k": key}, meta); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteOtherDiffsScope(t *testing.T) {
	t.Parallel()
	l2, err := sqlitecache.New(filepath.Join(t.TempDir(), "cache.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	stores := map[string]cache.Store{"memory": memorycache.NewJSONStore(1 << 20), "sqlite": l2}
	for name, s := range stores {
		pr7, pr8 := 7, 8
		seedEntry(t, s, "diff:old", "diff", "github.com", "o/r", &pr7)
		seedEntry(t, s, "diff:keep", "diff", "github.com", "o/r", &pr7)
		seedEntry(t, s, "diff:otherpr", "diff", "github.com", "o/r", &pr8)
		seedEntry(t, s, "diff:otherrepo", "diff", "github.com", "o/x", &pr7)
		seedEntry(t, s, "diff:ghes", "diff", "ghe.example.com", "o/r", &pr7)
		seedEntry(t, s, "commitdiff:abc", "diff", "github.com", "o/r", nil)
		seedEntry(t, s, "draft:7", "draft_inline", "github.com", "o/r", &pr7)
		seedEntry(t, s, "preview:7", "preview", "github.com", "o/r", &pr7)
		if err := s.DeleteOtherDiffs(context.Background(), "github.com", "o/r", 7, "diff:keep"); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"diff:keep", "diff:otherpr", "diff:otherrepo", "diff:ghes", "commitdiff:abc", "draft:7", "preview:7"} {
			var v map[string]string
			if _, found, _ := s.Get(context.Background(), key, &v); !found {
				t.Errorf("%s: %s was deleted", name, key)
			}
		}
		var v map[string]string
		if _, found, _ := s.Get(context.Background(), "diff:old", &v); found {
			t.Errorf("%s: diff:old survived", name)
		}
	}
}

func TestLoadDiffKeepsOnlyLatestHead(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "cache.db")
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	svc := contractService(contractCache(t, db), srv)
	older, newer := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, sha := range []string{older, newer} {
		if _, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, sha, false); err != nil {
			t.Fatal(err)
		}
	}
	// Another PR's diff and a commit diff are untouched.
	if _, _, err := svc.LoadDiff(context.Background(), testRepo(), 43, older, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoadCommitDiff(context.Background(), testRepo(), older, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, newer, false); err != nil {
		t.Fatal(err)
	}
	hits := srv.hits.Load()

	restarted := contractService(contractCache(t, db), srv)
	if _, fromCache, _ := restarted.LoadDiff(context.Background(), testRepo(), 42, newer, false); !fromCache {
		t.Error("latest head's diff must stay cached")
	}
	if _, fromCache, _ := restarted.LoadDiff(context.Background(), testRepo(), 43, older, false); !fromCache {
		t.Error("another PR's diff must stay cached")
	}
	if _, err := restarted.LoadCommitDiff(context.Background(), testRepo(), older, false); err != nil {
		t.Fatal(err)
	}
	if srv.hits.Load() != hits {
		t.Fatalf("kept entries were refetched (%d → %d requests)", hits, srv.hits.Load())
	}
	if _, fromCache, _ := restarted.LoadDiff(context.Background(), testRepo(), 42, older, false); fromCache {
		t.Error("the older head's diff must be pruned")
	}
}

func TestUnreadableCachedDiffIsRefetched(t *testing.T) {
	t.Parallel()
	for name, entry := range map[string]any{
		"bad gzip":       cachedDiff{Format: cachedDiffFormat, Gz: []byte("not gzip")},
		"unknown format": cachedDiff{Format: "something-new", Gz: []byte{}},
		"old json model": map[string]any{"Files": []any{}},
	} {
		srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
		c := contractCache(t, filepath.Join(t.TempDir(), "cache.db"))
		svc := contractService(c, srv)
		key := diffCacheKey("github.com", "owner/repo", 42, contractSHA)
		if err := c.Write(context.Background(), key, entry, diffMeta(key, testRepo(), 42, frozenNow)); err != nil {
			t.Fatal(err)
		}
		dm, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
		if err != nil || fromCache || len(dm.Files) == 0 || srv.hits.Load() != 1 {
			t.Fatalf("%s: files=%d fromCache=%v err=%v hits=%d", name, len(dm.Files), fromCache, err, srv.hits.Load())
		}
		if _, fromCache, _ := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false); !fromCache {
			t.Fatalf("%s: refetched diff must replace the bad entry", name)
		}
	}
}

func TestOldFormatDiffsAreIgnoredAndPurged(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "cache.db")
	l2, err := sqlitecache.New(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	pr := 42
	oldKey := "diff:v1:host=github.com:repo=owner/repo:pr=42:sha=" + contractSHA
	oldModel, _ := json.Marshal(map[string]any{"Files": []any{map[string]any{"NewPath": "stale.go"}}})
	for _, e := range []struct{ key, kind string }{
		{oldKey, "diff"},
		{"commitdiff:v1:host=github.com:repo=owner/repo:sha=" + contractSHA, "diff"},
		{"preview:v4:host=github.com:repo=owner/repo:pr=42", "preview"},
		{"draft_inline:v1:host=github.com:repo=owner/repo:pr=42:sha=x", "draft_inline"},
	} {
		meta := domain.CacheMeta{Kind: e.kind, Host: "github.com", Repo: "owner/repo", PRNumber: &pr, FetchedAt: frozenNow, ExpiresAt: frozenNow.Add(time.Hour)}
		if err := l2.Put(context.Background(), e.key, json.RawMessage(oldModel), meta); err != nil {
			t.Fatal(err)
		}
	}

	n, err := l2.DeleteKeyPrefixes(context.Background(), LegacyDiffKeyPrefixes...)
	if err != nil || n != 2 {
		t.Fatalf("purged %d entries (err %v), want the 2 old diffs", n, err)
	}
	var v json.RawMessage
	for _, key := range []string{"preview:v4:host=github.com:repo=owner/repo:pr=42", "draft_inline:v1:host=github.com:repo=owner/repo:pr=42:sha=x"} {
		if _, found, _ := l2.Get(context.Background(), key, &v); !found {
			t.Errorf("%s was purged", key)
		}
	}

	// An upgraded pho never reads an old entry: it fetches the diff (and the
	// fetch prunes the old entry).
	meta := domain.CacheMeta{Kind: "diff", Host: "github.com", Repo: "owner/repo", PRNumber: &pr, FetchedAt: frozenNow, ExpiresAt: frozenNow.Add(time.Hour)}
	if err := l2.Put(context.Background(), oldKey, json.RawMessage(oldModel), meta); err != nil {
		t.Fatal(err)
	}
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	c := cache.NewCoordinator(memorycache.NewJSONStore(1<<26), l2, nil)
	c.Now = func() time.Time { return frozenNow }
	dm, fromCache, err := contractService(c, srv).LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
	if err != nil || fromCache || srv.hits.Load() != 1 || dm.Files[0].NewPath == "stale.go" {
		t.Fatalf("fromCache=%v err=%v hits=%d", fromCache, err, srv.hits.Load())
	}

	if _, found, _ := l2.Get(context.Background(), oldKey, &v); found {
		t.Error("the old entry should be pruned by the fetch")
	}
}
