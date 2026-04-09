package memory

import "testing"

func putItem[V any, M any](t *testing.T, c *Cache[V, M], key string, value V, group Group, bytes int, meta M) {
	t.Helper()
	c.Put(key, value, Meta[M]{
		Group: group,
		Bytes: bytes,
		Data:  meta,
	})
}

func mustGet[V comparable, M comparable](t *testing.T, c *Cache[V, M], key string) (V, Meta[M]) {
	t.Helper()
	value, meta, ok := c.Get(key)
	if !ok {
		t.Fatalf("expected key %q to be present", key)
	}
	return value, meta
}

func mustMissing[V any, M any](t *testing.T, c *Cache[V, M], key string) {
	t.Helper()
	if _, _, ok := c.Get(key); ok {
		t.Fatalf("expected key %q to be missing", key)
	}
}

func TestPutGetDelete(t *testing.T) {
	c := New[string, string](16)

	putItem(t, c, "preview:1", "value-1", GroupPreview, 3, "meta-1")

	value, meta := mustGet(t, c, "preview:1")
	if value != "value-1" {
		t.Fatalf("value: got %q, want %q", value, "value-1")
	}
	if meta.Group != GroupPreview {
		t.Fatalf("group: got %v, want %v", meta.Group, GroupPreview)
	}
	if meta.Bytes != 3 {
		t.Fatalf("bytes: got %d, want %d", meta.Bytes, 3)
	}
	if meta.Data != "meta-1" {
		t.Fatalf("data: got %q, want %q", meta.Data, "meta-1")
	}

	c.Delete("preview:1")
	mustMissing(t, c, "preview:1")
}

func TestEvictionPrefersPreviewAndUsesLRU(t *testing.T) {
	c := New[string, string](5)

	putItem(t, c, "preview:old", "old", GroupPreview, 1, "p-old")
	putItem(t, c, "preview:new", "new", GroupPreview, 1, "p-new")
	putItem(t, c, "dashboard:1", "dash", GroupDashboard, 1, "dash")
	putItem(t, c, "recent:1", "recent", GroupRecent, 1, "recent")
	putItem(t, c, "discovery:1", "disc", GroupDiscovery, 1, "disc")

	// Access the older preview entry so the other preview becomes LRU.
	if _, _, ok := c.Get("preview:old"); !ok {
		t.Fatal("expected preview:old to be present before eviction")
	}

	putItem(t, c, "repo-index:1", "repo", GroupRepoIndex, 1, "repo")

	mustGet(t, c, "preview:old")
	mustMissing(t, c, "preview:new")
	mustGet(t, c, "dashboard:1")
	mustGet(t, c, "recent:1")
	mustGet(t, c, "discovery:1")
	mustGet(t, c, "repo-index:1")
}

func TestEvictionAcrossPriorityBuckets(t *testing.T) {
	c := New[string, string](3)

	putItem(t, c, "preview:1", "p1", GroupPreview, 1, "p1")
	putItem(t, c, "preview:2", "p2", GroupPreview, 1, "p2")
	putItem(t, c, "pr-index:1", "pr", GroupPRIndex, 1, "pr")
	putItem(t, c, "recent:1", "recent", GroupRecent, 1, "recent")
	putItem(t, c, "dashboard:1", "dash", GroupDashboard, 1, "dash")
	putItem(t, c, "discovery:1", "disc", GroupDiscovery, 1, "disc")
	putItem(t, c, "repo-index:1", "repo", GroupRepoIndex, 1, "repo")

	mustMissing(t, c, "preview:1")
	mustMissing(t, c, "preview:2")
	mustMissing(t, c, "pr-index:1")
	mustMissing(t, c, "recent:1")
	mustGet(t, c, "dashboard:1")
	mustGet(t, c, "discovery:1")
	mustGet(t, c, "repo-index:1")
}

func TestDiscoveryAndRepoIndexShareTheLowestPriorityBucket(t *testing.T) {
	c := New[string, string](2)

	putItem(t, c, "dashboard:1", "dash", GroupDashboard, 1, "dash")
	putItem(t, c, "discovery:1", "disc", GroupDiscovery, 1, "disc")
	putItem(t, c, "repo-index:1", "repo", GroupRepoIndex, 1, "repo")
	putItem(t, c, "repo-index:2", "repo-2", GroupRepoIndex, 1, "repo-2")

	mustMissing(t, c, "dashboard:1")
	mustMissing(t, c, "discovery:1")
	mustGet(t, c, "repo-index:1")
	mustGet(t, c, "repo-index:2")
}
