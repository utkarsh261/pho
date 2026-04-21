package mocks

import (
	"context"

	"github.com/utkarsh261/pho/internal/domain"
	githubclient "github.com/utkarsh261/pho/internal/github"
)

// Ensure MockGitHubClient implements github.GitHubClient at compile time.
var _ githubclient.GitHubClient = (*MockGitHubClient)(nil)

// MockGitHubClient implements github.GitHubClient for tests.
// Each function field must be set before calling the method.
// Calling an unset function panics with a descriptive message.
type MockGitHubClient struct {
	FetchViewerFn         func(ctx context.Context, host string) (string, error)
	FetchDashboardPRsFn   func(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error)
	FetchInvolvingPRsFn   func(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error)
	FetchRecentActivityFn func(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error)
	FetchPreviewFn        func(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error)
	PostCommentFn         func(ctx context.Context, host, pullRequestID, body string) error

	// Call counters — incremented on each call.
	FetchDashboardPRsCalls int
	FetchPreviewCalls      int
	PostCommentCalls       int
}

func (m *MockGitHubClient) FetchViewer(ctx context.Context, host string) (string, error) {
	if m.FetchViewerFn == nil {
		panic("MockGitHubClient.FetchViewer called but FetchViewerFn is nil")
	}
	return m.FetchViewerFn(ctx, host)
}

func (m *MockGitHubClient) FetchDashboardPRs(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
	if m.FetchDashboardPRsFn == nil {
		panic("MockGitHubClient.FetchDashboardPRs called but FetchDashboardPRsFn is nil")
	}
	m.FetchDashboardPRsCalls++
	return m.FetchDashboardPRsFn(ctx, repo)
}

func (m *MockGitHubClient) FetchInvolvingPRs(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error) {
	if m.FetchInvolvingPRsFn == nil {
		panic("MockGitHubClient.FetchInvolvingPRs called but FetchInvolvingPRsFn is nil")
	}
	return m.FetchInvolvingPRsFn(ctx, repo, viewer)
}

func (m *MockGitHubClient) FetchRecentActivity(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error) {
	if m.FetchRecentActivityFn == nil {
		panic("MockGitHubClient.FetchRecentActivity called but FetchRecentActivityFn is nil")
	}
	return m.FetchRecentActivityFn(ctx, repo)
}

func (m *MockGitHubClient) FetchPreview(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
	if m.FetchPreviewFn == nil {
		panic("MockGitHubClient.FetchPreview called but FetchPreviewFn is nil")
	}
	m.FetchPreviewCalls++
	return m.FetchPreviewFn(ctx, repo, number)
}

func (m *MockGitHubClient) PostComment(ctx context.Context, host, pullRequestID, body string) error {
	if m.PostCommentFn == nil {
		panic("MockGitHubClient.PostComment called but PostCommentFn is nil")
	}
	m.PostCommentCalls++
	return m.PostCommentFn(ctx, host, pullRequestID, body)
}
