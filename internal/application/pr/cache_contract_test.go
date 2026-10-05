package pr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
)

// These tests pin the diff cache's observable behaviour through the real
// PRService and a fake GitHub. They must pass unchanged across any change
// to how diffs are cached.

// fakeDiffServer serves raw diffs; failing makes every request return 500.
type fakeDiffServer struct {
	*httptest.Server
	raw     string
	files   []rest.ChangedFile // when set, the diff endpoints answer 406
	failing atomic.Bool
	hits    atomic.Int32
}

func newFakeDiffServer(t *testing.T, raw string) *fakeDiffServer {
	t.Helper()
	f := &fakeDiffServer{raw: raw}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if f.failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if f.files != nil {
			if r.Header.Get("Accept") == "application/vnd.github.v3.diff" {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			var body any = f.files
			if strings.Contains(r.URL.Path, "/commits/") {
				body = map[string]any{"files": f.files}
			}
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		_, _ = w.Write([]byte(f.raw))
	}))
	t.Cleanup(f.Close)
	return f
}

// contractCache is a coordinator over a memory L1 and a SQLite L2 at dbPath.
func contractCache(t *testing.T, dbPath string) *cache.Coordinator {
	t.Helper()
	l2, err := sqlitecache.New(dbPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l2.Close() })
	c := cache.NewCoordinator(memorycache.NewJSONStore(256*1024*1024), l2, nil)
	c.Now = func() time.Time { return frozenNow }
	return c
}

func contractService(c *cache.Coordinator, srv *fakeDiffServer) *PRService {
	return &PRService{
		Cache: c,
		REST:  &rest.Client{BaseURL: srv.URL, Token: "tok"},
		Owner: "owner", Repo: "repo",
		Log:          pholog.NewNop(),
		Now:          func() time.Time { return frozenNow },
		BackgroundFn: func(fn func()) { fn() },
	}
}

const contractSHA = "0123456789abcdef0123456789abcdef01234567"

func TestCacheContract_CachedDiffEqualsFresh(t *testing.T) {
	t.Parallel()
	for _, fx := range difftest.RawDiffs() {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			srv := newFakeDiffServer(t, fx.Raw)
			db := filepath.Join(t.TempDir(), "cache.db")
			svc := contractService(contractCache(t, db), srv)

			fresh, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
			if err != nil || fromCache {
				t.Fatalf("first load: fromCache=%v err=%v", fromCache, err)
			}
			cached, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
			if err != nil || !fromCache {
				t.Fatalf("second load: fromCache=%v err=%v", fromCache, err)
			}
			if srv.hits.Load() != 1 {
				t.Fatalf("expected 1 request, got %d", srv.hits.Load())
			}
			requireSameDiff(t, "memory hit", cached, fresh)

			// A new process: empty memory cache over the same SQLite file.
			restarted := contractService(contractCache(t, db), srv)
			fromDisk, fromCache, err := restarted.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
			if err != nil || !fromCache || srv.hits.Load() != 1 {
				t.Fatalf("disk load: fromCache=%v err=%v hits=%d", fromCache, err, srv.hits.Load())
			}
			requireSameDiff(t, "disk hit", fromDisk, fresh)
		})
	}
}

func TestCacheContract_CachedCommitDiffEqualsFresh(t *testing.T) {
	t.Parallel()
	fx := difftest.RawDiffs()[0]
	srv := newFakeDiffServer(t, fx.Raw)
	db := filepath.Join(t.TempDir(), "cache.db")
	svc := contractService(contractCache(t, db), srv)
	fresh, err := svc.LoadCommitDiff(context.Background(), testRepo(), contractSHA, false)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := svc.LoadCommitDiff(context.Background(), testRepo(), contractSHA, false)
	if err != nil || srv.hits.Load() != 1 {
		t.Fatalf("second load: err=%v hits=%d", err, srv.hits.Load())
	}
	requireSameDiff(t, "memory hit", cached, fresh)
	restarted := contractService(contractCache(t, db), srv)
	fromDisk, err := restarted.LoadCommitDiff(context.Background(), testRepo(), contractSHA, false)
	if err != nil || srv.hits.Load() != 1 {
		t.Fatalf("disk load: err=%v hits=%d", err, srv.hits.Load())
	}
	requireSameDiff(t, "disk hit", fromDisk, fresh)
}

func TestCacheContract_PerFileFallbackIsCached(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, "")
	srv.files = loadFilesFixture(t, "k8s142410")
	db := filepath.Join(t.TempDir(), "cache.db")
	svc := contractService(contractCache(t, db), srv)
	fresh, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
	if err != nil {
		t.Fatal(err)
	}
	hits := srv.hits.Load()
	restarted := contractService(contractCache(t, db), srv)
	cached, fromCache, err := restarted.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
	if err != nil || !fromCache || srv.hits.Load() != hits {
		t.Fatalf("fromCache=%v err=%v hits %d→%d", fromCache, err, hits, srv.hits.Load())
	}
	requireSameDiff(t, "disk hit", cached, fresh)
}

func TestCacheContract_StaleDiffServedWhenGitHubFails(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	svc := contractService(contractCache(t, filepath.Join(t.TempDir(), "cache.db")), srv)
	fresh, _, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
	if err != nil {
		t.Fatal(err)
	}
	commitFresh, err := svc.LoadCommitDiff(context.Background(), testRepo(), contractSHA, false)
	if err != nil {
		t.Fatal(err)
	}

	srv.failing.Store(true)
	got, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, true)
	if err == nil || !strings.Contains(err.Error(), "refresh diff owner/repo") || !fromCache {
		t.Fatalf("forced refresh with GitHub down: fromCache=%v err=%v", fromCache, err)
	}
	requireSameDiff(t, "stale", got, fresh)

	// A forced commit refresh does not read the cache, so it just fails...
	commitGot, err := svc.LoadCommitDiff(context.Background(), testRepo(), contractSHA, true)
	if err == nil || !strings.Contains(err.Error(), "fetch commit diff: ") || len(commitGot.Files) != 0 {
		t.Fatalf("forced commit refresh with GitHub down: files=%d err=%v", len(commitGot.Files), err)
	}
	// ...while a normal open still gets the cached commit diff.
	commitGot, err = svc.LoadCommitDiff(context.Background(), testRepo(), contractSHA, false)
	if err != nil {
		t.Fatal(err)
	}
	requireSameDiff(t, "cached commit", commitGot, commitFresh)
}

func TestCacheContract_NoCacheWithoutHeadSHA(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	svc := contractService(contractCache(t, filepath.Join(t.TempDir(), "cache.db")), srv)
	for range 2 {
		if _, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, "", false); err != nil || fromCache {
			t.Fatalf("fromCache=%v err=%v", fromCache, err)
		}
	}
	if srv.hits.Load() != 2 {
		t.Fatalf("expected a request per load without a head SHA, got %d", srv.hits.Load())
	}
}

func TestCacheContract_DifferentHeadsAreSeparate(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	svc := contractService(contractCache(t, filepath.Join(t.TempDir(), "cache.db")), srv)
	for _, sha := range []string{contractSHA, strings.Repeat("b", 40)} {
		if _, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, sha, false); err != nil || fromCache {
			t.Fatalf("%s: fromCache=%v err=%v", sha, fromCache, err)
		}
	}
	if srv.hits.Load() != 2 {
		t.Fatalf("expected one request per head, got %d", srv.hits.Load())
	}
}

// brokenStore fails every write and finds nothing.
type brokenStore struct{}

func (s *brokenStore) Get(context.Context, string, any) (domain.CacheMeta, bool, error) {
	return domain.CacheMeta{}, false, nil
}
func (s *brokenStore) Put(context.Context, string, any, domain.CacheMeta) error {
	return errors.New("disk full")
}
func (s *brokenStore) Delete(context.Context, string) error { return nil }
func (s *brokenStore) DeleteByRepo(context.Context, string, string) error {
	return nil
}

func TestCacheContract_WriteFailureDoesNotFailLoad(t *testing.T) {
	t.Parallel()
	srv := newFakeDiffServer(t, difftest.RawDiffs()[0].Raw)
	c := cache.NewCoordinator(&brokenStore{}, &brokenStore{}, nil)
	svc := contractService(c, srv)
	dm, fromCache, err := svc.LoadDiff(context.Background(), testRepo(), 42, contractSHA, false)
	if err != nil || fromCache || len(dm.Files) == 0 {
		t.Fatalf("files=%d fromCache=%v err=%v", len(dm.Files), fromCache, err)
	}
	if _, err := svc.LoadCommitDiff(context.Background(), testRepo(), contractSHA, false); err != nil {
		t.Fatal(err)
	}
}

func requireSameDiff(t *testing.T, label string, got, want model.DiffModel) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		for i := range min(len(got.Files), len(want.Files)) {
			if !reflect.DeepEqual(got.Files[i], want.Files[i]) {
				t.Fatalf("%s: file %d (%s) differs from the fresh load", label, i, want.Files[i].NewPath)
			}
		}
		t.Fatalf("%s: diff differs from the fresh load (files %d vs %d, stats %+v vs %+v)",
			label, len(got.Files), len(want.Files), got.Stats, want.Stats)
	}
}
