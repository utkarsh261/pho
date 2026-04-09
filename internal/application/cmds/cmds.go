package cmds

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utk/git-term/internal/domain"
)

// ViewerService resolves the active viewer login for a host.
type ViewerService interface {
	FetchViewer(ctx context.Context, host string) (string, error)
}

// DiscoveryService discovers repositories from a filesystem root.
type DiscoveryService interface {
	Discover(ctx context.Context, root string) ([]domain.Repository, error)
}

// DashboardService loads repo-scoped dashboard, involving, recent, and preview snapshots.
type DashboardService interface {
	LoadRepo(ctx context.Context, repo domain.Repository, force bool) (domain.DashboardSnapshot, error)
	LoadInvolving(ctx context.Context, repo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error)
	LoadRecent(ctx context.Context, repo domain.Repository, force bool) (domain.RecentSnapshot, error)
	LoadPreview(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error)
}

// SearchService rebuilds in-memory search indexes.
type SearchService interface {
	BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error
	BuildRepoIndex(repos []domain.Repository) error
}

// ViewerResolved is emitted when viewer resolution completes.
type ViewerResolved struct {
	Login string
	Err   error
}

// ReposDiscovered is emitted when local repository discovery completes.
type ReposDiscovered struct {
	Repos []domain.Repository
	Err   error
}

// DashboardLoaded is emitted when a dashboard snapshot completes loading.
type DashboardLoaded struct {
	Repo      string
	Snapshot  domain.DashboardSnapshot
	FromCache bool
	Err       error
}

// InvolvingLoaded is emitted when involving PRs complete loading.
type InvolvingLoaded struct {
	Repo      string
	Snapshot  domain.InvolvingSnapshot
	FromCache bool
	Err       error
}

// RecentLoaded is emitted when recent activity completes loading.
type RecentLoaded struct {
	Repo      string
	Snapshot  domain.RecentSnapshot
	FromCache bool
	Err       error
}

// PreviewLoaded is emitted when a PR preview completes loading.
type PreviewLoaded struct {
	Repo      string
	Number    int
	Preview   domain.PRPreviewSnapshot
	FromCache bool
	Err       error
}

// SearchIndexRebuilt is emitted when a search index rebuild completes.
type SearchIndexRebuilt struct {
	Repo string
	Err  error
}

// RefreshStarted marks a job key as in-flight.
type RefreshStarted struct {
	Key string
}

// RefreshFinished clears the in-flight marker for a job key.
type RefreshFinished struct {
	Key string
}

// RefreshFailed records a job failure for a specific key.
type RefreshFailed struct {
	Key string
	Err error
}

// ResolveViewerCmd resolves the viewer login for the given host.
func ResolveViewerCmd(svc ViewerService, host string) tea.Cmd {
	return func() tea.Msg {
		login, err := svc.FetchViewer(context.Background(), host)
		return ViewerResolved{Login: login, Err: err}
	}
}

// DiscoverReposCmd discovers repositories from the provided root path.
func DiscoverReposCmd(svc DiscoveryService, root string) tea.Cmd {
	return func() tea.Msg {
		repos, err := svc.Discover(context.Background(), root)
		return ReposDiscovered{Repos: repos, Err: err}
	}
}

// LoadDashboardCmd loads the dashboard snapshot for a repository.
func LoadDashboardCmd(svc DashboardService, repo domain.Repository, force bool) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadRepo(context.Background(), repo, force)
		return DashboardLoaded{Repo: repoKey(repo), Snapshot: snap, Err: err}
	}
}

// LoadInvolvingCmd loads the involving snapshot for a repository.
func LoadInvolvingCmd(svc DashboardService, repo domain.Repository, viewer string, force bool) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadInvolving(context.Background(), repo, viewer, force)
		return InvolvingLoaded{Repo: repoKey(repo), Snapshot: snap, Err: err}
	}
}

// LoadRecentCmd loads the recent activity snapshot for a repository.
func LoadRecentCmd(svc DashboardService, repo domain.Repository, force bool) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadRecent(context.Background(), repo, force)
		return RecentLoaded{Repo: repoKey(repo), Snapshot: snap, Err: err}
	}
}

// LoadPreviewCmd loads the preview snapshot for a single PR.
func LoadPreviewCmd(svc DashboardService, repo string, number int) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadPreview(context.Background(), repo, number)
		return PreviewLoaded{Repo: repo, Number: number, Preview: snap, Err: err}
	}
}

// RebuildPRIndexCmd rebuilds the PR index for a single repository.
func RebuildPRIndexCmd(svc SearchService, repo domain.Repository, snap domain.DashboardSnapshot) tea.Cmd {
	return func() tea.Msg {
		err := svc.BuildPRIndex(repo, snap)
		return SearchIndexRebuilt{Repo: repoKey(repo), Err: err}
	}
}

// RebuildRepoIndexCmd rebuilds the repo search index.
func RebuildRepoIndexCmd(svc SearchService, repos []domain.Repository) tea.Cmd {
	return func() tea.Msg {
		err := svc.BuildRepoIndex(repos)
		return SearchIndexRebuilt{Err: err}
	}
}

func repoKey(repo domain.Repository) string {
	if repo.FullName != "" {
		return repo.FullName
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner + "/" + repo.Name
	}
	return repo.Name
}
