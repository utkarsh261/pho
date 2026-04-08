package testutil

import (
	"strings"
	"time"

	"github.com/utk/git-term/internal/domain"
)

// Repo returns a Repository with sensible defaults. Override with options.
func Repo(name string, opts ...func(*domain.Repository)) domain.Repository {
	repoName := name
	owner := ""
	if o, n, ok := strings.Cut(name, "/"); ok {
		owner, repoName = o, n
	}

	r := domain.Repository{
		Host:      "github.com",
		Owner:     owner,
		Name:      repoName,
		FullName:  name,
		LocalPath: "/tmp/test/repo",
	}
	for _, o := range opts {
		o(&r)
	}
	return r
}

// PR returns a PullRequestSummary with sensible defaults. Override with options.
func PR(number int, opts ...func(*domain.PullRequestSummary)) domain.PullRequestSummary {
	now := time.Now()
	pr := domain.PullRequestSummary{
		Repo:        "myorg/myrepo",
		Number:      number,
		Title:       "Fix bug #" + itoa(number),
		Author:      "testuser",
		State:       domain.PRStateOpen,
		HeadRefName: "fix/bug-" + itoa(number),
		BaseRefName: "main",
		CreatedAt:   now.Add(-7 * 24 * time.Hour),
		UpdatedAt:   now.Add(-1 * time.Hour),
		CIStatus:    domain.CIStatusSuccess,
	}
	for _, o := range opts {
		o(&pr)
	}
	return pr
}

// DashboardSnap returns a DashboardSnapshot from provided PRs.
func DashboardSnap(repo domain.Repository, prs ...domain.PullRequestSummary) domain.DashboardSnapshot {
	return domain.DashboardSnapshot{
		Repo:      repo,
		PRs:       prs,
		FetchedAt: time.Now(),
		Truncated: false,
	}
}

// InvolvingSnap returns an InvolvingSnapshot from provided PRs.
func InvolvingSnap(repo domain.Repository, prs ...domain.PullRequestSummary) domain.InvolvingSnapshot {
	return domain.InvolvingSnapshot{
		Repo:      repo,
		PRs:       prs,
		FetchedAt: time.Now(),
		Truncated: false,
	}
}

// RecentSnap returns a RecentSnapshot from provided activity items.
func RecentSnap(repo domain.Repository, items ...domain.ActivityItem) domain.RecentSnapshot {
	return domain.RecentSnapshot{
		Repo:      repo,
		Items:     items,
		FetchedAt: time.Now(),
	}
}

// SeededState returns an AppState with discovered repos and a selected repo.
func SeededState(opts ...func(*domain.AppState)) domain.AppState {
	repos := []domain.Repository{
		Repo("myorg/repo-one"),
		Repo("myorg/repo-two"),
		Repo("myorg/repo-three"),
	}
	selected := repos[0]

	state := domain.AppState{
		Repos: domain.RepoState{
			Discovered:    repos,
			SelectedIndex: 0,
			SelectedRepo:  &selected,
		},
		Dashboard: domain.DashboardState{
			ActiveTab: domain.TabMyPRs,
			PRsByTab:  map[domain.DashboardTab][]domain.PullRequestSummary{},
		},
	}
	for _, o := range opts {
		o(&state)
	}
	return state
}

// ---- Functional option helpers for PullRequestSummary ----

func WithAuthor(login string) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.Author = login }
}

func WithState(s domain.PRState) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.State = s }
}

func WithDraft(v bool) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.IsDraft = v }
}

func WithCIStatus(s domain.CIStatus) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.CIStatus = s }
}

func WithReviewDecision(rd domain.ReviewDecision) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.ReviewDecision = rd }
}

func WithRequestedReviewers(logins ...string) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.RequestedReviewers = logins }
}

func WithLatestReview(login, state string, at time.Time) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) {
		pr.LatestReviews = append(pr.LatestReviews, domain.ReviewSummary{
			AuthorLogin: login,
			State:       state,
			SubmittedAt: at,
		})
	}
}

func WithHeadOID(oid string) func(*domain.PullRequestSummary) {
	return func(pr *domain.PullRequestSummary) { pr.HeadRefOID = oid }
}

// ---- Functional option helpers for Repository ----

func Pinned() func(*domain.Repository) {
	return func(r *domain.Repository) { r.Pinned = true }
}

func WithLocalPath(p string) func(*domain.Repository) {
	return func(r *domain.Repository) { r.LocalPath = p }
}

// ---- helpers ----

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
