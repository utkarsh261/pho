package github

import (
	"context"

	"github.com/utkarsh261/pho/internal/domain"
)

// GitHubClient is the transport interface for all GitHub API operations.
// Implementations live in internal/github/graphql. Mocks live in internal/testutil/mocks.
type GitHubClient interface {
	FetchViewer(ctx context.Context, host string) (string, error)
	FetchDashboardPRs(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error)
	FetchInvolvingPRs(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error)
	FetchRecentActivity(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error)
	FetchPreview(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error)
	PostComment(ctx context.Context, host, pullRequestID, body string) error
}
