package model

// ViewerResponse is the raw GraphQL payload for viewer identity resolution.
type ViewerResponse struct {
	Data ViewerData `json:"data"`
}

// ViewerData is the inner data object for viewer identity resolution.
type ViewerData struct {
	Viewer ViewerNode `json:"viewer"`
}

// DashboardResponse is the raw GraphQL payload for the repo dashboard query.
type DashboardResponse struct {
	Data DashboardData `json:"data"`
}

// DashboardData is the inner data object for dashboard PR queries.
type DashboardData struct {
	Repository RepositoryNode `json:"repository"`
}

// InvolvingResponse is the raw GraphQL payload for the involving search query.
type InvolvingResponse struct {
	Data InvolvingData `json:"data"`
}

// InvolvingData is the inner data object for involving search queries.
type InvolvingData struct {
	Search SearchConnection `json:"search"`
}

// RecentResponse is the raw GraphQL payload for the recent activity query.
type RecentResponse struct {
	Data RecentData `json:"data"`
}

// RecentData is the inner data object for recent activity queries.
type RecentData struct {
	Repository RepositoryNode `json:"repository"`
}

// PreviewResponse is the raw GraphQL payload for the preview query.
type PreviewResponse struct {
	Data PreviewData `json:"data"`
}

// PreviewData is the inner data object for preview queries.
type PreviewData struct {
	Repository RepositoryNode `json:"repository"`
}

// AddCommentData is the data object returned by the addComment mutation.
type AddCommentData struct {
	AddComment struct {
		Subject struct {
			ID string `json:"id"`
		} `json:"subject"`
	} `json:"addComment"`
}

// ViewerNode identifies the authenticated viewer.
type ViewerNode struct {
	Login string `json:"login"`
}

// RepositoryNode is shared by repo-scoped query responses.
type RepositoryNode struct {
	NameWithOwner string                `json:"nameWithOwner"`
	PullRequests  PullRequestConnection `json:"pullRequests"`
	PullRequest   *PullRequestNode      `json:"pullRequest,omitempty"`
}

// SearchConnection is the root search connection for involving PRs.
type SearchConnection struct {
	IssueCount int               `json:"issueCount"`
	PageInfo   PageInfo          `json:"pageInfo"`
	Nodes      []PullRequestNode `json:"nodes"`
}

// PullRequestConnection is the standard pull request connection.
type PullRequestConnection struct {
	TotalCount int               `json:"totalCount"`
	PageInfo   PageInfo          `json:"pageInfo"`
	Nodes      []PullRequestNode `json:"nodes"`
}

// PageInfo is the standard GraphQL page info block.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// PullRequestNode is a transport DTO that spans summary, preview, and activity queries.
type PullRequestNode struct {
	ID                       string                      `json:"id"`
	Number                   int                         `json:"number"`
	Title                    string                      `json:"title"`
	Body                     string                      `json:"body,omitempty"`
	State                    string                      `json:"state"`
	IsDraft                  bool                        `json:"isDraft"`
	CreatedAt                string                      `json:"createdAt"`
	UpdatedAt                string                      `json:"updatedAt"`
	HeadRefName              string                      `json:"headRefName"`
	HeadRefOid               string                      `json:"headRefOid,omitempty"`
	BaseRefName              string                      `json:"baseRefName"`
	Additions                int                         `json:"additions"`
	Deletions                int                         `json:"deletions"`
	ChangedFiles             int                         `json:"changedFiles"`
	Comments                 IssueCommentConnection      `json:"comments"`
	ReviewThreads            CountNode                   `json:"reviewThreads"`
	ReviewDecision           *string                     `json:"reviewDecision"`
	Mergeable                string                      `json:"mergeable,omitempty"`
	MergeState               string                      `json:"mergeState,omitempty"`
	Labels                   LabelConnection             `json:"labels"`
	Author                   *ActorNode                  `json:"author"`
	Assignees                AssigneeConnection          `json:"assignees"`
	ReviewRequests           ReviewRequestConnection     `json:"reviewRequests"`
	LatestOpinionatedReviews OpinionatedReviewConnection `json:"latestOpinionatedReviews"`
	StatusCheckRollup        *StatusCheckRollup          `json:"statusCheckRollup,omitempty"`
	Commits                  CommitConnection            `json:"commits"`
	Reviews                  ReviewConnection            `json:"reviews"`
	Files                    FileConnection              `json:"files"`
	TimelineItems            TimelineItemConnection      `json:"timelineItems"`
	Repository               *RepositoryRef              `json:"repository,omitempty"`
}

// LabelConnection carries PR labels.
type LabelConnection struct {
	Nodes []LabelNode `json:"nodes"`
}

// LabelNode represents a single GitHub label.
type LabelNode struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CountNode represents a totalCount-only GraphQL object.
type CountNode struct {
	TotalCount int `json:"totalCount"`
}

// IssueCommentNode captures a single PR comment (IssueComment).
type IssueCommentNode struct {
	Author    *ActorNode `json:"author"`
	Body      string     `json:"body,omitempty"`
	CreatedAt string     `json:"createdAt,omitempty"`
}

// IssueCommentConnection carries PR comment nodes with a total count.
type IssueCommentConnection struct {
	TotalCount int                `json:"totalCount"`
	Nodes      []IssueCommentNode `json:"nodes"`
}

// ActorNode captures a login-bearing GraphQL actor.
type ActorNode struct {
	Typename  string `json:"__typename,omitempty"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl"`
}

// RepositoryRef references the parent repo from nested search responses.
type RepositoryRef struct {
	NameWithOwner string `json:"nameWithOwner"`
}

// AssigneeConnection carries PR assignee logins.
type AssigneeConnection struct {
	Nodes []ActorNode `json:"nodes"`
}

// ReviewRequestConnection carries direct review request recipients.
type ReviewRequestConnection struct {
	Nodes []ReviewRequestNode `json:"nodes"`
}

// ReviewRequestNode wraps the requested reviewer union.
type ReviewRequestNode struct {
	RequestedReviewer RequestedReviewer `json:"requestedReviewer"`
}

// RequestedReviewer represents the User/Team union for requested reviewers.
type RequestedReviewer struct {
	Typename     string           `json:"__typename"`
	Login        string           `json:"login,omitempty"`
	Slug         string           `json:"slug,omitempty"`
	Organization *OrganizationRef `json:"organization,omitempty"`
}

// OrganizationRef is used when a requested reviewer is a team.
type OrganizationRef struct {
	Login string `json:"login"`
}

// OpinionatedReviewConnection carries the latest opinionated review per reviewer.
type OpinionatedReviewConnection struct {
	Nodes []OpinionatedReviewNode `json:"nodes"`
}

// OpinionatedReviewNode is one latest review record.
type OpinionatedReviewNode struct {
	State       string     `json:"state"`
	SubmittedAt *string    `json:"submittedAt"`
	Author      *ActorNode `json:"author"`
	Commit      *CommitRef `json:"commit"`
}

// CommitRef is used by review summaries to point at the reviewed SHA.
type CommitRef struct {
	OID string `json:"oid"`
}

// CommitConnection carries nested commit status rollups.
type CommitConnection struct {
	Nodes []CommitNode `json:"nodes"`
}

// CommitNode wraps a commit object.
type CommitNode struct {
	Commit CommitData `json:"commit"`
}

// CommitData is the commit object embedded in summary and preview queries.
type CommitData struct {
	OID               string             `json:"oid,omitempty"`
	StatusCheckRollup *StatusCheckRollup `json:"statusCheckRollup,omitempty"`
}

// StatusCheckRollup captures coarse and detailed CI state.
type StatusCheckRollup struct {
	State    string                  `json:"state"`
	Contexts StatusContextConnection `json:"contexts"`
}

// StatusContextConnection carries the detailed CI rows.
type StatusContextConnection struct {
	Nodes []StatusContextNode `json:"nodes"`
}

// StatusContextNode captures check run/status context rows.
type StatusContextNode struct {
	Typename   string `json:"__typename,omitempty"`
	Name       string `json:"name,omitempty"`
	Context    string `json:"context,omitempty"`
	Status     string `json:"status,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	State      string `json:"state,omitempty"`
	DetailsURL string `json:"detailsUrl,omitempty"`
}

// ReviewConnection carries preview review rows.
type ReviewConnection struct {
	Nodes []ReviewNode `json:"nodes"`
}

// ReviewNode captures an individual review for preview rendering.
type ReviewNode struct {
	Author      *ActorNode `json:"author"`
	State       string     `json:"state"`
	SubmittedAt *string    `json:"submittedAt"`
	Body        string     `json:"body,omitempty"`
}

// FileConnection carries file stats for preview rendering.
type FileConnection struct {
	Nodes []FileNode `json:"nodes"`
}

// FileNode describes one changed file.
type FileNode struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// TimelineItemConnection carries recent activity rows.
type TimelineItemConnection struct {
	Nodes []TimelineItemNode `json:"nodes"`
}

// TimelineItemNode is a polymorphic activity row.
type TimelineItemNode struct {
	Typename     string          `json:"__typename"`
	ID           string          `json:"id"`
	Body         string          `json:"body,omitempty"`
	CreatedAt    string          `json:"createdAt,omitempty"`
	SubmittedAt  *string         `json:"submittedAt,omitempty"`
	State        string          `json:"state,omitempty"`
	MergeRefName string          `json:"mergeRefName,omitempty"`
	Actor        *ActorNode      `json:"actor,omitempty"`
	Author       *ActorNode      `json:"author,omitempty"`
	Commit       *TimelineCommit `json:"commit,omitempty"`
}

// TimelineAuthor matches the commit author shape used in timeline items.
type TimelineAuthor struct {
	User *ActorNode `json:"user"`
	Name string     `json:"name,omitempty"` // git author name, populated when User is nil
}

// TimelineCommit captures commit metadata in timeline items.
type TimelineCommit struct {
	OID             string          `json:"oid"`
	MessageHeadline string          `json:"messageHeadline,omitempty"`
	CommittedDate   string          `json:"committedDate,omitempty"`
	Author          *TimelineAuthor `json:"author,omitempty"`
}
