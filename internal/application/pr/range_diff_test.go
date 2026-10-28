package pr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
)

const rangeRawDiff = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
 keep
-old
+new
`

// serveCompare answers the compare JSON request with compareJSON and the raw
// diff request with diffStatus/diffBody. It counts requests.
func serveCompare(t *testing.T, compareJSON string, diffStatus int, diffBody string) (*PRService, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/compare/aaa...bbb") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if strings.Contains(r.Header.Get("Accept"), "diff") {
			w.WriteHeader(diffStatus)
			_, _ = w.Write([]byte(diffBody))
			return
		}
		_, _ = w.Write([]byte(compareJSON))
	}))
	t.Cleanup(srv.Close)
	svc := &PRService{
		Cache: newTestCoordinator(t),
		REST:  &rest.Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Token: "tok"},
		Now:   time.Now,
		Log:   pholog.NewNop(),
	}
	return svc, &calls
}

func TestLoadRangeDiffAhead(t *testing.T) {
	t.Parallel()
	svc, calls := serveCompare(t,
		`{"status":"ahead","total_commits":2,"commits":[{"sha":"c1","parents":[{"sha":"aaa"}]},{"sha":"c2","parents":[{"sha":"c1"}]}]}`,
		http.StatusOK, rangeRawDiff)

	rd, err := svc.LoadRangeDiff(context.Background(), testRepo(), 7, "aaa", "bbb", false)
	if err != nil {
		t.Fatal(err)
	}
	if rd.Rewritten || rd.BaseMerged || rd.Commits != 2 {
		t.Errorf("range = %+v", rd)
	}
	if rd.Diff.BaseSHA != "aaa" || rd.Diff.HeadSHA != "bbb" || rd.Diff.PRNumber != 7 {
		t.Errorf("diff shas = %s..%s pr %d", rd.Diff.BaseSHA, rd.Diff.HeadSHA, rd.Diff.PRNumber)
	}
	if len(rd.Diff.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(rd.Diff.Files))
	}
	for _, l := range rd.Diff.Files[0].Hunks[0].Lines {
		for _, a := range l.Anchors {
			want := "bbb"
			if a.Side == "LEFT" {
				want = "aaa"
			}
			if a.CommitSHA != want {
				t.Errorf("%s anchor on %q = %s, want %s", a.Side, l.Raw, a.CommitSHA, want)
			}
		}
	}

	// Second load is served from the cache.
	before := calls.Load()
	if _, err := svc.LoadRangeDiff(context.Background(), testRepo(), 7, "aaa", "bbb", false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before {
		t.Errorf("cached load made %d requests", calls.Load()-before)
	}
}

func TestLoadRangeDiffBaseMerged(t *testing.T) {
	t.Parallel()
	svc, _ := serveCompare(t,
		`{"status":"ahead","total_commits":1,"commits":[{"sha":"m","parents":[{"sha":"aaa"},{"sha":"main"}]}]}`,
		http.StatusOK, rangeRawDiff)
	rd, err := svc.LoadRangeDiff(context.Background(), testRepo(), 7, "aaa", "bbb", false)
	if err != nil {
		t.Fatal(err)
	}
	if !rd.BaseMerged {
		t.Error("BaseMerged = false, want true")
	}
}

func TestLoadRangeDiffRewrittenSkipsDiff(t *testing.T) {
	t.Parallel()
	svc, calls := serveCompare(t, `{"status":"diverged","total_commits":3}`, http.StatusOK, rangeRawDiff)
	rd, err := svc.LoadRangeDiff(context.Background(), testRepo(), 7, "aaa", "bbb", false)
	if err != nil {
		t.Fatal(err)
	}
	if !rd.Rewritten || len(rd.Diff.Files) != 0 {
		t.Errorf("range = %+v", rd)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1 (no diff fetch)", calls.Load())
	}
}

func TestLoadRangeDiffTooLargeUsesCompareFiles(t *testing.T) {
	t.Parallel()
	svc, _ := serveCompare(t,
		`{"status":"ahead","total_commits":1,"files":[{"filename":"b.go","status":"modified","additions":1,"deletions":1,"changes":2,"patch":"@@ -1 +1 @@\n-x\n+y"}]}`,
		http.StatusNotAcceptable, `{"message":"too_large"}`)
	rd, err := svc.LoadRangeDiff(context.Background(), testRepo(), 7, "aaa", "bbb", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rd.Diff.Files) != 1 || rd.Diff.Files[0].NewPath != "b.go" {
		t.Errorf("files = %+v", rd.Diff.Files)
	}
}

func TestLoadRangeDiffNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	svc := &PRService{
		Cache: newTestCoordinator(t),
		REST:  &rest.Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Token: "tok"},
		Now:   time.Now,
	}
	_, err := svc.LoadRangeDiff(context.Background(), testRepo(), 7, "aaa", "bbb", false)
	if !errors.Is(err, rest.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
