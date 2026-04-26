package cmds

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
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
	LoadAllPRsPage(ctx context.Context, repo domain.Repository, cursor string) ([]domain.PullRequestSummary, bool, string, error)
}

type SearchService interface {
	BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error
	BuildRepoIndex(repos []domain.Repository) error
}

type PRService interface {
	LoadDetail(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error)
	LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error)
	PostComment(ctx context.Context, prID string, body string) error
	PostReviewComment(ctx context.Context, prID string, body string) error
	ApprovePR(ctx context.Context, prID string, body string) error
	SubmitReviewWithComments(ctx context.Context, prID, body, event string, comments []domain.DraftInlineComment) error
	SaveDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string, drafts []domain.DraftInlineComment) error
	LoadDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) ([]domain.DraftInlineComment, error)
	DeleteDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) error
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
	Host  string
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

// AllPRsPageLoaded is emitted when a background all-PRs page fetch completes.
type AllPRsPageLoaded struct {
	Repo       string
	Entries    []domain.PullRequestSummary
	HasMore    bool
	NextCursor string
	PagesLeft  int
	Err        error
}

// CommentPosted is emitted when a PR comment has been successfully posted.
type CommentPosted struct{}

// CommentFailed is emitted when posting a PR comment fails.
type CommentFailed struct{ Err error }

// ApprovalPosted is emitted when a PR review approval has been successfully submitted.
type ApprovalPosted struct{}

// ApprovalFailed is emitted when submitting a PR review approval fails.
type ApprovalFailed struct{ Err error }

// ReviewPosted is emitted when a PR review with inline comments has been successfully submitted.
type ReviewPosted struct{}

// ReviewFailed is emitted when submitting a PR review with inline comments fails.
type ReviewFailed struct{ Err error }

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
		return ViewerResolved{Host: host, Login: login, Err: err}
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

func LoadPreviewCmd(svc DashboardService, repo string, number int, host string) tea.Cmd {
	return func() tea.Msg {
		repoArg := repo
		if host != "" {
			repoArg = host + "/" + repo
		}
		snap, err := svc.LoadPreview(context.Background(), repoArg, number)
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

// FetchAllPRsPageCmd fires a background all-PRs page fetch for the jump index.
func FetchAllPRsPageCmd(svc DashboardService, repo domain.Repository, cursor string, pagesLeft int) tea.Cmd {
	return func() tea.Msg {
		entries, hasMore, nextCursor, err := svc.LoadAllPRsPage(context.Background(), repo, cursor)
		return AllPRsPageLoaded{
			Repo:       repoKey(repo),
			Entries:    entries,
			HasMore:    hasMore,
			NextCursor: nextCursor,
			PagesLeft:  pagesLeft,
			Err:        err,
		}
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

func PostReviewCommentCmd(svc PRService, prID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.PostReviewComment(context.Background(), prID, body); err != nil {
			return CommentFailed{Err: err}
		}
		return CommentPosted{}
	}
}

func PostCommentCmd(svc PRService, prID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.PostComment(context.Background(), prID, body); err != nil {
			return CommentFailed{Err: err}
		}
		return CommentPosted{}
	}
}

func ApprovePRCmd(svc PRService, prID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.ApprovePR(context.Background(), prID, body); err != nil {
			return ApprovalFailed{Err: err}
		}
		return ApprovalPosted{}
	}
}

func SubmitReviewWithDraftsCmd(svc PRService, prID, body, event string, drafts []domain.DraftInlineComment) tea.Cmd {
	return func() tea.Msg {
		if err := svc.SubmitReviewWithComments(context.Background(), prID, body, event, drafts); err != nil {
			return ReviewFailed{Err: err}
		}
		return ReviewPosted{}
	}
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
