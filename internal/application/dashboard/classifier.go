package dashboard

import (
	"strings"

	"github.com/utk/git-term/internal/domain"
)

// SummaryTabClassifier splits dashboard PR summaries into the local tabs.
//
// Phase 1 only classifies my_prs and needs_review locally. involving and recent
// are hydrated by separate snapshots.
type SummaryTabClassifier interface {
	Classify(viewer string, prs []domain.PullRequestSummary) map[domain.DashboardTab][]domain.PullRequestSummary
}

// DefaultSummaryTabClassifier is the Phase 1 classifier implementation.
type DefaultSummaryTabClassifier struct{}

// Classify applies the Phase 1 my_prs / needs_review rules.
func (DefaultSummaryTabClassifier) Classify(viewer string, prs []domain.PullRequestSummary) map[domain.DashboardTab][]domain.PullRequestSummary {
	out := map[domain.DashboardTab][]domain.PullRequestSummary{
		domain.TabMyPRs:       {},
		domain.TabNeedsReview: {},
	}
	for _, pr := range prs {
		if matchesLogin(viewer, pr.Author) {
			out[domain.TabMyPRs] = append(out[domain.TabMyPRs], pr)
		}
		if shouldNeedReview(viewer, pr) {
			out[domain.TabNeedsReview] = append(out[domain.TabNeedsReview], pr)
		}
	}
	return out
}

func shouldNeedReview(viewer string, pr domain.PullRequestSummary) bool {
	if pr.State != domain.PRStateOpen {
		return false
	}

	requested := hasLogin(pr.RequestedReviewers, viewer)
	assigned := hasLogin(pr.AssigneeLogins, viewer)
	latest := latestViewerReview(pr.LatestReviews, viewer)

	if requested || assigned {
		return true
	}

	if latest == nil {
		return false
	}

	switch latest.State {
	case string(domain.ReviewDecisionApproved):
		return !approvalStillFresh(*latest, pr)
	case string(domain.ReviewDecisionChangesRequested):
		return true
	default:
		return false
	}
}

func approvalStillFresh(review domain.ReviewSummary, pr domain.PullRequestSummary) bool {
	if review.CommitSHA != "" && pr.HeadRefOID != "" {
		return strings.EqualFold(review.CommitSHA, pr.HeadRefOID)
	}
	if review.SubmittedAt.IsZero() || pr.UpdatedAt.IsZero() {
		return false
	}
	return !pr.UpdatedAt.After(review.SubmittedAt)
}

func latestViewerReview(reviews []domain.ReviewSummary, viewer string) *domain.ReviewSummary {
	for i := len(reviews) - 1; i >= 0; i-- {
		if matchesLogin(viewer, reviews[i].AuthorLogin) {
			return &reviews[i]
		}
	}
	return nil
}

func hasLogin(logins []string, viewer string) bool {
	for _, login := range logins {
		if matchesLogin(viewer, login) {
			return true
		}
	}
	return false
}

func matchesLogin(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b)
}
