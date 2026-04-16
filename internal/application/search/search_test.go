package search_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/application/search"
	"github.com/utkarsh261/pho/internal/domain"
)

func TestService_SearchPRsExactNumberRanksFirst(t *testing.T) {
	svc := search.New()
	repo := makeRepo("org/one")
	other := makeRepo("org/two")

	mustBuildPRIndex(t, svc, repo, prSnap(repo,
		pr(42, "Refactor parser", "refactor/parser", time.Now().Add(-2*time.Hour)),
		pr(7, "Fix build", "build/fix", time.Now().Add(-1*time.Hour)),
	))
	mustBuildPRIndex(t, svc, other, prSnap(other,
		pr(42, "Nearby exact match but older", "feature/exact", time.Now().Add(-30*time.Hour)),
	))

	got := svc.SearchPRs("42", 10)
	if len(got) == 0 {
		t.Fatal("expected results")
	}
	if got[0].Repo != repo.FullName || got[0].Number != 42 {
		t.Fatalf("first result = %#v, want exact number match from %s#42", got[0], repo.FullName)
	}
}

func TestService_SearchPRsTitlePrefixRanksAboveBranchPrefix(t *testing.T) {
	svc := search.New()
	repo := makeRepo("org/one")

	mustBuildPRIndex(t, svc, repo, prSnap(repo,
		pr(1, "fix login flow", "topic/login", time.Now().Add(-1*time.Hour)),
		pr(2, "unrelated title", "fix/login-branch", time.Now().Add(-30*time.Minute)),
	))

	got := svc.SearchPRs("fix", 10)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(got))
	}
	if got[0].Number != 1 {
		t.Fatalf("first result = %#v, want title-prefix match #1 before branch-prefix match", got[0])
	}
}

func TestService_CurrentRepoAndTabBoostsLocalMatches(t *testing.T) {
	svc := search.New()
	current := makeRepo("org/current")
	other := makeRepo("org/other")

	mustBuildPRIndex(t, svc, current, prSnap(current,
		pr(10, "fix auth", "topic/auth", time.Now().Add(-8*time.Hour)),
		pr(11, "small change", "topic/small", time.Now().Add(-1*time.Hour)),
	))
	mustBuildPRIndex(t, svc, other, prSnap(other,
		pr(20, "fix auth", "topic/auth", time.Now().Add(-5*time.Minute)),
	))

	svc.SetCurrentRepo(current.FullName)
	svc.SetCurrentTab(domain.TabNeedsReview)
	svc.SetPRTabs(current.FullName, 10, domain.TabNeedsReview)

	got := svc.SearchPRs("fix", 10)
	if len(got) == 0 {
		t.Fatal("expected results")
	}
	if got[0].Repo != current.FullName || got[0].Number != 10 {
		t.Fatalf("first result = %#v, want current-repo/tab boosted result %s#10", got[0], current.FullName)
	}
}

func TestService_RebuildPRIndexReplacesSnapshot(t *testing.T) {
	svc := search.New()
	repo := makeRepo("org/repo")

	mustBuildPRIndex(t, svc, repo, prSnap(repo,
		pr(1, "old title", "branch/old", time.Now().Add(-4*time.Hour)),
	))
	if got := svc.SearchPRs("old", 10); len(got) != 1 || got[0].Number != 1 {
		t.Fatalf("initial search = %#v", got)
	}

	mustBuildPRIndex(t, svc, repo, prSnap(repo,
		pr(1, "new title", "branch/new", time.Now().Add(-2*time.Hour)),
	))
	got := svc.SearchPRs("new", 10)
	if len(got) != 1 || got[0].Number != 1 {
		t.Fatalf("rebuilt search = %#v", got)
	}
	if got[0].Title != "new title" {
		t.Fatalf("rebuilt entry = %#v, want updated title", got[0])
	}
}

func TestService_RebuildRepoIndexReplacesSnapshot(t *testing.T) {
	svc := search.New()
	first := makeRepo("org/alpha")
	second := makeRepo("org/beta")

	mustBuildRepoIndex(t, svc, []domain.Repository{first})
	if got := svc.SearchRepos("alpha", 10); len(got) != 1 || got[0].Repo != first.FullName {
		t.Fatalf("initial repo search = %#v", got)
	}

	mustBuildRepoIndex(t, svc, []domain.Repository{second})
	if got := svc.SearchRepos("alpha", 10); len(got) != 0 {
		t.Fatalf("stale repo search should be empty, got %#v", got)
	}
	if got := svc.SearchRepos("beta", 10); len(got) != 1 || got[0].Repo != second.FullName {
		t.Fatalf("rebuilt repo search = %#v", got)
	}
}

func TestService_SearchPRsPerformance500(t *testing.T) {
	svc := search.New()
	repo := makeRepo("org/big")

	prs := make([]domain.PullRequestSummary, 0, 500)
	base := time.Now()
	for i := 0; i < 500; i++ {
		prs = append(prs, pr(i+1, fmt.Sprintf("feature update %d", i), fmt.Sprintf("branch/%d", i), base.Add(-time.Duration(i)*time.Minute)))
	}
	mustBuildPRIndex(t, svc, repo, prSnap(repo, prs...))

	start := time.Now()
	got := svc.SearchPRs("feature", 50)
	elapsed := time.Since(start)
	if len(got) == 0 {
		t.Fatal("expected search results")
	}
	if elapsed > 10*time.Millisecond {
		t.Fatalf("search took %s, expected <= 10ms for 500 synthetic PRs", elapsed)
	}
}

func mustBuildPRIndex(t *testing.T, svc *search.Service, repo domain.Repository, snap domain.DashboardSnapshot) {
	t.Helper()
	if err := svc.BuildPRIndex(repo, snap); err != nil {
		t.Fatalf("BuildPRIndex() error = %v", err)
	}
}

func mustBuildRepoIndex(t *testing.T, svc *search.Service, repos []domain.Repository) {
	t.Helper()
	if err := svc.BuildRepoIndex(repos); err != nil {
		t.Fatalf("BuildRepoIndex() error = %v", err)
	}
}

func makeRepo(full string) domain.Repository {
	owner, name, _ := splitRepo(full)
	return domain.Repository{FullName: full, Owner: owner, Name: name, LocalPath: "/tmp/" + name}
}

func pr(number int, title, branch string, updatedAt time.Time) domain.PullRequestSummary {
	return domain.PullRequestSummary{
		Number:      number,
		Title:       title,
		HeadRefName: branch,
		Author:      "octocat",
		State:       domain.PRStateOpen,
		UpdatedAt:   updatedAt,
	}
}

func prSnap(repo domain.Repository, prs ...domain.PullRequestSummary) domain.DashboardSnapshot {
	return domain.DashboardSnapshot{
		Repo:      repo,
		PRs:       prs,
		FetchedAt: time.Now(),
	}
}

func splitRepo(full string) (string, string, bool) {
	for i := 0; i < len(full); i++ {
		if full[i] == '/' {
			return full[:i], full[i+1:], true
		}
	}
	return "", full, false
}
