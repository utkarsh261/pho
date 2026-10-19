package pr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
)

func labelService(t *testing.T, coord *cache.Coordinator, h http.HandlerFunc) *PRService {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &PRService{
		Cache: coord,
		REST:  &rest.Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Token: "tok"},
		Now:   func() time.Time { return frozenNow },
		Log:   pholog.NewNop(),
	}
}

func TestLoadRepoLabelsCachesFreshList(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator(t)
	var calls atomic.Int32
	svc := labelService(t, coord, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/repos/owner/repo/labels" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"name":"bug","color":"d73a4a","description":"Broken"}]`)
	})

	for i := 0; i < 2; i++ {
		got, err := svc.LoadRepoLabels(context.Background(), testRepo())
		if err != nil {
			t.Fatal(err)
		}
		want := domain.RepoLabel{Label: domain.Label{Name: "bug", Color: "d73a4a"}, Description: "Broken"}
		if len(got.Labels) != 1 || got.Labels[0] != want || got.Truncated {
			t.Fatalf("labels = %+v", got)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("fetched %d times, want 1 (second load from cache)", calls.Load())
	}
}

func TestLoadRepoLabelsFallsBackToStale(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator(t)
	repo := testRepo()
	key := repoLabelsCacheKey(repo.Host, repo.FullName)
	stale := domain.RepoLabelList{Labels: []domain.RepoLabel{{Label: domain.Label{Name: "old"}}}}
	meta := domain.CacheMeta{Key: key, Kind: cacheKindRepoLabels, Version: cacheVersion, Host: repo.Host, Repo: repo.FullName,
		FetchedAt: frozenNow.Add(-time.Hour), ExpiresAt: frozenNow.Add(-time.Minute), Encoding: "json"}
	if err := coord.Write(context.Background(), key, stale, meta); err != nil {
		t.Fatal(err)
	}
	svc := labelService(t, coord, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	got, err := svc.LoadRepoLabels(context.Background(), repo)
	if err != nil || len(got.Labels) != 1 || got.Labels[0].Name != "old" {
		t.Fatalf("got %+v, %v; want stale list", got, err)
	}

	empty := labelService(t, newTestCoordinator(t), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	if _, err := empty.LoadRepoLabels(context.Background(), repo); err == nil {
		t.Fatal("no cache and a failed fetch should error")
	}
}

// recordingLabelAPI accepts every label request and records them in order.
type recordingLabelAPI struct {
	mu     sync.Mutex
	calls  []string
	failOn string // "METHOD path" answered with failStatus
	status int
}

func (a *recordingLabelAPI) handler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	call := r.Method + " " + r.URL.EscapedPath()
	if len(body) > 0 {
		var v any
		_ = json.Unmarshal(body, &v)
		call += fmt.Sprintf(" %v", v)
	}
	a.calls = append(a.calls, call)
	if a.failOn != "" && strings.HasPrefix(call, a.failOn) {
		w.WriteHeader(a.status)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/labels" {
		w.WriteHeader(http.StatusCreated)
	}
}

func TestApplyLabelsOrderAndCaches(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator(t)
	repo := testRepo()
	ctx := context.Background()

	previewKey := previewCacheKey(repo.Host, repo.FullName, 42)
	labelsKey := repoLabelsCacheKey(repo.Host, repo.FullName)
	draftKey := draftInlineCacheKey(repo.Host, repo.FullName, 42, "sha")
	_ = coord.Write(ctx, previewKey, domain.PRPreviewSnapshot{Number: 42}, previewMeta(previewKey, repo, 42, frozenNow))
	_ = coord.Write(ctx, labelsKey, domain.RepoLabelList{}, domain.CacheMeta{Key: labelsKey, Version: cacheVersion, Host: repo.Host, Repo: repo.FullName, ExpiresAt: frozenNow.Add(time.Hour)})
	_ = coord.Write(ctx, draftKey, []string{"draft"}, draftInlineMeta(draftKey, repo, 42, "sha", frozenNow))

	api := &recordingLabelAPI{}
	svc := labelService(t, coord, api.handler)
	change := domain.LabelChange{
		Create: []domain.Label{{Name: "needs-qa", Color: "1d76db"}},
		Add:    []string{"needs-qa", "bug"},
		Remove: []string{"area/diff", "wontfix"},
	}
	if err := svc.ApplyLabels(ctx, repo, 42, change); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /repos/owner/repo/labels map[color:1d76db name:needs-qa]",
		"POST /repos/owner/repo/issues/42/labels map[labels:[needs-qa bug]]",
		"DELETE /repos/owner/repo/issues/42/labels/area%2Fdiff",
		"DELETE /repos/owner/repo/issues/42/labels/wontfix",
	}
	if strings.Join(api.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(api.calls, "\n"), strings.Join(want, "\n"))
	}

	var v any
	if _, found, _ := coord.L2.Get(ctx, previewKey, &v); found {
		t.Error("preview cache should be dropped")
	}
	if _, found, _ := coord.L2.Get(ctx, labelsKey, &v); found {
		t.Error("repo labels cache should be dropped after creating a label")
	}
	if _, found, _ := coord.L2.Get(ctx, draftKey, &v); !found {
		t.Error("review drafts must survive a label change")
	}
}

func TestApplyLabelsStopsAtFirstFailure(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator(t)
	repo := testRepo()
	ctx := context.Background()
	previewKey := previewCacheKey(repo.Host, repo.FullName, 42)
	_ = coord.Write(ctx, previewKey, domain.PRPreviewSnapshot{Number: 42}, previewMeta(previewKey, repo, 42, frozenNow))

	api := &recordingLabelAPI{failOn: "POST /repos/owner/repo/labels", status: http.StatusForbidden}
	svc := labelService(t, coord, api.handler)
	err := svc.ApplyLabels(ctx, repo, 42, domain.LabelChange{
		Create: []domain.Label{{Name: "new", Color: "1d76db"}},
		Add:    []string{"new"},
		Remove: []string{"bug"},
	})
	if !errors.Is(err, domain.ErrLabelPermission) || !strings.Contains(err.Error(), "create labels") {
		t.Fatalf("err = %v, want create permission error", err)
	}
	if len(api.calls) != 1 {
		t.Errorf("calls after failure: %v", api.calls)
	}
	var v any
	if _, found, _ := coord.L2.Get(ctx, previewKey, &v); found {
		t.Error("preview cache should be dropped even on failure")
	}
}
