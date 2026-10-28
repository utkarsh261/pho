package graphql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	githubpkg "github.com/utkarsh261/pho/internal/github"
	"github.com/utkarsh261/pho/internal/github/model"
)

func TestSummarySelectionStackFields(t *testing.T) {
	on := buildDashboardQuery(githubpkg.GitHubHostProfile{SupportsPRStack: true})
	if !strings.Contains(on, "stackEntry { position }") || !strings.Contains(on, "stack { number") {
		t.Fatalf("expected stack fields, got:\n%s", on)
	}
	off := buildDashboardQuery(githubpkg.GitHubHostProfile{})
	if strings.Contains(off, "stack") {
		t.Fatalf("expected no stack fields, got:\n%s", off)
	}
}

func TestNormalizeNativeStack(t *testing.T) {
	var node model.PullRequestNode
	err := json.Unmarshal([]byte(`{
		"number": 21, "state": "OPEN", "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
		"stackEntry": {"position": 2},
		"stack": {"number": 4, "size": 3, "baseRefName": "main", "entries": {"nodes": [
			{"position": 3, "pullRequest": {"number": 22, "title": "top", "state": "OPEN", "isDraft": true}},
			{"position": 1, "pullRequest": {"number": 20, "title": "base", "state": "MERGED"}},
			{"position": 2, "pullRequest": {"number": 21, "title": "mid", "state": "OPEN"}}
		]}}
	}`), &node)
	if err != nil {
		t.Fatal(err)
	}
	s, err := normalizePullRequestSummary(testRepo(), node)
	if err != nil {
		t.Fatal(err)
	}
	ns := s.NativeStack
	if ns == nil || ns.Number != 4 || ns.Trunk != "main" || ns.Position != 2 || len(ns.Layers) != 3 {
		t.Fatalf("native stack = %+v", ns)
	}
	if ns.Layers[0].Number != 20 || ns.Layers[0].State != domain.PRStateMerged || !ns.Layers[2].IsDraft {
		t.Fatalf("layers not ordered by position: %+v", ns.Layers)
	}

	node.Stack, node.StackEntry = nil, nil
	if s, _ := normalizePullRequestSummary(testRepo(), node); s.NativeStack != nil {
		t.Fatalf("expected no stack, got %+v", s.NativeStack)
	}
}

func TestFetchDashboardPRs_DropsStackFieldsWhenUnsupported(t *testing.T) {
	fixture := mustReadFixture(t, filepath.Join("dashboard", "response.json"))
	requests := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		var req struct {
			Query string `json:"query"`
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}
		if requests == 1 {
			if !strings.Contains(req.Query, "stackEntry") {
				t.Errorf("first request should ask for stacks:\n%s", req.Query)
			}
			return jsonResponse(`{"data": null, "errors": [{"message": "Field 'stackEntry' doesn't exist on type 'PullRequest'"}]}`), nil
		}
		if strings.Contains(req.Query, "stack") {
			t.Errorf("retry should not ask for stacks:\n%s", req.Query)
		}
		return jsonResponse(string(fixture)), nil
	})

	client := NewClient([]githubpkg.GitHubHostProfile{{
		Host:                   "example.com",
		GraphQLURL:             "http://example.com/graphql",
		Token:                  "token",
		SupportsTopLevelRollup: true,
		SupportsHeadRefOID:     true,
		SupportsPRStack:        true,
	}}, &http.Client{Transport: rt}, nil)

	repo := testRepo()
	repo.Host = "example.com"
	summaries, _, _, _, err := client.FetchDashboardPRs(context.Background(), repo)
	if err != nil {
		t.Fatalf("fetch dashboard: %v", err)
	}
	if requests != 2 || len(summaries) != 3 {
		t.Fatalf("requests=%d summaries=%d", requests, len(summaries))
	}
}
