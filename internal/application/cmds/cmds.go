package cmds

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

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
	LoadPreview(ctx context.Context, repo string, number int, force bool) (domain.PRPreviewSnapshot, error)
	LoadAllPRsPage(ctx context.Context, repo domain.Repository, cursor string) ([]domain.PullRequestSummary, bool, string, error)
	InvalidateRepo(ctx context.Context, repo domain.Repository) error
}

type SearchService interface {
	BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error
	BuildRepoIndex(repos []domain.Repository) error
}

type PRService interface {
	LoadDetail(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error)
	LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error)
	LoadPRCommits(ctx context.Context, repo domain.Repository, number int, force bool) ([]domain.Commit, error)
	LoadCommitDiff(ctx context.Context, repo domain.Repository, sha string, force bool) (model.DiffModel, error)
	PostComment(ctx context.Context, repo domain.Repository, prID string, body string) error
	PostCommentReply(ctx context.Context, repo domain.Repository, prID, commentID, body string) error
	PostReviewComment(ctx context.Context, repo domain.Repository, prID string, body string) error
	PostThreadReply(ctx context.Context, repo domain.Repository, threadID, body string) error
	ApprovePR(ctx context.Context, repo domain.Repository, prID string, body string) error
	SubmitReviewWithComments(ctx context.Context, repo domain.Repository, prID, body, event string, comments []domain.DraftInlineComment) error
	SaveDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string, drafts []domain.DraftInlineComment) error
	LoadDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) ([]domain.DraftInlineComment, error)
	DeleteDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) error
	MergePR(ctx context.Context, repo domain.Repository, number int, prID string, headRefOID string, method string) error
	CheckMergeable(ctx context.Context, repo domain.Repository, number int) (domain.MergeableState, error)
	ClosePR(ctx context.Context, repo domain.Repository, number int, prID string) error
	ReopenPR(ctx context.Context, repo domain.Repository, number int, prID string) error
	UpdatePR(ctx context.Context, repo domain.Repository, number int, prID string, title string, body string) error
	CreatePR(ctx context.Context, params domain.CreatePRParams) (domain.PullRequestSummary, error)
	FetchRepoInfo(ctx context.Context, repo domain.Repository) (domain.RepoInfo, error)
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

type CommitsLoaded struct {
	Repo    string
	Number  int
	Commits []domain.Commit
	Err     error
}

type CommitDiffLoaded struct {
	Repo string
	SHA  string
	Diff model.DiffModel
	Err  error
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

type CommentPosted struct{}

type CommentFailed struct{ Err error }

type ApprovalPosted struct{}

type ApprovalFailed struct{ Err error }

type ReviewPosted struct{}

type ReviewFailed struct{ Err error }

type MergeableChecked struct {
	Repo   string
	Number int
	State  domain.MergeableState
	Err    error
}

type MergePRMsg struct {
	Repo   string
	Number int
	Method string
	Err    error
}

type PRStateChangedMsg struct {
	Repo     string
	Number   int
	NewState domain.PRState
	Err      error
}

type PRUpdated struct {
	Repo   string
	Number int
	Title  string
	Body   string
	Err    error
}

// PRCreated is emitted when a new PR is successfully created.
type PRCreated struct {
	Repo    string
	Number  int
	Summary domain.PullRequestSummary
	Err     error
}

// CreatePRFormData carries the preflight information for the create-PR form.
type CreatePRFormData struct {
	Repo           domain.Repository
	CurrentBranch  string
	DefaultBase    string
	LastCommitMsg  string
	IsPushed       bool
	IsFork         bool
	ParentFullName string // empty if not a fork
	LocalBranches  []string
	RemoteBranches []string
	Err            error
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

func LoadPreviewCmd(svc DashboardService, repo string, number int, host string, force bool) tea.Cmd {
	return func() tea.Msg {
		repoArg := repo
		if host != "" {
			repoArg = host + "/" + repo
		}
		snap, err := svc.LoadPreview(context.Background(), repoArg, number, force)
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

func PostReviewCommentCmd(svc PRService, repo domain.Repository, prID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.PostReviewComment(context.Background(), repo, prID, body); err != nil {
			return CommentFailed{Err: err}
		}
		return CommentPosted{}
	}
}

func PostCommentCmd(svc PRService, repo domain.Repository, prID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.PostComment(context.Background(), repo, prID, body); err != nil {
			return CommentFailed{Err: err}
		}
		return CommentPosted{}
	}
}

func PostCommentReplyCmd(svc PRService, repo domain.Repository, prID, commentID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.PostCommentReply(context.Background(), repo, prID, commentID, body); err != nil {
			return CommentFailed{Err: err}
		}
		return CommentPosted{}
	}
}

func PostThreadReplyCmd(svc PRService, repo domain.Repository, threadID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.PostThreadReply(context.Background(), repo, threadID, body); err != nil {
			return CommentFailed{Err: err}
		}
		return CommentPosted{}
	}
}

func ApprovePRCmd(svc PRService, repo domain.Repository, prID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.ApprovePR(context.Background(), repo, prID, body); err != nil {
			return ApprovalFailed{Err: err}
		}
		return ApprovalPosted{}
	}
}

func SubmitReviewWithDraftsCmd(svc PRService, repo domain.Repository, prID, body, event string, drafts []domain.DraftInlineComment) tea.Cmd {
	return func() tea.Msg {
		if err := svc.SubmitReviewWithComments(context.Background(), repo, prID, body, event, drafts); err != nil {
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

func LoadPRCommitsCmd(svc PRService, repo domain.Repository, number int, force bool) tea.Cmd {
	return func() tea.Msg {
		commits, err := svc.LoadPRCommits(context.Background(), repo, number, force)
		return CommitsLoaded{
			Repo:    repoKey(repo),
			Number:  number,
			Commits: commits,
			Err:     err,
		}
	}
}

func LoadCommitDiffCmd(svc PRService, repo domain.Repository, sha string, force bool) tea.Cmd {
	return func() tea.Msg {
		diff, err := svc.LoadCommitDiff(context.Background(), repo, sha, force)
		return CommitDiffLoaded{
			Repo: repoKey(repo),
			SHA:  sha,
			Diff: diff,
			Err:  err,
		}
	}
}

func CheckMergeableCmd(svc PRService, repo domain.Repository, number int) tea.Cmd {
	return func() tea.Msg {
		state, err := svc.CheckMergeable(context.Background(), repo, number)
		return MergeableChecked{
			Repo:   repoKey(repo),
			Number: number,
			State:  state,
			Err:    err,
		}
	}
}

func MergePRCmd(svc PRService, repo domain.Repository, number int, prID string, headRefOID string, method string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.MergePR(context.Background(), repo, number, prID, headRefOID, method); err != nil {
			return MergePRMsg{Repo: repoKey(repo), Number: number, Method: method, Err: err}
		}
		return MergePRMsg{Repo: repoKey(repo), Number: number, Method: method}
	}
}

func ClosePRCmd(svc PRService, repo domain.Repository, number int, prID string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.ClosePR(context.Background(), repo, number, prID); err != nil {
			return PRStateChangedMsg{Repo: repoKey(repo), Number: number, NewState: domain.PRStateClosed, Err: err}
		}
		return PRStateChangedMsg{Repo: repoKey(repo), Number: number, NewState: domain.PRStateClosed}
	}
}

func ReopenPRCmd(svc PRService, repo domain.Repository, number int, prID string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.ReopenPR(context.Background(), repo, number, prID); err != nil {
			return PRStateChangedMsg{Repo: repoKey(repo), Number: number, NewState: domain.PRStateOpen, Err: err}
		}
		return PRStateChangedMsg{Repo: repoKey(repo), Number: number, NewState: domain.PRStateOpen}
	}
}

// CheckoutResult is emitted when the local git checkout command completes.
type CheckoutResult struct {
	Branch  string
	Stashed bool
	Err     error
}

// CheckoutBranchCmd runs git fetch + checkout for a PR branch asynchronously.
// If the working tree is dirty, changes are stashed before checkout.
// For cross-repository PRs it fetches refs/pull/<number>/head and creates a
// local branch named pr-<number> (with suffixes if necessary).
func CheckoutBranchCmd(repo domain.Repository, prNumber int, branch string, isCrossRepo bool) tea.Cmd {
	return func() tea.Msg {
		if repo.LocalPath == "" {
			return CheckoutResult{Err: errors.New("not a local repo")}
		}
		if _, err := execGit(repo.LocalPath, "rev-parse", "--git-dir"); err != nil {
			return CheckoutResult{Err: fmt.Errorf("not a git repo: %w", err)}
		}

		stashed := false
		if out, err := execGit(repo.LocalPath, "status", "--porcelain"); err == nil && out != "" {
			if _, err := execGit(repo.LocalPath, "stash", "push", "--include-untracked", "-m", fmt.Sprintf("pho: checkout PR #%d", prNumber)); err != nil {
				return CheckoutResult{Err: fmt.Errorf("stash failed: %w", err)}
			}
			stashed = true
		}

		name, args, localBranch, err := checkoutCommand(repo.LocalPath, prNumber, branch, isCrossRepo)
		if err != nil {
			if stashed {
				_, _ = execGit(repo.LocalPath, "stash", "pop")
			}
			return CheckoutResult{Err: err}
		}
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			if !isCrossRepo && strings.Contains(string(out), "couldn't find remote ref") {
				name, args, localBranch, err = checkoutCommand(repo.LocalPath, prNumber, branch, true)
				if err != nil {
					if stashed {
						_, _ = execGit(repo.LocalPath, "stash", "pop")
					}
					return CheckoutResult{Err: err}
				}
				out, err = exec.Command(name, args...).CombinedOutput()
			}
			if err != nil {
				if stashed {
					_, _ = execGit(repo.LocalPath, "stash", "pop")
				}
				return CheckoutResult{Err: parseGitError(string(out))}
			}
		}
		return CheckoutResult{Branch: localBranch, Stashed: stashed}
	}
}

// checkoutCommand builds the git command for checking out a PR branch.
// Returns the command name, args, the local branch name that will be checked out, and any error.
func checkoutCommand(localPath string, prNumber int, branch string, isCrossRepo bool) (string, []string, string, error) {
	if isCrossRepo {
		localBranch, err := findUnusedBranch(localPath, prNumber)
		if err != nil {
			return "", nil, "", err
		}
		script := fmt.Sprintf("git -C %s fetch origin refs/pull/%d/head && git -C %s checkout -b %s FETCH_HEAD",
			shellQuote(localPath), prNumber, shellQuote(localPath), shellQuote(localBranch))
		return "sh", []string{"-c", script}, localBranch, nil
	}
	script := fmt.Sprintf("git -C %s fetch origin %s && git -C %s checkout %s",
		shellQuote(localPath), shellQuote(branch), shellQuote(localPath), shellQuote(branch))
	return "sh", []string{"-c", script}, branch, nil
}

func findUnusedBranch(localPath string, prNumber int) (string, error) {
	base := fmt.Sprintf("pr-%d", prNumber)
	for _, suffix := range []string{"", "-1", "-2", "-3", "-4", "-5", "-6", "-7", "-8", "-9", "-10"} {
		name := base + suffix
		if _, err := execGit(localPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+name); err != nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("local branches %s through %s-10 already exist", base, base)
}

func UpdatePRCmd(svc PRService, repo domain.Repository, number int, prID string, title string, body string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.UpdatePR(context.Background(), repo, number, prID, title, body); err != nil {
			return PRUpdated{Repo: repoKey(repo), Number: number, Title: title, Body: body, Err: err}
		}
		return PRUpdated{Repo: repoKey(repo), Number: number, Title: title, Body: body}
	}
}

// CreatePRCmd fires the REST API call to create a pull request.
func CreatePRCmd(svc PRService, params domain.CreatePRParams) tea.Cmd {
	return func() tea.Msg {
		summary, err := svc.CreatePR(context.Background(), params)
		return PRCreated{
			Repo:    repoKey(params.Repo),
			Number:  summary.Number,
			Summary: summary,
			Err:     err,
		}
	}
}

// LoadCreatePRFormDataCmd gathers local git state and remote repo metadata
// needed to pre-populate the create-PR form.
func LoadCreatePRFormDataCmd(repo domain.Repository, svc PRService) tea.Cmd {
	return func() tea.Msg {
		data := CreatePRFormData{Repo: repo}

		if repo.LocalPath == "" {
			data.Err = errors.New("no local path for repo")
			return data
		}

		// Current branch
		branch, err := execGit(repo.LocalPath, "branch", "--show-current")
		if err != nil {
			data.Err = fmt.Errorf("git branch: %w", err)
			return data
		}
		data.CurrentBranch = branch

		// Default base branch
		if base, err := execGit(repo.LocalPath, "rev-parse", "--abbrev-ref", "origin/HEAD"); err == nil && strings.HasPrefix(base, "origin/") {
			data.DefaultBase = strings.TrimPrefix(base, "origin/")
		}
		if data.DefaultBase == "" {
			data.DefaultBase = "main"
		}

		// Last commit message
		if msg, err := execGit(repo.LocalPath, "log", "-1", "--pretty=%B"); err == nil {
			data.LastCommitMsg = strings.TrimSpace(msg)
		}

		// Is branch pushed?
		if _, err := execGit(repo.LocalPath, "rev-parse", "--verify", "--quiet", branch+"@{u}"); err == nil {
			data.IsPushed = true
		}

		// Local branches (sorted by most recent commit).
		if out, err := execGit(repo.LocalPath, "branch", "--sort=-committerdate", "--format=%(refname:short)"); err == nil {
			for _, b := range strings.Split(out, "\n") {
				b = strings.TrimSpace(b)
				if b != "" {
					data.LocalBranches = append(data.LocalBranches, b)
				}
			}
		}

		// Remote branches (sorted by most recent commit).
		if out, err := execGit(repo.LocalPath, "branch", "-r", "--sort=-committerdate", "--format=%(refname:short)"); err == nil {
			for _, b := range strings.Split(out, "\n") {
				b = strings.TrimSpace(b)
				if b != "" && !strings.Contains(b, "HEAD ->") {
					data.RemoteBranches = append(data.RemoteBranches, b)
				}
			}
		}

		// Remote repo metadata
		info, err := svc.FetchRepoInfo(context.Background(), repo)
		if err == nil {
			data.IsFork = info.Fork
			if info.ParentFullName != "" {
				data.ParentFullName = info.ParentFullName
			}
			if info.DefaultBranch != "" {
				data.DefaultBase = info.DefaultBranch
			}
		}

		return data
	}
}

func execGit(localPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", localPath}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func parseGitError(stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "couldn't find remote ref"):
		return errors.New("branch not found on origin")
	case strings.Contains(s, "your local changes") || strings.Contains(s, "would be overwritten"):
		return errors.New("working tree dirty — stash or commit first")
	case strings.Contains(s, "authentication failed") || strings.Contains(s, "could not resolve host"):
		return errors.New("network/auth error")
	}
	msg := strings.TrimSpace(stderr)
	if len(msg) > 100 {
		r := []rune(msg)
		if len(r) > 100 {
			msg = string(r[:100]) + "…"
		}
	}
	return errors.New("git error: " + msg)
}

func shellQuote(s string) string {
	if strings.ContainsAny(s, "' \t\n\r&|;<>()$`\"*?[]#~=") {
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}
	return s
}
