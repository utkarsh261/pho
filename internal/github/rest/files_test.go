package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// apiPrefixes covers github.com (API at the root) and GitHub Enterprise
// Server (API under /api/v3).
var apiPrefixes = []string{"", "/api/v3"}

func filesPage(start, n int) []ChangedFile {
	out := make([]ChangedFile, n)
	for i := range out {
		out[i] = ChangedFile{Filename: fmt.Sprintf("f%04d.go", start+i), Status: "modified", Patch: "@@ -1 +1 @@\n-a\n+b"}
	}
	return out
}

// serveFiles serves total files across pages of 100 under prefix+path,
// advertising rel="last" when withLast, else only rel="next".
func serveFiles(t *testing.T, prefix, path string, total int, withLast bool, wrap func([]ChangedFile) any) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	pages := (total + filesPerPage - 1) / filesPerPage
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != prefix+path {
			t.Errorf("path = %s, want %s", r.URL.Path, prefix+path)
		}
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q", got)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var links []string
		if page < pages {
			links = append(links, fmt.Sprintf(`<%s%s%s?per_page=100&page=%d>; rel="next"`, srv.URL, prefix, path, page+1))
			if withLast {
				links = append(links, fmt.Sprintf(`<%s%s%s?per_page=100&page=%d>; rel="last"`, srv.URL, prefix, path, pages))
			}
		}
		w.Header().Set("Link", strings.Join(links, ", "))
		start := (page - 1) * filesPerPage
		n := min(filesPerPage, max(total-start, 0))
		_ = json.NewEncoder(w).Encode(wrap(filesPage(start, n)))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func asList(f []ChangedFile) any { return f }

func TestFetchPRFilesPagesInOrder(t *testing.T) {
	t.Parallel()
	for _, prefix := range apiPrefixes {
		for _, withLast := range []bool{true, false} {
			srv, _ := serveFiles(t, prefix, "/repos/o/r/pulls/7/files", 326, withLast, asList)
			c := &Client{BaseURL: srv.URL + prefix, Token: "tok", HTTPClient: srv.Client()}
			files, err := c.FetchPRFiles(context.Background(), "o", "r", 7)
			if err != nil {
				t.Fatalf("prefix %q last=%v: %v", prefix, withLast, err)
			}
			if len(files) != 326 {
				t.Fatalf("prefix %q last=%v: got %d files", prefix, withLast, len(files))
			}
			for i, f := range files {
				if want := fmt.Sprintf("f%04d.go", i); f.Filename != want {
					t.Fatalf("file %d = %s, want %s", i, f.Filename, want)
				}
			}
		}
	}
}

func TestFetchPRFilesStopsAtGitHubLimit(t *testing.T) {
	t.Parallel()
	srv, calls := serveFiles(t, "", "/repos/o/r/pulls/7/files", 4000, true, asList)
	c := &Client{BaseURL: srv.URL, Token: "tok", HTTPClient: srv.Client()}
	files, err := c.FetchPRFiles(context.Background(), "o", "r", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3000 || calls.Load() != 30 {
		t.Fatalf("got %d files in %d requests, want 3000 in 30", len(files), calls.Load())
	}
}

func TestFetchCommitFiles(t *testing.T) {
	t.Parallel()
	for _, prefix := range apiPrefixes {
		srv, _ := serveFiles(t, prefix, "/repos/o/r/commits/abc", 150, true, func(f []ChangedFile) any {
			return map[string]any{"sha": "abc", "files": f}
		})
		c := &Client{BaseURL: srv.URL + prefix, Token: "tok", HTTPClient: srv.Client()}
		files, err := c.FetchCommitFiles(context.Background(), "o", "r", "abc")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 150 {
			t.Fatalf("prefix %q: got %d files", prefix, len(files))
		}
	}
}

func TestFetchFilesPageError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Link", `<x?page=2>; rel="next", <x?per_page=100&page=3>; rel="last"`)
		_ = json.NewEncoder(w).Encode(filesPage(0, 100))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "tok", HTTPClient: srv.Client()}
	if _, err := c.FetchPRFiles(context.Background(), "o", "r", 7); err == nil {
		t.Fatal("expected an error when a page fails")
	}
}

func TestDiffTooLargeDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"github.com files limit", 406, `{"message":"Sorry, the diff exceeded the maximum number of files (300).","errors":[{"resource":"PullRequest","field":"diff","code":"too_large"}]}`, true},
		{"enterprise custom message", 406, `{"message":"Sorry, this diff is temporarily unavailable due to heavy server load."}`, true},
		{"too_large on another status", 422, `{"errors":[{"code":"too_large"}]}`, true},
		{"not found", 404, `{"message":"Not Found"}`, false},
		{"server error", 500, `oops`, false},
	}
	for _, tc := range cases {
		for _, prefix := range apiPrefixes {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			c := &Client{BaseURL: srv.URL + prefix, Token: "tok", HTTPClient: srv.Client()}
			_, prErr := c.FetchRawDiff(context.Background(), "o", "r", 1)
			_, commitErr := c.FetchCommitDiff(context.Background(), "o", "r", "abc")
			srv.Close()
			for _, err := range []error{prErr, commitErr} {
				if err == nil {
					t.Fatalf("%s: expected error", tc.name)
				}
				if got := errors.Is(err, ErrDiffTooLarge); got != tc.want {
					t.Errorf("%s (prefix %q): errors.Is(ErrDiffTooLarge) = %v, want %v", tc.name, prefix, got, tc.want)
				}
			}
		}
	}
}
