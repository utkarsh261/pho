package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
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

func TestFetchCommitDiffSuccess(t *testing.T) {
	t.Parallel()
	expectedDiff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != acceptDiffHeader {
			t.Errorf("expected Accept=%q, got %q", acceptDiffHeader, got)
		}
		if r.URL.Path != "/repos/owner/repo/commits/abc1234" {
			t.Errorf("expected path=/repos/owner/repo/commits/abc1234, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedDiff))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	diff, err := client.FetchCommitDiff(context.Background(), "owner", "repo", "abc1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != expectedDiff {
		t.Errorf("diff mismatch:\ngot:  %q\nwant: %q", diff, expectedDiff)
	}
}

func TestFetchCommitDiffNotFound(t *testing.T) {
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

	_, err := client.FetchCommitDiff(context.Background(), "owner", "repo", "badsha")
	if err == nil {
		t.Fatal("expected error for 404 response")
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

func TestFetchRepoInfoSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo" {
			t.Errorf("expected path=/repos/owner/repo, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != acceptJSONHeader {
			t.Errorf("expected Accept=%q, got %q", acceptJSONHeader, got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"default_branch": "main",
			"fork": false
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	info, err := client.FetchRepoInfo(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DefaultBranch != "main" {
		t.Errorf("expected default_branch=main, got %q", info.DefaultBranch)
	}
	if info.Fork {
		t.Error("expected fork=false")
	}
}

func TestFetchRepoInfoForkWithParent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"default_branch": "develop",
			"fork": true,
			"parent": {
				"full_name": "upstream-org/upstream-repo"
			}
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	info, err := client.FetchRepoInfo(context.Background(), "fork-owner", "fork-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DefaultBranch != "develop" {
		t.Errorf("expected default_branch=develop, got %q", info.DefaultBranch)
	}
	if !info.Fork {
		t.Error("expected fork=true")
	}
	if info.Parent == nil || info.Parent.FullName != "upstream-org/upstream-repo" {
		t.Errorf("expected parent full_name=upstream-org/upstream-repo, got %v", info.Parent)
	}
}

func TestFetchRepoInfoServerError(t *testing.T) {
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

	_, err := client.FetchRepoInfo(context.Background(), "owner", "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestCreatePullRequestSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected method=POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/pulls" {
			t.Errorf("expected path=/repos/owner/repo/pulls, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			t.Errorf("expected Authorization=%q, got %q", "token test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"number": 42,
			"title": "Add feature",
			"body": "Description here",
			"state": "open",
			"html_url": "https://github.com/owner/repo/pull/42",
			"head": {"ref": "feature-branch"},
			"base": {"ref": "main"},
			"draft": false,
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"user": {"login": "testuser"}
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	params := domain.CreatePRParams{
		Title: "Add feature",
		Body:  "Description here",
		Head:  "feature-branch",
		Base:  "main",
		Draft: false,
	}

	pr, err := client.CreatePullRequest(context.Background(), "owner", "repo", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected number=42, got %d", pr.Number)
	}
	if pr.Title != "Add feature" {
		t.Errorf("expected title=%q, got %q", "Add feature", pr.Title)
	}
	if pr.HTMLURL != "https://github.com/owner/repo/pull/42" {
		t.Errorf("expected html_url=%q, got %q", "https://github.com/owner/repo/pull/42", pr.HTMLURL)
	}
	if pr.Head.Ref != "feature-branch" {
		t.Errorf("expected head=%q, got %q", "feature-branch", pr.Head.Ref)
	}
	if pr.Base.Ref != "main" {
		t.Errorf("expected base=%q, got %q", "main", pr.Base.Ref)
	}
	if pr.IsDraft {
		t.Error("expected draft=false")
	}
}

func TestCreatePullRequestDraft(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"number": 1,
			"title": "WIP: Feature",
			"state": "open",
			"html_url": "https://github.com/owner/repo/pull/1",
			"head": {"ref": "wip-branch"},
			"base": {"ref": "main"},
			"draft": true,
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"user": {"login": "testuser"}
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	params := domain.CreatePRParams{
		Title: "WIP: Feature",
		Head:  "wip-branch",
		Base:  "main",
		Draft: true,
	}

	pr, err := client.CreatePullRequest(context.Background(), "owner", "repo", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pr.IsDraft {
		t.Error("expected draft=true")
	}
}

func TestCreatePullRequestEmptyBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"number": 1,
			"title": "No body PR",
			"state": "open",
			"html_url": "https://github.com/owner/repo/pull/1",
			"head": {"ref": "branch"},
			"base": {"ref": "main"},
			"draft": false,
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"user": {"login": "testuser"}
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	params := domain.CreatePRParams{
		Title: "No body PR",
		Head:  "branch",
		Base:  "main",
	}

	_, err := client.CreatePullRequest(context.Background(), "owner", "repo", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePullRequestValidationError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"message": "Validation Failed",
			"errors": [{"resource": "PullRequest", "field": "base", "code": "invalid"}]
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	params := domain.CreatePRParams{
		Title: "Test",
		Head:  "branch",
		Base:  "nonexistent-branch",
	}

	_, err := client.CreatePullRequest(context.Background(), "owner", "repo", params)
	if err == nil {
		t.Fatal("expected error for validation failure")
	}
}

func TestCreatePullRequestServerError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message": "Internal Server Error"}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
	}

	params := domain.CreatePRParams{
		Title: "Test",
		Head:  "branch",
		Base:  "main",
	}

	_, err := client.CreatePullRequest(context.Background(), "owner", "repo", params)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestBuildRepoURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		baseURL string
		owner   string
		repo    string
		want    string
	}{
		{
			baseURL: "https://api.github.com",
			owner:   "owner",
			repo:    "repo",
			want:    "https://api.github.com/repos/owner/repo",
		},
		{
			baseURL: "https://github.example.com/api/v3",
			owner:   "org",
			repo:    "project",
			want:    "https://github.example.com/api/v3/repos/org/project",
		},
	}

	for _, tc := range tests {
		got := buildRepoURL(tc.baseURL, tc.owner, tc.repo)
		if got != tc.want {
			t.Errorf("buildRepoURL(%q, %q, %q) = %q, want %q",
				tc.baseURL, tc.owner, tc.repo, got, tc.want)
		}
	}
}

func TestBuildCreatePRURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		baseURL string
		owner   string
		repo    string
		want    string
	}{
		{
			baseURL: "https://api.github.com",
			owner:   "owner",
			repo:    "repo",
			want:    "https://api.github.com/repos/owner/repo/pulls",
		},
		{
			baseURL: "https://github.example.com/api/v3",
			owner:   "org",
			repo:    "project",
			want:    "https://github.example.com/api/v3/repos/org/project/pulls",
		},
	}

	for _, tc := range tests {
		got := buildCreatePRURL(tc.baseURL, tc.owner, tc.repo)
		if got != tc.want {
			t.Errorf("buildCreatePRURL(%q, %q, %q) = %q, want %q",
				tc.baseURL, tc.owner, tc.repo, got, tc.want)
		}
	}
}

func TestUpdateBranchSuccess202(t *testing.T) {
	t.Parallel()
	var capturedMethod, capturedPath string
	var capturedHeaders = map[string]string{}
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedHeaders["Accept"] = r.Header.Get("Accept")
		capturedHeaders["Content-Type"] = r.Header.Get("Content-Type")
		capturedHeaders["User-Agent"] = r.Header.Get("User-Agent")
		capturedHeaders["Authorization"] = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message":"update-branch queued","url":"https://api.github.com/repos/owner/repo/pulls/42"}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Token: "test-token"}
	err := client.UpdateBranch(context.Background(), "owner", "repo", 42, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("expected method=PUT, got %s", capturedMethod)
	}
	if capturedPath != "/repos/owner/repo/pulls/42/update-branch" {
		t.Errorf("expected path=/repos/owner/repo/pulls/42/update-branch, got %s", capturedPath)
	}
	if capturedHeaders["Accept"] != acceptJSONHeader {
		t.Errorf("expected Accept=%q, got %q", acceptJSONHeader, capturedHeaders["Accept"])
	}
	if capturedHeaders["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", capturedHeaders["Content-Type"])
	}
	if capturedHeaders["User-Agent"] != userAgentHeader {
		t.Errorf("expected User-Agent=%q, got %q", userAgentHeader, capturedHeaders["User-Agent"])
	}
	if capturedHeaders["Authorization"] != "token test-token" {
		t.Errorf("expected Authorization=%q, got %q", "token test-token", capturedHeaders["Authorization"])
	}
	if string(capturedBody) != `{}` {
		t.Errorf("expected empty body `{}`, got %q", string(capturedBody))
	}
}

func TestUpdateBranchWithExpectedHeadSHA(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Token: "test-token"}
	err := client.UpdateBranch(context.Background(), "owner", "repo", 42, "6dcb09b5b57875f334f61aebed695e2e4193db5e")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(capturedBody, &got); err != nil {
		t.Fatalf("body not valid JSON: %v (body=%q)", err, string(capturedBody))
	}
	sha, ok := got["expected_head_sha"]
	if !ok {
		t.Fatalf("expected expected_head_sha key in body, got %q", string(capturedBody))
	}
	if sha != "6dcb09b5b57875f334f61aebed695e2e4193db5e" {
		t.Errorf("expected expected_head_sha=6dcb..., got %v", sha)
	}
	// No other keys should be present.
	if len(got) != 1 {
		t.Errorf("expected exactly 1 key, got %d (%v)", len(got), got)
	}
}

func TestUpdateBranchEmptyBodyWhenNoExpectedSHA(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Token: "test-token"}
	if err := client.UpdateBranch(context.Background(), "owner", "repo", 42, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(capturedBody, &got); err != nil {
		t.Fatalf("body not valid JSON: %v (body=%q)", err, string(capturedBody))
	}
	if _, ok := got["expected_head_sha"]; ok {
		t.Errorf("expected no expected_head_sha key, got %q", string(capturedBody))
	}
	if len(got) != 0 {
		t.Errorf("expected empty JSON object, got %v", got)
	}
}

func TestUpdateBranchNon202ReturnsError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Validation Failed: branch is not behind base"}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Token: "test-token"}
	err := client.UpdateBranch(context.Background(), "owner", "repo", 42, "")
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected error to contain status code 422, got %v", err)
	}
	if !strings.Contains(err.Error(), "Validation Failed") {
		t.Errorf("expected error to contain response body, got %v", err)
	}
}

func TestUpdateBranchNetworkError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	// Close before sending the request.
	server.Close()

	client := &Client{BaseURL: server.URL, Token: "test-token"}
	err := client.UpdateBranch(context.Background(), "owner", "repo", 42, "")
	if err == nil {
		t.Fatal("expected error for closed server")
	}
	if !strings.Contains(err.Error(), "rest: request failed") {
		t.Errorf("expected wrapped network error, got %v", err)
	}
}

func TestUpdateBranchCtxCancelled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &Client{BaseURL: server.URL, Token: "test-token"}
	err := client.UpdateBranch(ctx, "owner", "repo", 42, "")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestUpdateBranchURLBuilder(t *testing.T) {
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
			want:    "https://api.github.com/repos/owner/repo/pulls/42/update-branch",
		},
		{
			baseURL: "https://github.example.com/api/v3",
			owner:   "org",
			repo:    "project",
			number:  7,
			want:    "https://github.example.com/api/v3/repos/org/project/pulls/7/update-branch",
		},
	}
	for _, tc := range tests {
		got := buildUpdateBranchURL(tc.baseURL, tc.owner, tc.repo, tc.number)
		if got != tc.want {
			t.Errorf("buildUpdateBranchURL(%q, %q, %q, %d) = %q, want %q",
				tc.baseURL, tc.owner, tc.repo, tc.number, got, tc.want)
		}
	}
}
