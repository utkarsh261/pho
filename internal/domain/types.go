// Package domain defines the canonical data model for pho.
// Types in this package are shared across the application, cache, and UI
// layers. No GitHub transport details belong here.
package domain

import "time"

// Enums

type PRState string

const (
	PRStateOpen   PRState = "OPEN"
	PRStateClosed PRState = "CLOSED"
	PRStateMerged PRState = "MERGED"
)

type ReviewDecision string

const (
	ReviewDecisionApproved         ReviewDecision = "APPROVED"
	ReviewDecisionChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	ReviewDecisionReviewRequired   ReviewDecision = "REVIEW_REQUIRED"
	ReviewDecisionNone             ReviewDecision = ""
)

type CIStatus string

const (
	CIStatusPending CIStatus = "PENDING"
	CIStatusSuccess CIStatus = "SUCCESS"
	CIStatusFailure CIStatus = "FAILURE"
	CIStatusError   CIStatus = "ERROR"
	CIStatusNone    CIStatus = ""
)

type DashboardTab string

const (
	TabMyPRs       DashboardTab = "my_prs"
	TabNeedsReview DashboardTab = "needs_review"
	TabInvolving   DashboardTab = "involving"
	TabRecent      DashboardTab = "recent"
)

type Freshness string

const (
	FreshnessFresh      Freshness = "fresh"
	FreshnessStale      Freshness = "stale"
	FreshnessRefreshing Freshness = "refreshing"
	FreshnessErrorStale Freshness = "error-stale"
)

type SearchMode string

const (
	SearchModePRs   SearchMode = "prs"
	SearchModeRepos SearchMode = "repos"
)

type ActivityKind string

const (
	ActivityKindCommit          ActivityKind = "commit"
	ActivityKindComment         ActivityKind = "comment"
	ActivityKindReview          ActivityKind = "review"
	ActivityKindReviewRequested ActivityKind = "review_requested"
	ActivityKindMerged          ActivityKind = "merged"
	ActivityKindClosed          ActivityKind = "closed"
	ActivityKindReopened        ActivityKind = "reopened"
	ActivityKindLabeled         ActivityKind = "labeled"
)

// Core domain entities

type Repository struct {
	ID             string
	Host           string
	Owner          string
	Name           string
	FullName       string // owner/name
	LocalPath      string
	RemoteURL      string
	LastScannedAt  time.Time
	LastActivityAt *time.Time
	Pinned         bool
	Excluded       bool
}

type ActivitySnippet struct {
	Kind      ActivityKind
	Author    string
	Body      string
	OccuredAt time.Time
}

type PullRequestSummary struct {
	ID                string
	Repo              string
	Number            int
	Title             string
	Author            string
	State             PRState
	IsDraft           bool
	ReviewDecision    ReviewDecision
	CIStatus          CIStatus
	UpdatedAt         time.Time
	CreatedAt         time.Time
	HeadRefName       string
	HeadRefOID        string
	BaseRefName       string
	CommentCount      int
	ReviewThreadCount int
	UnresolvedCount   int
	Additions         int
	Deletions         int
	FileCount         int
	LatestActivity    *ActivitySnippet

	// Fields used for tab classification.
	RequestedReviewers []string // logins of directly requested reviewer users
	AssigneeLogins     []string
	LatestReviews      []ReviewSummary // one per reviewer, latest opinionated only
}

type ReviewSummary struct {
	AuthorLogin string
	State       string
	SubmittedAt time.Time
	CommitSHA   string
}

type PRListRow struct {
	Number            int
	Title             string
	Branch            string
	State             PRState
	CIStatus          CIStatus
	NeedsViewerReview bool
	UpdatedAt         time.Time
}

type PreviewReviewer struct {
	Login       string
	State       string
	Avatar      string
	Body        string    // review comment body (may be empty for approval-only reviews)
	SubmittedAt time.Time // zero if not provided
}

type PreviewComment struct {
	Login     string
	Body      string
	CreatedAt time.Time
}

type PreviewCheckRow struct {
	Name    string
	State   string
	Context string
}

type PreviewFileStat struct {
	Path      string
	Additions int
	Deletions int
}

type PRPreviewSnapshot struct {
	Repo           string
	Number         int
	Title          string
	BodyExcerpt    string
	Author         string
	State          PRState
	IsDraft        bool
	CIStatus       CIStatus
	ReviewDecision ReviewDecision
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Reviewers      []PreviewReviewer
	Checks         []PreviewCheckRow
	FileCount      int
	Additions      int
	Deletions      int
	TopFiles       []PreviewFileStat
	LatestActivity *ActivitySnippet
	Labels         []Label
	Assignees      []string
	Mergeable      string
	MergeState     string
	Comments       []PreviewComment
}

type ActivityItem struct {
	ID          string
	Repo        string
	PRNumber    int
	Kind        ActivityKind
	Author      string
	BodySnippet string
	CommitOID   string
	OccurredAt  time.Time
}

// Snapshot envelopes

type DiscoverySnapshot struct {
	Repos     []Repository
	FetchedAt time.Time
}

type DashboardSnapshot struct {
	Repo       Repository
	PRs        []PullRequestSummary
	TotalCount int
	Truncated  bool
	EndCursor  string
	FetchedAt  time.Time
}

type InvolvingSnapshot struct {
	Repo       Repository
	PRs        []PullRequestSummary
	TotalCount int
	Truncated  bool
	FetchedAt  time.Time
}

type RecentSnapshot struct {
	Repo      Repository
	Items     []ActivityItem
	FetchedAt time.Time
}

type PreviewSnapshot struct {
	Repo      string
	Number    int
	Preview   PRPreviewSnapshot
	FetchedAt time.Time
}

// Cache

type CacheMeta struct {
	Key          string
	Kind         string
	Version      int
	Host         string
	Repo         string
	PRNumber     *int
	FetchedAt    time.Time
	ExpiresAt    time.Time
	ETag         string
	LastModified string
	SizeBytes    int
	Encoding     string
}

// App state

type SessionState struct {
	ViewerByHost map[string]string
	ActiveHost   string
	Offline      bool
	StartedAt    time.Time
}

type RepoState struct {
	Discovered    []Repository
	SelectedIndex int
	SelectedRepo  *Repository
	CursorIndex   int
	OrderFrozen   bool
	Focused       bool
	LastScanAt    *time.Time
}

type DashboardState struct {
	ActiveTab      DashboardTab
	PRsByTab       map[DashboardTab][]PullRequestSummary
	RecentItems    []ActivityItem
	SelectedIndex  int
	Preview        *PRPreviewSnapshot
	PreviewLoading bool
	ListFocused    bool
	LastRefreshAt  map[DashboardTab]time.Time
	FreshnessByTab map[DashboardTab]Freshness
	Truncated      bool
	TotalCount     int
}

type SearchResult struct {
	Kind    SearchResultKind
	Repo    string
	Number  int
	Title   string
	Branch  string
	Author  string
	Score   float64
	State   PRState
	IsDraft bool
}

type SearchResultKind string

const (
	SearchResultPR   SearchResultKind = "pr"
	SearchResultRepo SearchResultKind = "repo"
)

type SearchState struct {
	OverlayOpen   bool
	Mode          SearchMode
	Query         string
	Results       []SearchResult
	SelectedIndex int
	PartialPRs    bool
}

type JobState struct {
	InFlight map[string]bool
}

type AppError struct {
	Kind    ErrorKind
	Message string
	Repo    string
}

type ErrorKind string

const (
	ErrorKindAuth      ErrorKind = "auth"
	ErrorKindNetwork   ErrorKind = "network"
	ErrorKindRateLimit ErrorKind = "rate_limit"
	ErrorKindParse     ErrorKind = "parse"
	ErrorKindCache     ErrorKind = "cache"
	ErrorKindDiscovery ErrorKind = "discovery"
)

type ErrorState struct {
	Errors         []AppError
	RateLimitReset *time.Time
}

type ConfigState struct {
	Loaded bool
}

type AppState struct {
	Session   SessionState
	Config    ConfigState
	Repos     RepoState
	Dashboard DashboardState
	Search    SearchState
	Jobs      JobState
	Errors    ErrorState
}

// View stack

type PrimaryView string

const (
	PrimaryViewDashboard PrimaryView = "dashboard"
	PrimaryViewPRDetail  PrimaryView = "pr_detail"
)

type OverlayView string

const (
	OverlayViewCmdPalette OverlayView = "cmd_palette"
)

type FocusTarget string

const (
	FocusRepoPanel    FocusTarget = "repo_panel"
	FocusPRListPanel  FocusTarget = "pr_list_panel"
	FocusPreviewPanel FocusTarget = "preview_panel"
	FocusCmdPalette   FocusTarget = "cmd_palette"
)

// PR Detail domain types

// PRChecks represents the CI/checks status for a PR.
type PRChecks struct {
	State    string // SUCCESS, FAILURE, PENDING, ERROR
	Contexts []CheckContext
}

// CheckContext represents a single CI check.
type CheckContext struct {
	Name       string
	State      string // COMPLETED, IN_PROGRESS, QUEUED, WAITING
	Conclusion string // SUCCESS, FAILURE, NEUTRAL, SKIPPED
}

// Label is a GitHub label.
type Label struct {
	Name  string
	Color string
}

// User is a minimal GitHub user.
type User struct {
	Login string
}

// Reviewer is a review participant with their review state.
type Reviewer struct {
	Login string
	State string // APPROVED, CHANGES_REQUESTED, COMMENTED, etc.
}

// ReviewRequest represents a pending review request.
type ReviewRequest struct {
	Login  string
	IsTeam bool
}

// PRDetailSection enumerates the three sections in the content viewport.
type PRDetailSection int

const (
	SectionDescription PRDetailSection = iota
	SectionDiff
	SectionComments
)

// SectionState tracks the load status of a single content section.
// Reserved for future use when sections are rendered independently.
type SectionState struct {
	Loaded  bool
	Loading bool
	Error   error
}

// PRDetailState is the planned state container for the PR detail view.
// Currently unused — PRDetailModel manages its own parallel state.
// Kept as a type alias target for when the view layer is refactored.
type PRDetailState struct {
	Repo    string
	Number  int
	Summary PullRequestSummary // extended via FetchPreview (labels, assignees)
	Reviews []PreviewReviewer  // from FetchPreview reviews(first:20)
	Checks  []PreviewCheckRow  // from FetchPreview statusCheckRollup
	// Sections map[PRDetailSection]*SectionState — deferred until section-level
	// loading is implemented. PRDetailModel currently uses parallel bool fields.
}

// FocusPRDetail is the internal focus target for PR detail view.
// It is NOT added to the dashboard focus cycle.
const FocusPRDetail FocusTarget = "pr_detail"
