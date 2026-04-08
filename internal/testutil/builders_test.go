package testutil

import (
	"testing"
	"time"

	"github.com/utk/git-term/internal/domain"
)

func TestRepo_Defaults(t *testing.T) {
	r := Repo("alice/myrepo")
	if r.Owner != "alice" {
		t.Errorf("expected Owner=alice, got %q", r.Owner)
	}
	if r.Name != "myrepo" {
		t.Errorf("expected Name=myrepo, got %q", r.Name)
	}
	if r.FullName != "alice/myrepo" {
		t.Errorf("expected FullName=alice/myrepo, got %q", r.FullName)
	}
	if r.Host != "github.com" {
		t.Errorf("expected Host=github.com, got %q", r.Host)
	}
	if r.LocalPath != "/tmp/test/repo" {
		t.Errorf("expected LocalPath=/tmp/test/repo, got %q", r.LocalPath)
	}
}

func TestRepo_WithLocalPath(t *testing.T) {
	r := Repo("alice/myrepo", WithLocalPath("/custom/path"))
	if r.LocalPath != "/custom/path" {
		t.Errorf("expected LocalPath=/custom/path, got %q", r.LocalPath)
	}
}

func TestRepo_Pinned(t *testing.T) {
	r := Repo("alice/myrepo", Pinned())
	if !r.Pinned {
		t.Error("expected Pinned=true")
	}
}

func TestPR_Defaults(t *testing.T) {
	pr := PR(42)
	if pr.Number != 42 {
		t.Errorf("expected Number=42, got %d", pr.Number)
	}
	if pr.State != domain.PRStateOpen {
		t.Errorf("expected State=OPEN, got %q", pr.State)
	}
	if pr.Author != "testuser" {
		t.Errorf("expected Author=testuser, got %q", pr.Author)
	}
	if pr.Title != "Fix bug #42" {
		t.Errorf("expected Title='Fix bug #42', got %q", pr.Title)
	}
	if pr.HeadRefName != "fix/bug-42" {
		t.Errorf("expected HeadRefName=fix/bug-42, got %q", pr.HeadRefName)
	}
	if pr.BaseRefName != "main" {
		t.Errorf("expected BaseRefName=main, got %q", pr.BaseRefName)
	}
	if pr.CIStatus != domain.CIStatusSuccess {
		t.Errorf("expected CIStatus=SUCCESS, got %q", pr.CIStatus)
	}
	// CreatedAt should be roughly one week ago
	oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)
	diff := pr.CreatedAt.Sub(oneWeekAgo)
	if diff > time.Minute || diff < -time.Minute {
		t.Errorf("expected CreatedAt ~1 week ago, diff=%v", diff)
	}
	// UpdatedAt should be roughly one hour ago
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	diff = pr.UpdatedAt.Sub(oneHourAgo)
	if diff > time.Minute || diff < -time.Minute {
		t.Errorf("expected UpdatedAt ~1 hour ago, diff=%v", diff)
	}
}

func TestPR_WithAuthor(t *testing.T) {
	pr := PR(1, WithAuthor("carol"))
	if pr.Author != "carol" {
		t.Errorf("expected Author=carol, got %q", pr.Author)
	}
}

func TestPR_WithState(t *testing.T) {
	pr := PR(1, WithState(domain.PRStateMerged))
	if pr.State != domain.PRStateMerged {
		t.Errorf("expected State=MERGED, got %q", pr.State)
	}
}

func TestPR_WithDraft(t *testing.T) {
	pr := PR(1, WithDraft(true))
	if !pr.IsDraft {
		t.Error("expected IsDraft=true")
	}
}

func TestPR_WithCIStatus(t *testing.T) {
	pr := PR(1, WithCIStatus(domain.CIStatusFailure))
	if pr.CIStatus != domain.CIStatusFailure {
		t.Errorf("expected CIStatus=FAILURE, got %q", pr.CIStatus)
	}
}

func TestPR_WithReviewDecision(t *testing.T) {
	pr := PR(1, WithReviewDecision(domain.ReviewDecisionApproved))
	if pr.ReviewDecision != domain.ReviewDecisionApproved {
		t.Errorf("expected ReviewDecision=APPROVED, got %q", pr.ReviewDecision)
	}
}

func TestPR_WithRequestedReviewers(t *testing.T) {
	pr := PR(1, WithRequestedReviewers("alice", "bob"))
	if len(pr.RequestedReviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d", len(pr.RequestedReviewers))
	}
	if pr.RequestedReviewers[0] != "alice" || pr.RequestedReviewers[1] != "bob" {
		t.Errorf("unexpected reviewers: %v", pr.RequestedReviewers)
	}
}

func TestPR_WithLatestReview(t *testing.T) {
	at := time.Now()
	pr := PR(1, WithLatestReview("carol", "APPROVED", at))
	if len(pr.LatestReviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(pr.LatestReviews))
	}
	rv := pr.LatestReviews[0]
	if rv.AuthorLogin != "carol" {
		t.Errorf("expected AuthorLogin=carol, got %q", rv.AuthorLogin)
	}
	if rv.State != "APPROVED" {
		t.Errorf("expected State=APPROVED, got %q", rv.State)
	}
}

func TestPR_WithHeadOID(t *testing.T) {
	pr := PR(1, WithHeadOID("deadbeef"))
	if pr.HeadRefOID != "deadbeef" {
		t.Errorf("expected HeadRefOID=deadbeef, got %q", pr.HeadRefOID)
	}
}

func TestDashboardSnap(t *testing.T) {
	repo := Repo("alice/myrepo")
	pr1 := PR(1)
	pr2 := PR(2)
	before := time.Now()
	snap := DashboardSnap(repo, pr1, pr2)
	after := time.Now()

	if len(snap.PRs) != 2 {
		t.Errorf("expected 2 PRs, got %d", len(snap.PRs))
	}
	if snap.Repo.FullName != "alice/myrepo" {
		t.Errorf("expected Repo.FullName=alice/myrepo, got %q", snap.Repo.FullName)
	}
	if snap.FetchedAt.Before(before) || snap.FetchedAt.After(after) {
		t.Error("FetchedAt should be set to approximately now")
	}
	if snap.Truncated {
		t.Error("expected Truncated=false by default")
	}
}

func TestSeededState(t *testing.T) {
	state := SeededState()
	if len(state.Repos.Discovered) != 3 {
		t.Errorf("expected 3 discovered repos, got %d", len(state.Repos.Discovered))
	}
	if state.Repos.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", state.Repos.SelectedIndex)
	}
	if state.Repos.SelectedRepo == nil {
		t.Fatal("expected SelectedRepo to be set")
	}
	if state.Dashboard.ActiveTab != domain.TabMyPRs {
		t.Errorf("expected ActiveTab=TabMyPRs, got %q", state.Dashboard.ActiveTab)
	}
	if state.Dashboard.PRsByTab == nil {
		t.Error("expected PRsByTab to be initialized")
	}
}

func TestSeededState_WithOption(t *testing.T) {
	state := SeededState(func(s *domain.AppState) {
		s.Dashboard.ActiveTab = domain.TabNeedsReview
	})
	if state.Dashboard.ActiveTab != domain.TabNeedsReview {
		t.Errorf("expected ActiveTab=TabNeedsReview after option, got %q", state.Dashboard.ActiveTab)
	}
}
