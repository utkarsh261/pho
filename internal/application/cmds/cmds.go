package cmds

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/domain"
)

type ViewerService interface {
	FetchViewer(ctx context.Context, host string) (string, error)
}

type DiscoveryService interface {
	Discover(ctx context.Context, root string) ([]domain.Repository, error)
}

type DashboardService interface {
	LoadRepo(ctx context.Context, repo domain.Repository, force bool) (domain.DashboardSnapshot, error)
	LoadInvolving(ctx context.Context, repo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error)
	LoadRecent(ctx context.Context, repo domain.Repository, force bool) (domain.RecentSnapshot, error)
	LoadPreview(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error)
}

type SearchService interface {
	BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error
	BuildRepoIndex(repos []domain.Repository) error
}

type PRService interface {
	LoadDetail(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error)
	LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error)
}

type PRDetailLoaded struct {
	Repo      string
	Number    int
	Detail    domain.PRPreviewSnapshot
	FromCache bool
	Err       error
}

type DiffLoaded struct {
	Repo      string
	Number    int
	Diff      model.DiffModel
	FromCache bool
	Err       error
}

type ViewerResolved struct {
	Login string
	Err   error
}

type ReposDiscovered struct {
	Repos []domain.Repository
	Err   error
}

type DashboardLoaded struct {
	Repo      string
	Snapshot  domain.DashboardSnapshot
	FromCache bool
	Err       error
}

type InvolvingLoaded struct {
	Repo      string
	Snapshot  domain.InvolvingSnapshot
	FromCache bool
	Err       error
}

type RecentLoaded struct {
	Repo      string
	Snapshot  domain.RecentSnapshot
	FromCache bool
	Err       error
}

type PreviewLoaded struct {
	Repo      string
	Number    int
	Preview   domain.PRPreviewSnapshot
	FromCache bool
	Err       error
}

type SearchIndexRebuilt struct {
	Repo string
	Err  error
}

type RefreshStarted struct {
	Key string
}

type RefreshFinished struct {
	Key string
}

type RefreshFailed struct {
	Key string
	Err error
}

func ResolveViewerCmd(svc ViewerService, host string) tea.Cmd {
	return func() tea.Msg {
		login, err := svc.FetchViewer(context.Background(), host)
		return ViewerResolved{Login: login, Err: err}
	}
}

func DiscoverReposCmd(svc DiscoveryService, root string) tea.Cmd {
	return func() tea.Msg {
		repos, err := svc.Discover(context.Background(), root)
		return ReposDiscovered{Repos: repos, Err: err}
	}
}

func LoadDashboardCmd(svc DashboardService, repo domain.Repository, force bool) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadRepo(context.Background(), repo, force)
		return DashboardLoaded{Repo: repoKey(repo), Snapshot: snap, Err: err}
	}
}

func LoadInvolvingCmd(svc DashboardService, repo domain.Repository, viewer string, force bool) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadInvolving(context.Background(), repo, viewer, force)
		return InvolvingLoaded{Repo: repoKey(repo), Snapshot: snap, Err: err}
	}
}

func LoadRecentCmd(svc DashboardService, repo domain.Repository, force bool) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadRecent(context.Background(), repo, force)
		return RecentLoaded{Repo: repoKey(repo), Snapshot: snap, Err: err}
	}
}

func LoadPreviewCmd(svc DashboardService, repo string, number int) tea.Cmd {
	return func() tea.Msg {
		snap, err := svc.LoadPreview(context.Background(), repo, number)
		return PreviewLoaded{Repo: repo, Number: number, Preview: snap, Err: err}
	}
}

func RebuildPRIndexCmd(svc SearchService, repo domain.Repository, snap domain.DashboardSnapshot) tea.Cmd {
	return func() tea.Msg {
		err := svc.BuildPRIndex(repo, snap)
		return SearchIndexRebuilt{Repo: repoKey(repo), Err: err}
	}
}

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

func LoadPRDetailCmd(svc PRService, repo domain.Repository, number int, force bool) tea.Cmd {
	return func() tea.Msg {
		detail, fromCache, err := svc.LoadDetail(context.Background(), repo, number, force)
		return PRDetailLoaded{
			Repo:      repoKey(repo),
			Number:    number,
			Detail:    detail,
			FromCache: fromCache,
			Err:       err,
		}
	}
}

func LoadDiffCmd(svc PRService, repo domain.Repository, number int, headSHA string, force bool) tea.Cmd {
	return func() tea.Msg {
		diff, fromCache, err := svc.LoadDiff(context.Background(), repo, number, headSHA, force)
		return DiffLoaded{
			Repo:      repoKey(repo),
			Number:    number,
			Diff:      diff,
			FromCache: fromCache,
			Err:       err,
		}
	}
}
