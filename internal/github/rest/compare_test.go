package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newCompareServer(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/repos/o/r/compare/aaa...bbb"; r.URL.Path != want {
			t.Errorf("path = %s, want %s", r.URL.Path, want)
		}
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: "tok", HTTPClient: srv.Client()}
}

func TestFetchCompareDecodes(t *testing.T) {
	t.Parallel()
	c := newCompareServer(t, http.StatusOK, `{
		"status": "ahead", "ahead_by": 2, "total_commits": 2,
		"commits": [{"sha": "c1", "parents": [{"sha": "aaa"}]}, {"sha": "c2", "parents": [{"sha": "c1"}, {"sha": "m"}]}],
		"files": [{"filename": "a.go", "status": "modified", "additions": 1, "deletions": 1, "changes": 2, "patch": "@@ -1 +1 @@\n-a\n+b"}]
	}`)
	cmp, err := c.FetchCompare(context.Background(), "o", "r", "aaa", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Status != "ahead" || cmp.AheadBy != 2 || cmp.TotalCommits != 2 {
		t.Errorf("compare = %+v", cmp)
	}
	if len(cmp.Commits) != 2 || len(cmp.Commits[1].Parents) != 2 {
		t.Errorf("commits = %+v", cmp.Commits)
	}
	if len(cmp.Files) != 1 || cmp.Files[0].Filename != "a.go" {
		t.Errorf("files = %+v", cmp.Files)
	}
}

func TestFetchCompareNotFound(t *testing.T) {
	t.Parallel()
	c := newCompareServer(t, http.StatusNotFound, `{"message":"Not Found"}`)
	_, err := c.FetchCompare(context.Background(), "o", "r", "aaa", "bbb")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFetchCompareDiff(t *testing.T) {
	t.Parallel()
	c := newCompareServer(t, http.StatusOK, "diff --git a/a.go b/a.go\n")
	raw, err := c.FetchCompareDiff(context.Background(), "o", "r", "aaa", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	if raw != "diff --git a/a.go b/a.go\n" {
		t.Errorf("raw = %q", raw)
	}
}

func TestFetchCompareDiffTooLarge(t *testing.T) {
	t.Parallel()
	c := newCompareServer(t, http.StatusNotAcceptable, `{"message":"too_large"}`)
	_, err := c.FetchCompareDiff(context.Background(), "o", "r", "aaa", "bbb")
	if !errors.Is(err, ErrDiffTooLarge) {
		t.Fatalf("err = %v, want ErrDiffTooLarge", err)
	}
}
