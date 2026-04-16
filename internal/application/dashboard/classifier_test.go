package dashboard

import (
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

func TestDefaultSummaryTabClassifier_Classify(t *testing.T) {
	t.Parallel()

	viewer := "alice"
	base := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	head := "head-sha"

	cases := []struct {
		name string
		pr   domain.PullRequestSummary
		want map[domain.DashboardTab]bool
	}{
		{
			name: "authored draft lands in my_prs only",
			pr: testutil.PR(1,
				testutil.WithAuthor(viewer),
				testutil.WithDraft(true),
			),
			want: map[domain.DashboardTab]bool{
				domain.TabMyPRs: true,
			},
		},
		{
			name: "requested review lands in needs_review",
			pr: testutil.PR(2,
				testutil.WithRequestedReviewers(viewer),
			),
			want: map[domain.DashboardTab]bool{
				domain.TabNeedsReview: true,
			},
		},
		{
			name: "assignee lands in needs_review",
			pr: testutil.PR(3,
				func(pr *domain.PullRequestSummary) { pr.AssigneeLogins = []string{viewer} },
			),
			want: map[domain.DashboardTab]bool{
				domain.TabNeedsReview: true,
			},
		},
		{
			name: "viewer approved and not requested stays out",
			pr: testutil.PR(4,
				testutil.WithLatestReview(viewer, string(domain.ReviewDecisionApproved), base.Add(-30*time.Minute)),
				testutil.WithHeadOID(head),
				// UpdatedAt must be before SubmittedAt so the time-based freshness check
				// treats the approval as still valid (CommitSHA is empty so SHA check is skipped).
				func(pr *domain.PullRequestSummary) { pr.UpdatedAt = base.Add(-2 * time.Hour) },
			),
			want: map[domain.DashboardTab]bool{},
		},
		{
			name: "viewer approved but re-requested stays in",
			pr: testutil.PR(5,
				testutil.WithRequestedReviewers(viewer),
				testutil.WithLatestReview(viewer, string(domain.ReviewDecisionApproved), base.Add(-30*time.Minute)),
				testutil.WithHeadOID(head),
			),
			want: map[domain.DashboardTab]bool{
				domain.TabNeedsReview: true,
			},
		},
		{
			name: "viewer approved but head moved on re-enters needs_review",
			pr: testutil.PR(6,
				func(pr *domain.PullRequestSummary) {
					pr.LatestReviews = []domain.ReviewSummary{{
						AuthorLogin: viewer,
						State:       string(domain.ReviewDecisionApproved),
						SubmittedAt: base.Add(-30 * time.Minute),
						CommitSHA:   "old-head",
					}}
					pr.HeadRefOID = "new-head"
				},
			),
			want: map[domain.DashboardTab]bool{
				domain.TabNeedsReview: true,
			},
		},
		{
			name: "viewer approved without commit sha but updated after approval re-enters needs_review",
			pr: testutil.PR(7,
				func(pr *domain.PullRequestSummary) {
					pr.LatestReviews = []domain.ReviewSummary{{
						AuthorLogin: viewer,
						State:       string(domain.ReviewDecisionApproved),
						SubmittedAt: base.Add(-2 * time.Hour),
					}}
					pr.HeadRefOID = ""
					pr.UpdatedAt = base.Add(-10 * time.Minute)
				},
			),
			want: map[domain.DashboardTab]bool{
				domain.TabNeedsReview: true,
			},
		},
		{
			name: "draft requested still lands in needs_review",
			pr: testutil.PR(8,
				testutil.WithDraft(true),
				testutil.WithRequestedReviewers(viewer),
			),
			want: map[domain.DashboardTab]bool{
				domain.TabNeedsReview: true,
			},
		},
		{
			name: "authored requested PR lands in both tabs",
			pr: testutil.PR(9,
				testutil.WithAuthor(viewer),
				testutil.WithRequestedReviewers(viewer),
			),
			want: map[domain.DashboardTab]bool{
				domain.TabMyPRs:       true,
				domain.TabNeedsReview: true,
			},
		},
	}

	classifier := DefaultSummaryTabClassifier{}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := classifier.Classify(viewer, []domain.PullRequestSummary{tc.pr})
			for tab, wantPresent := range tc.want {
				if !wantPresent {
					continue
				}
				if len(got[tab]) != 1 {
					t.Fatalf("tab %s: got %d entries, want 1", tab, len(got[tab]))
				}
				if got[tab][0].Number != tc.pr.Number {
					t.Fatalf("tab %s: got PR #%d, want #%d", tab, got[tab][0].Number, tc.pr.Number)
				}
			}
			for _, tab := range []domain.DashboardTab{domain.TabMyPRs, domain.TabNeedsReview} {
				if !tc.want[tab] && len(got[tab]) != 0 {
					t.Fatalf("tab %s: expected empty, got %d entries", tab, len(got[tab]))
				}
			}
		})
	}
}
