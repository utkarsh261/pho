package pr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
)

// stackedMergeClient rejects GraphQL merges the way GitHub does for a PR in
// a native stack.
type stackedMergeClient struct{ fakeGitHubClient }

func (*stackedMergeClient) MergePullRequest(_ context.Context, _, _, _, _ string) error {
	return errors.New("graphql query failed: This pull request is part of a stack and must be merged using the asynchronous merge REST API.")
}

func asyncMergeService(t *testing.T, handler http.HandlerFunc) *PRService {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &PRService{
		Cache:  newTestCoordinator(t),
		Client: &stackedMergeClient{},
		REST:   &rest.Client{BaseURL: server.URL, HTTPClient: server.Client(), Token: "t"},
		Owner:  "owner",
		Repo:   "repo",
		Log:    pholog.NewNop(),
	}
}

func TestMergePRFallsBackToAsyncMergeForStacks(t *testing.T) {
	t.Parallel()
	var put map[string]any
	polls := 0
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/repos/owner/repo/pulls/42/merge-async":
			_ = json.NewDecoder(r.Body).Decode(&put)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending","details":{"uuid":"u1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42/merge-async/u1":
			polls++
			_, _ = w.Write([]byte(`{"status":"merged","details":{"sha":"abc"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if err := svc.MergePR(context.Background(), testRepo(), 42, "pr_1", "head1", "SQUASH"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if put["merge_method"] != "squash" || put["sha"] != "head1" {
		t.Errorf("merge-async body = %v", put)
	}
	if polls != 1 {
		t.Errorf("polled %d times, want 1", polls)
	}
}

func TestMergePRAsyncFailureCarriesGitHubReason(t *testing.T) {
	t.Parallel()
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"failed","details":{"message":"Required status check \"ci\" is failing"}}`))
	})
	err := svc.MergePR(context.Background(), testRepo(), 42, "pr_1", "head1", "MERGE")
	if err == nil || !strings.Contains(err.Error(), `"ci" is failing`) {
		t.Fatalf("err = %v, want GitHub's reason", err)
	}
}

func TestMergePRKeepsGraphQLForUnstackedPRs(t *testing.T) {
	t.Parallel()
	svc := &PRService{Cache: newTestCoordinator(t), Client: &fakeGitHubClient{}, Log: pholog.NewNop()}
	if err := svc.MergePR(context.Background(), testRepo(), 42, "pr_1", "head1", "MERGE"); err != nil {
		t.Fatalf("merge: %v", err)
	}
}

func TestMergeStackedPRGoesStraightToAsync(t *testing.T) {
	t.Parallel()
	puts := 0
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		puts++
		_, _ = w.Write([]byte(`{"status":"merged","details":{}}`))
	})
	svc.Client = &fakeGitHubClient{} // a GraphQL merge would succeed: it must not be tried
	if err := svc.MergeStackedPR(context.Background(), testRepo(), 42, "head1", "MERGE"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if puts != 1 {
		t.Errorf("merge-async calls = %d, want 1", puts)
	}
}

func TestMergeAsyncPollAccepts202(t *testing.T) {
	t.Parallel()
	polls := 0
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending","details":{"uuid":"u1"}}`))
			return
		}
		polls++
		if polls == 1 {
			w.WriteHeader(http.StatusAccepted) // still running
			_, _ = w.Write([]byte(`{"status":"pending","details":{"uuid":"u1"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"merged","details":{}}`))
	})
	if err := svc.MergeStackedPR(context.Background(), testRepo(), 42, "head1", "MERGE"); err != nil {
		t.Fatalf("a 202 while polling is not a failure: %v", err)
	}
}

func TestMergeAsyncConflictWithoutMergeIsAnError(t *testing.T) {
	t.Parallel()
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Head branch was modified"}`))
	})
	err := svc.MergeStackedPR(context.Background(), testRepo(), 42, "head1", "MERGE")
	if err == nil || !strings.Contains(err.Error(), "Head branch was modified") {
		t.Fatalf("err = %v, want GitHub's message", err)
	}
}

func TestMergeAsyncLongErrorKeepsGitHubsReason(t *testing.T) {
	t.Parallel()
	reason := "Repository rule violations found: " + strings.Repeat("a required check is missing; ", 30)
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message":           reason,
			"documentation_url": "https://docs.github.com/rest/pulls/pulls#merge-a-pull-request-asynchronously",
		})
	})
	err := svc.MergeStackedPR(context.Background(), testRepo(), 42, "head1", "MERGE")
	if err == nil || err.Error() != reason {
		t.Fatalf("err = %v, want GitHub's whole reason (%d bytes)", err, len(reason))
	}
}

// Not parallel: it shortens the package's merge deadline.
func TestMergeAsyncStillRunningIsNotSuccess(t *testing.T) {
	wait := mergeAsyncWait
	mergeAsyncWait = 0
	defer func() { mergeAsyncWait = wait }()
	svc := asyncMergeService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"pending","details":{"uuid":"u1"}}`))
	})
	err := svc.MergeStackedPR(context.Background(), testRepo(), 42, "head1", "MERGE")
	if !errors.Is(err, domain.ErrMergeInProgress) {
		t.Fatalf("err = %v, want ErrMergeInProgress", err)
	}
}
