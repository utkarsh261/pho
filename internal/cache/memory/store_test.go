package memory

import (
	"context"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

func TestJSONStoreRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewJSONStore(1024 * 1024)
	key := "dashboard:v1:host=github.com:repo=acme/api:kind=prs"

	repo := testutil.Repo("acme/api")
	snap := testutil.DashboardSnap(repo, testutil.PR(1))
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      "dashboard_prs",
		Version:   1,
		Host:      "github.com",
		Repo:      "acme/api",
		FetchedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := store.Put(ctx, key, snap, meta); err != nil {
		t.Fatalf("put: %v", err)
	}

	var got domain.DashboardSnapshot
	gotMeta, found, err := store.Get(ctx, key, &got)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatalf("expected hit")
	}
	if got.Repo.FullName != "acme/api" {
		t.Fatalf("unexpected repo: %q", got.Repo.FullName)
	}
	if gotMeta.Kind != "dashboard_prs" {
		t.Fatalf("unexpected kind: %q", gotMeta.Kind)
	}
}

func TestJSONStoreCorruptPayloadDeletesEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewJSONStore(1024)
	key := "preview:v1:host=github.com:repo=acme/api:pr=1"

	// Insert invalid JSON directly into the inner cache.
	store.inner.Put(key, []byte("{bad"), Meta[domain.CacheMeta]{
		Group: GroupPreview,
		Bytes: 4,
		Data: domain.CacheMeta{
			Key:       key,
			Kind:      "preview",
			Version:   1,
			FetchedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Minute),
		},
	})

	var out domain.PreviewSnapshot
	_, found, err := store.Get(ctx, key, &out)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found {
		t.Fatalf("expected miss after decode failure")
	}
	if _, _, ok := store.inner.Get(key); ok {
		t.Fatalf("expected key to be deleted after decode failure")
	}
}
