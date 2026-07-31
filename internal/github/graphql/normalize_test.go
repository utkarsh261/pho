package graphql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	githubpkg "github.com/utkarsh261/pho/internal/github"
	"github.com/utkarsh261/pho/internal/github/model"
)

func TestNormalizeDashboardResponse_Fixture(t *testing.T) {
	var resp model.DashboardResponse
	loadFixture(t, filepath.Join("dashboard", "response.json"), &resp)

	repo := testRepo()
	summaries, total, truncated, cursor, err := normalizeDashboardResponse(repo, resp.Data)
	if err != nil {
		t.Fatalf("normalize dashboard: %v", err)
	}
	if total != 3 || truncated {
		t.Fatalf("unexpected pagination: total=%d truncated=%v", total, truncated)
	}
	if cursor != "cursor_abc" {
		t.Fatalf("unexpected cursor: %q", cursor)
	}
	if len(summaries) != 3 {
		t.Fatalf("unexpected summary count: %d", len(summaries))
	}

	if got := summaries[0]; got.Number != 142 || got.Repo != "myorg/myrepo" || got.CIStatus != domain.CIStatusSuccess || got.Author != "alice" {
		t.Fatalf("unexpected first summary: %+v", got)
	}
	if got := summaries[0].RequestedReviewers; len(got) != 1 || got[0] != "bob" {
		t.Fatalf("unexpected requested reviewers: %#v", got)
	}
	if got := summaries[1]; got.Number != 138 || got.CIStatus != domain.CIStatusPending || got.ReviewDecision != domain.ReviewDecisionApproved {
		t.Fatalf("unexpected second summary: %+v", got)
	}
	if got := summaries[1].LatestReviews; len(got) != 1 || got[0].AuthorLogin != "carol" || got[0].CommitSHA != "def456abc789def456abc789def456abc789def4" {
		t.Fatalf("unexpected latest review summary: %#v", got)
	}
	if got := summaries[2]; !got.IsDraft || got.ReviewDecision != domain.ReviewDecisionNone || got.CIStatus != domain.CIStatusFailure {
		t.Fatalf("unexpected third summary: %+v", got)
	}
}

func TestNormalizeInvolvingResponse_Fixture(t *testing.T) {
	var resp model.InvolvingResponse
	loadFixture(t, filepath.Join("involving", "response.json"), &resp)

	repo := testRepo()
	summaries, total, truncated, err := normalizeInvolvingResponse(repo, resp.Data)
	if err != nil {
		t.Fatalf("normalize involving: %v", err)
	}
	if total != 2 || truncated {
		t.Fatalf("unexpected involving pagination: total=%d truncated=%v", total, truncated)
	}
	if len(summaries) != 2 {
		t.Fatalf("unexpected summary count: %d", len(summaries))
	}
	if got := summaries[1]; got.Number != 138 || got.Repo != "myorg/myrepo" || got.Author != "bob" || got.CIStatus != domain.CIStatusPending {
		t.Fatalf("unexpected involving summary: %+v", got)
	}
}

func TestNormalizePreviewResponse_Fixture(t *testing.T) {
	var resp model.PreviewResponse
	loadFixture(t, filepath.Join("preview", "response.json"), &resp)

	repo := testRepo()
	snapshot, err := normalizePreviewResponse(repo, 142, resp.Data)
	if err != nil {
		t.Fatalf("normalize preview: %v", err)
	}
	if snapshot.Number != 142 || snapshot.Repo != "myorg/myrepo" || snapshot.Author != "alice" {
		t.Fatalf("unexpected preview snapshot: %+v", snapshot)
	}
	if !strings.Contains(snapshot.BodyExcerpt, "nil pointer dereference") {
		t.Fatalf("unexpected body excerpt: %q", snapshot.BodyExcerpt)
	}
	if snapshot.ReviewDecision != domain.ReviewDecisionReviewRequired || snapshot.CIStatus != domain.CIStatusSuccess {
		t.Fatalf("unexpected preview status: %+v", snapshot)
	}
	if len(snapshot.Reviewers) != 3 || snapshot.Reviewers[0].Login != "carol" || len(snapshot.Checks) != 3 || len(snapshot.TopFiles) != 3 {
		t.Fatalf("unexpected preview details: reviewers=%#v checks=%#v files=%#v", snapshot.Reviewers, snapshot.Checks, snapshot.TopFiles)
	}
	if snapshot.LatestActivity != nil {
		t.Fatalf("expected nil latest activity in fixture: %+v", snapshot.LatestActivity)
	}
}

func TestNormalizePreviewPreservesHeadRefOID(t *testing.T) {
	t.Parallel()
	node := model.PullRequestNode{
		Number:     42,
		State:      "OPEN",
		CreatedAt:  "2026-07-30T12:00:00Z",
		UpdatedAt:  "2026-07-31T12:00:00Z",
		HeadRefOid: "def456",
	}
	snapshot, err := normalizePreviewNode(testRepo(), 42, node)
	if err != nil {
		t.Fatalf("normalize preview: %v", err)
	}
	if snapshot.HeadRefOID != "def456" {
		t.Fatalf("HeadRefOID=%q, want def456", snapshot.HeadRefOID)
	}
}

func TestPreviewReviewThreads(t *testing.T) {
	t.Parallel()
	five := 5
	one := 1
	conn := model.ReviewThreadConnection{
		Nodes: []model.ReviewThreadNode{
			{
				ID:         "thread1",
				Path:       "a.go",
				Line:       &five,
				IsResolved: false,
				Comments: model.ReviewThreadCommentConnection{
					Nodes: []model.ReviewThreadCommentNode{
						{ID: "c1", Author: &model.ActorNode{Login: "alice"}, Body: "first", CreatedAt: "2024-01-01T00:00:00Z"},
						{ID: "c2", Author: &model.ActorNode{Login: "bob"}, Body: "second", CreatedAt: "2024-01-02T00:00:00Z"},
					},
				},
			},
			{ID: "empty", Path: "b.go", Line: &one, Comments: model.ReviewThreadCommentConnection{}},
			{ID: "", Path: "c.go", Line: &one, Comments: model.ReviewThreadCommentConnection{
				Nodes: []model.ReviewThreadCommentNode{
					{ID: "c3", Author: &model.ActorNode{Login: "dave"}, Body: "orphan"},
				},
			}},
		},
	}
	threads := previewReviewThreads(conn)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].ID != "thread1" || threads[0].Path != "a.go" || threads[0].Line != 5 {
		t.Errorf("unexpected first thread: %+v", threads[0])
	}
	if len(threads[0].Comments) != 2 {
		t.Fatalf("expected 2 comments in first thread, got %d", len(threads[0].Comments))
	}
	if threads[0].Comments[0].ID != "c1" || threads[0].Comments[0].Login != "alice" {
		t.Errorf("unexpected first comment: %+v", threads[0].Comments[0])
	}
}

func TestPreviewCommentsWithIDs(t *testing.T) {
	t.Parallel()
	conn := model.IssueCommentConnection{
		Nodes: []model.IssueCommentNode{
			{ID: "c1", Author: &model.ActorNode{Login: "alice"}, Body: "hello", CreatedAt: "2024-01-01T00:00:00Z"},
			{ID: "c2", Author: &model.ActorNode{Login: "bob"}, Body: "world", CreatedAt: "2024-01-02T00:00:00Z"},
		},
	}
	comments := previewComments(conn)
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].ID != "c1" {
		t.Errorf("expected first comment ID=c1, got %q", comments[0].ID)
	}
	if comments[1].ID != "c2" {
		t.Errorf("expected second comment ID=c2, got %q", comments[1].ID)
	}
}

func TestDashboardQueryShapeByHostProfile(t *testing.T) {
	top := buildDashboardQuery(githubpkg.GitHubHostProfile{SupportsTopLevelRollup: true, SupportsHeadRefOID: true})
	if !strings.Contains(top, "statusCheckRollup {") || strings.Contains(top, "commits(last: 1)") {
		t.Fatalf("expected top-level rollup query, got:\n%s", top)
	}

	nested := buildDashboardQuery(githubpkg.GitHubHostProfile{SupportsTopLevelRollup: false, SupportsHeadRefOID: true})
	if !strings.Contains(nested, "commits(last: 1)") || !strings.Contains(nested, "statusCheckRollup {") {
		t.Fatalf("expected nested rollup query, got:\n%s", nested)
	}
}

func TestFetchDashboardPRs_RetriesWithNestedRollup(t *testing.T) {
	fixture := mustReadFixture(t, filepath.Join("dashboard", "response.json"))
	requests := 0

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++

		var req struct {
			Query string `json:"query"`
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		if requests == 1 {
			if !strings.Contains(req.Query, "statusCheckRollup {") || strings.Contains(req.Query, "commits(last: 1)") {
				t.Errorf("first request did not use top-level rollup query:\n%s", req.Query)
			}
			return jsonResponse(`{
				"data": {
					"repository": {
						"nameWithOwner": "myorg/myrepo",
						"pullRequests": {
							"totalCount": 0,
							"pageInfo": {"hasNextPage": false, "endCursor": ""},
							"nodes": []
						}
					}
				},
				"errors": [{"message": "Cannot query field \"statusCheckRollup\" on type \"PullRequest\"."}]
			}`), nil
		}

		if !strings.Contains(req.Query, "commits(last: 1)") {
			t.Errorf("second request did not use nested rollup query:\n%s", req.Query)
		}
		return jsonResponse(string(fixture)), nil
	})

	client := NewClient([]githubpkg.GitHubHostProfile{{
		Host:                   "example.com",
		GraphQLURL:             "http://example.com/graphql",
		Token:                  "token",
		SupportsTopLevelRollup: true,
		SupportsHeadRefOID:     true,
	}}, &http.Client{Transport: rt}, nil)

	repo := testRepo()
	repo.Host = "example.com"
	summaries, total, truncated, cursor, err := client.FetchDashboardPRs(context.Background(), repo)
	if err != nil {
		t.Fatalf("fetch dashboard: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
	if total != 3 || truncated || cursor != "cursor_abc" || len(summaries) != 3 {
		t.Fatalf("unexpected fetched data: total=%d truncated=%v cursor=%q summaries=%d", total, truncated, cursor, len(summaries))
	}
	if summaries[0].CIStatus != domain.CIStatusSuccess || summaries[1].CIStatus != domain.CIStatusPending {
		t.Fatalf("unexpected normalized status after fallback: %#v", summaries)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func loadFixture(t *testing.T, rel string, dest any) {
	t.Helper()
	data := mustReadFixture(t, rel)
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatalf("unmarshal %s: %v", rel, err)
	}
}

func mustReadFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "graphql", rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func testRepo() domain.Repository {
	return domain.Repository{
		Host:     "example.com",
		Owner:    "myorg",
		Name:     "myrepo",
		FullName: "myorg/myrepo",
	}
}

func TestTimelineCommitAuthor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		author *model.TimelineAuthor
		want   string
	}{
		{name: "nil author", author: nil, want: ""},
		{name: "user login preferred", author: &model.TimelineAuthor{User: &model.ActorNode{Login: "octocat"}, Name: "Octo Cat"}, want: "octocat"},
		{name: "git name fallback when user nil", author: &model.TimelineAuthor{User: nil, Name: "Utkarsh Sharma"}, want: "Utkarsh Sharma"},
		{name: "both empty", author: &model.TimelineAuthor{User: nil, Name: ""}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := timelineCommitAuthor(tc.author); got != tc.want {
				t.Errorf("timelineCommitAuthor() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeCommitsResponse(t *testing.T) {
	t.Parallel()

	resp := model.CommitsData{
		Repository: model.RepositoryNode{
			PullRequest: &model.PullRequestNode{
				Commits: model.CommitConnection{
					Nodes: []model.CommitNode{
						{
							Commit: model.CommitData{
								OID:             "sha1",
								MessageHeadline: "First commit",
								MessageBody:     "Body 1",
								CommittedDate:   "2024-01-01T00:00:00Z",
								Author: &model.CommitAuthor{
									Name: "Alice",
									User: &model.ActorNode{Login: "alice"},
								},
							},
						},
						{
							Commit: model.CommitData{
								OID:             "sha2",
								MessageHeadline: "Second commit",
								CommittedDate:   "2024-01-02T00:00:00Z",
								Author: &model.CommitAuthor{
									Name: "Bob",
									User: nil,
								},
							},
						},
					},
				},
			},
		},
	}

	commits := normalizeCommitsResponse(resp)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// GraphQL returns oldest first; normalizeCommitsResponse reverses to newest first.
	if commits[0].SHA != "sha2" {
		t.Errorf("expected first commit SHA=sha2 (newest), got %q", commits[0].SHA)
	}
	if commits[0].MessageHeadline != "Second commit" {
		t.Errorf("unexpected headline: %q", commits[0].MessageHeadline)
	}
	if commits[0].AuthorName != "Bob" {
		t.Errorf("unexpected author name: %q", commits[0].AuthorName)
	}
	if commits[0].AuthorLogin != "" {
		t.Errorf("expected empty login when user is nil, got %q", commits[0].AuthorLogin)
	}

	if commits[1].SHA != "sha1" {
		t.Errorf("expected second commit SHA=sha1 (oldest), got %q", commits[1].SHA)
	}
	if commits[1].AuthorLogin != "alice" {
		t.Errorf("unexpected author login: %q", commits[1].AuthorLogin)
	}
}

func TestNormalizeCommitsResponseEmpty(t *testing.T) {
	t.Parallel()
	resp := model.CommitsData{}
	commits := normalizeCommitsResponse(resp)
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for nil PR, got %d", len(commits))
	}
}

func TestNormalizePreviewResponse_ReviewThreadsFromJSON(t *testing.T) {
	raw := `{
	  "data": {
	    "repository": {
	      "nameWithOwner": "myorg/myrepo",
	      "pullRequest": {
	        "id": "PR_1",
	        "number": 42,
	        "title": "Test PR",
	        "body": "body text",
	        "state": "OPEN",
	        "isDraft": false,
	        "createdAt": "2026-06-01T00:00:00Z",
	        "updatedAt": "2026-06-02T00:00:00Z",
	        "headRefName": "main",
	        "baseRefName": "main",
	        "additions": 1,
	        "deletions": 1,
	        "changedFiles": 1,
	        "reviewDecision": "APPROVED",
	        "mergeable": "MERGEABLE",
	        "mergeStateStatus": "CLEAN",
	        "author": {"login": "alice"},
	        "assignees": {"nodes": []},
	        "reviews": {"nodes": [
	          {"author": {"login": "bob"}, "state": "COMMENTED", "submittedAt": "2026-06-01T12:00:00Z", "body": "test latest", "avatarUrl": ""}
	        ]},
	        "reviewThreads": {"nodes": [
	          {
	            "id": "thread1",
	            "path": "handlers.go",
	            "line": 103,
	            "isResolved": false,
	            "comments": {"nodes": [
	              {"id": "c1", "author": {"login": "bob"}, "body": "test", "createdAt": "2026-06-01T12:00:00Z"},
	              {"id": "c2", "author": {"login": "alice"}, "body": "test inline", "createdAt": "2026-06-02T12:00:00Z"}
	            ]}
	          },
	          {
	            "id": "thread2",
	            "path": "handlers.go",
	            "line": 110,
	            "isResolved": false,
	            "comments": {"nodes": [
	              {"id": "c3", "author": {"login": "bob"}, "body": "test new", "createdAt": "2026-06-01T12:00:00Z"}
	            ]}
	          }
	        ]},
	        "comments": {"nodes": []},
	        "files": {"nodes": []},
	        "timelineItems": {"nodes": []},
	        "repository": {"nameWithOwner": "myorg/myrepo"},
	        "statusCheckRollup": null,
	        "labels": {"nodes": []},
	        "reviewRequests": {"nodes": []},
	        "latestOpinionatedReviews": {"nodes": []}
	      }
	    }
	  }
	}`
	var resp model.PreviewResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	repo := domain.Repository{Host: "github.com", FullName: "myorg/myrepo"}
	snapshot, err := normalizePreviewResponse(repo, 42, resp.Data)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(snapshot.ReviewThreads) != 2 {
		t.Fatalf("expected 2 review threads, got %d", len(snapshot.ReviewThreads))
	}
	th1 := snapshot.ReviewThreads[0]
	if th1.ID != "thread1" || th1.Path != "handlers.go" || th1.Line != 103 {
		t.Errorf("unexpected thread1: %+v", th1)
	}
	if len(th1.Comments) != 2 {
		t.Fatalf("expected 2 comments in thread1, got %d", len(th1.Comments))
	}
	if th1.Comments[0].Login != "bob" || th1.Comments[0].Body != "test" {
		t.Errorf("unexpected thread1 comment0: %+v", th1.Comments[0])
	}
	if th1.Comments[1].Login != "alice" || th1.Comments[1].Body != "test inline" {
		t.Errorf("unexpected thread1 comment1: %+v", th1.Comments[1])
	}
	th2 := snapshot.ReviewThreads[1]
	if th2.ID != "thread2" || th2.Path != "handlers.go" || th2.Line != 110 {
		t.Errorf("unexpected thread2: %+v", th2)
	}
	if len(th2.Comments) != 1 {
		t.Fatalf("expected 1 comment in thread2, got %d", len(th2.Comments))
	}

	if len(snapshot.Reviewers) != 1 || snapshot.Reviewers[0].Login != "bob" {
		t.Errorf("unexpected reviewers: %+v", snapshot.Reviewers)
	}

	if len(snapshot.Reviewers[0].InlineComments) != 0 {
		t.Errorf("expected InlineComments to be empty, got %d", len(snapshot.Reviewers[0].InlineComments))
	}
}

func TestPreviewReviewThreads_LineNullFallback(t *testing.T) {
	t.Parallel()
	actual := 42
	outdatedLine := 99
	conn := model.ReviewThreadConnection{
		Nodes: []model.ReviewThreadNode{
			{
				ID:           "thread1",
				Path:         "a.go",
				Line:         nil,
				OriginalLine: &outdatedLine,
				IsResolved:   false,
				Comments: model.ReviewThreadCommentConnection{
					Nodes: []model.ReviewThreadCommentNode{
						{ID: "c1", Author: &model.ActorNode{Login: "alice"}, Body: "outdated", CreatedAt: "2024-01-01T00:00:00Z"},
					},
				},
			},
			{
				ID:           "thread2",
				Path:         "b.go",
				Line:         &actual,
				OriginalLine: nil,
				IsResolved:   false,
				Comments: model.ReviewThreadCommentConnection{
					Nodes: []model.ReviewThreadCommentNode{
						{ID: "c2", Author: &model.ActorNode{Login: "bob"}, Body: "current", CreatedAt: "2024-01-02T00:00:00Z"},
					},
				},
			},
			{
				ID:           "thread3",
				Path:         "c.go",
				Line:         nil,
				OriginalLine: nil,
				IsResolved:   false,
				Comments: model.ReviewThreadCommentConnection{
					Nodes: []model.ReviewThreadCommentNode{
						{ID: "c3", Author: &model.ActorNode{Login: "carol"}, Body: "no line", CreatedAt: "2024-01-03T00:00:00Z"},
					},
				},
			},
		},
	}
	threads := previewReviewThreads(conn)
	if len(threads) != 3 {
		t.Fatalf("expected 3 threads, got %d", len(threads))
	}
	if threads[0].Line != 99 {
		t.Errorf("expected thread1 Line=99 (originalLine fallback), got %d", threads[0].Line)
	}
	if threads[1].Line != 42 {
		t.Errorf("expected thread2 Line=42 (from line), got %d", threads[1].Line)
	}
	if threads[2].Line != 0 {
		t.Errorf("expected thread3 Line=0 (both null), got %d", threads[2].Line)
	}
}

func TestPreviewReviewThreads_GhostAuthor(t *testing.T) {
	t.Parallel()
	line := 10
	conn := model.ReviewThreadConnection{
		Nodes: []model.ReviewThreadNode{
			{
				ID:   "thread1",
				Path: "a.go",
				Line: &line,
				Comments: model.ReviewThreadCommentConnection{
					Nodes: []model.ReviewThreadCommentNode{
						{ID: "c1", Author: nil, Body: "deleted user comment", CreatedAt: "2024-01-01T00:00:00Z"},
						{ID: "c2", Author: &model.ActorNode{Login: ""}, Body: "empty login", CreatedAt: "2024-01-02T00:00:00Z"},
						{ID: "c3", Author: &model.ActorNode{Login: "alice"}, Body: "normal", CreatedAt: "2024-01-03T00:00:00Z"},
					},
				},
			},
		},
	}
	threads := previewReviewThreads(conn)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if len(threads[0].Comments) != 3 {
		t.Fatalf("expected 3 comments (including ghost authors), got %d", len(threads[0].Comments))
	}
	if threads[0].Comments[0].Login != "ghost" {
		t.Errorf("expected nil author to get login 'ghost', got %q", threads[0].Comments[0].Login)
	}
	if threads[0].Comments[1].Login != "ghost" {
		t.Errorf("expected empty login to get 'ghost', got %q", threads[0].Comments[1].Login)
	}
	if threads[0].Comments[2].Login != "alice" {
		t.Errorf("expected 'alice', got %q", threads[0].Comments[2].Login)
	}
}

func TestPreviewReviewThreads_GhostAuthorDoesNotDropThread(t *testing.T) {
	t.Parallel()
	line := 5
	conn := model.ReviewThreadConnection{
		Nodes: []model.ReviewThreadNode{
			{
				ID:   "thread1",
				Path: "a.go",
				Line: &line,
				Comments: model.ReviewThreadCommentConnection{
					Nodes: []model.ReviewThreadCommentNode{
						{ID: "c1", Author: nil, Body: "only comment, deleted author", CreatedAt: "2024-01-01T00:00:00Z"},
					},
				},
			},
		},
	}
	threads := previewReviewThreads(conn)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread (ghost author should not drop it), got %d", len(threads))
	}
	if threads[0].Comments[0].Login != "ghost" {
		t.Errorf("expected ghost login, got %q", threads[0].Comments[0].Login)
	}
}
