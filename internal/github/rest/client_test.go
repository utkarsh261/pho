package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRawDiffSuccess(t *testing.T) {
	t.Parallel()
	expectedDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Accept header.
		if got := r.Header.Get("Accept"); got != acceptDiffHeader {
			t.Errorf("expected Accept=%q, got %q", acceptDiffHeader, got)
		}
		// Verify Auth header.
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			t.Errorf("expected Authorization=%q, got %q", "token test-token", got)
		}
		// Verify URL path.
		if r.URL.Path != "/repos/owner/repo/pulls/42" {
			t.Errorf("expected path=/repos/owner/repo/pulls/42, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/x-diff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedDiff))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	diff, err := client.FetchRawDiff(context.Background(), "owner", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != expectedDiff {
		t.Errorf("diff mismatch:\ngot:  %q\nwant: %q", diff, expectedDiff)
	}
}

func TestFetchRawDiffServerError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	_, err := client.FetchRawDiff(context.Background(), "owner", "repo", 42)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestFetchRawDiffURLBuilder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		baseURL string
		owner   string
		repo    string
		number  int
		want    string
	}{
		{
			baseURL: "https://api.github.com",
			owner:   "owner",
			repo:    "repo",
			number:  42,
			want:    "https://api.github.com/repos/owner/repo/pulls/42",
		},
		{
			baseURL: "https://github.example.com/api/v3",
			owner:   "org",
			repo:    "project",
			number:  1,
			want:    "https://github.example.com/api/v3/repos/org/project/pulls/1",
		},
	}

	for _, tc := range tests {
		got := buildDiffURL(tc.baseURL, tc.owner, tc.repo, tc.number)
		if got != tc.want {
			t.Errorf("buildDiffURL(%q, %q, %q, %d) = %q, want %q",
				tc.baseURL, tc.owner, tc.repo, tc.number, got, tc.want)
		}
	}
}

func TestFetchRawDiffUserAgent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgentHeader {
			t.Errorf("expected User-Agent=%q, got %q", userAgentHeader, got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	_, err := client.FetchRawDiff(context.Background(), "owner", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchRawDiffNilHTTPClient(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Client with nil HTTPClient should fall back to http.DefaultClient.
	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	_, err := client.FetchRawDiff(context.Background(), "owner", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error with nil HTTPClient: %v", err)
	}
}

func TestFetchRawDiffContextCancelled(t *testing.T) {
	t.Parallel()
	// Use a server that never responds — cancel the context immediately.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block forever — context will be cancelled.
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}
	_, err := client.FetchRawDiff(ctx, "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
