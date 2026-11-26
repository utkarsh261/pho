package graphql

import (
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	githubpkg "github.com/utkarsh261/pho/internal/github"
)

func TestSearchTabQuery(t *testing.T) {
	repo := domain.Repository{Owner: "acme", Name: "web", FullName: "acme/web"}
	for _, tc := range []struct{ query, want string }{
		{"author:alice author:bob", "repo:acme/web is:pr is:open author:alice author:bob"},
		{"author:@me is:merged", "repo:acme/web is:pr author:@me is:merged"},
		{"IS:CLOSED label:x", "repo:acme/web is:pr IS:CLOSED label:x"},
		{"state:open", "repo:acme/web is:pr state:open"},
		{"-is:draft", "repo:acme/web is:pr is:open -is:draft"},
		{"label:is:merged-later", "repo:acme/web is:pr is:open label:is:merged-later"},
	} {
		if got := SearchTabQuery(repo, tc.query); got != tc.want {
			t.Errorf("SearchTabQuery(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}

	noFull := domain.Repository{Owner: "acme", Name: "web"}
	if got := SearchTabQuery(noFull, "label:x"); got != "repo:acme/web is:pr is:open label:x" {
		t.Errorf("without FullName = %q", got)
	}
}

func TestCustomTabQueryOperation(t *testing.T) {
	q := buildCustomTabQuery(githubpkg.GitHubHostProfile{})
	if !strings.HasPrefix(q, "query CustomTabPRsQuery($query: String!)") {
		t.Fatalf("query = %.60q", q)
	}
	if !strings.HasPrefix(buildInvolvingQuery(githubpkg.GitHubHostProfile{}), "query InvolvingPRsQuery(") {
		t.Fatal("involving query lost its operation name")
	}
}
