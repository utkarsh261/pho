package model

type ViewerResponse struct {
	Data ViewerData `json:"data"`
}

type ViewerData struct {
	Viewer ViewerNode `json:"viewer"`
}

type DashboardResponse struct {
	Data DashboardData `json:"data"`
}

type DashboardData struct {
	Repository RepositoryNode `json:"repository"`
}

type InvolvingResponse struct {
	Data InvolvingData `json:"data"`
}

type InvolvingData struct {
	Search SearchConnection `json:"search"`
}

type PreviewResponse struct {
	Data PreviewData `json:"data"`
}

type PreviewData struct {
	Repository RepositoryNode `json:"repository"`
}

type AddCommentData struct {
	AddComment struct {
		Subject struct {
			ID string `json:"id"`
		} `json:"subject"`
	} `json:"addComment"`
}

type AddPullRequestReviewData struct {
	AddPullRequestReview struct {
		PullRequestReview struct {
			ID string `json:"id"`
		} `json:"pullRequestReview"`
	} `json:"addPullRequestReview"`
}

type AddPullRequestReviewThreadReplyData struct {
	AddPullRequestReviewThreadReply struct {
		Comment struct {
			ID string `json:"id"`
		} `json:"comment"`
	} `json:"addPullRequestReviewThreadReply"`
}

type MergePullRequestData struct {
	MergePullRequest struct {
		PullRequest struct {
			ID    string
			State string
		}
	}
}

type PullRequestStateData struct {
	ClosePullRequest struct {
		PullRequest struct {
			ID    string
			State string
		}
	} `json:"closePullRequest"`
	ReopenPullRequest struct {
		PullRequest struct {
			ID    string
			State string
		}
	} `json:"reopenPullRequest"`
}

type UpdatePullRequestData struct {
	UpdatePullRequest struct {
		PullRequest struct {
			ID string `json:"id"`
		} `json:"pullRequest"`
	} `json:"updatePullRequest"`
}

type CheckMergeableData struct {
	Repository struct {
		PullRequest struct {
			Mergeable        string `json:"mergeable"`
			MergeStateStatus string `json:"mergeStateStatus"`
			HeadRefOid       string `json:"headRefOid"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

type ViewerNode struct {
	Login string `json:"login"`
}

type RepositoryNode struct {
	NameWithOwner string                `json:"nameWithOwner"`
	PullRequests  PullRequestConnection `json:"pullRequests"`
	PullRequest   *PullRequestNode      `json:"pullRequest,omitempty"`
}

type SearchConnection struct {
	IssueCount int               `json:"issueCount"`
	PageInfo   PageInfo          `json:"pageInfo"`
	Nodes      []PullRequestNode `json:"nodes"`
}

type PullRequestConnection struct {
	TotalCount int               `json:"totalCount"`
	PageInfo   PageInfo          `json:"pageInfo"`
	Nodes      []PullRequestNode `json:"nodes"`
}

type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

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
	ReviewDecision           *string                     `json:"reviewDecision"`
	Mergeable                string                      `json:"mergeable,omitempty"`
	MergeState               string                      `json:"mergeStateStatus,omitempty"`
	Labels                   LabelConnection             `json:"labels"`
	Author                   *ActorNode                  `json:"author"`
	Assignees                AssigneeConnection          `json:"assignees"`
	ReviewRequests           ReviewRequestConnection     `json:"reviewRequests"`
	LatestOpinionatedReviews OpinionatedReviewConnection `json:"latestOpinionatedReviews"`
	StatusCheckRollup        *StatusCheckRollup          `json:"statusCheckRollup,omitempty"`
	Commits                  CommitConnection            `json:"commits"`
	Reviews                  ReviewConnection            `json:"reviews"`
	ReviewThreads            ReviewThreadConnection      `json:"reviewThreads"`
	Files                    FileConnection              `json:"files"`
	TimelineItems            TimelineItemConnection      `json:"timelineItems"`
	Repository               *RepositoryRef              `json:"repository,omitempty"`
}

type LabelConnection struct {
	Nodes []LabelNode `json:"nodes"`
}

type LabelNode struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type CountNode struct {
	TotalCount int `json:"totalCount"`
}

type IssueCommentNode struct {
	ID        string     `json:"id"`
	Author    *ActorNode `json:"author"`
	Body      string     `json:"body,omitempty"`
	CreatedAt string     `json:"createdAt,omitempty"`
}

type IssueCommentConnection struct {
	TotalCount int                `json:"totalCount"`
	Nodes      []IssueCommentNode `json:"nodes"`
}

type ActorNode struct {
	Typename  string `json:"__typename,omitempty"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl"`
}

type RepositoryRef struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type AssigneeConnection struct {
	Nodes []ActorNode `json:"nodes"`
}

type ReviewRequestConnection struct {
	Nodes []ReviewRequestNode `json:"nodes"`
}

type ReviewRequestNode struct {
	RequestedReviewer RequestedReviewer `json:"requestedReviewer"`
}

type RequestedReviewer struct {
	Typename     string           `json:"__typename"`
	Login        string           `json:"login,omitempty"`
	Slug         string           `json:"slug,omitempty"`
	Organization *OrganizationRef `json:"organization,omitempty"`
}

type OrganizationRef struct {
	Login string `json:"login"`
}

type OpinionatedReviewConnection struct {
	Nodes []OpinionatedReviewNode `json:"nodes"`
}

type OpinionatedReviewNode struct {
	State       string     `json:"state"`
	SubmittedAt *string    `json:"submittedAt"`
	Author      *ActorNode `json:"author"`
	Commit      *CommitRef `json:"commit"`
}

type CommitRef struct {
	OID string `json:"oid"`
}

type CommitConnection struct {
	Nodes []CommitNode `json:"nodes"`
}

type CommitNode struct {
	Commit CommitData `json:"commit"`
}

type CommitData struct {
	OID               string             `json:"oid,omitempty"`
	MessageHeadline   string             `json:"messageHeadline,omitempty"`
	MessageBody       string             `json:"messageBody,omitempty"`
	CommittedDate     string             `json:"committedDate,omitempty"`
	Author            *CommitAuthor      `json:"author,omitempty"`
	StatusCheckRollup *StatusCheckRollup `json:"statusCheckRollup,omitempty"`
}

type CommitAuthor struct {
	Name  string     `json:"name"`
	Email string     `json:"email"`
	User  *ActorNode `json:"user,omitempty"`
}

type StatusCheckRollup struct {
	State    string                  `json:"state"`
	Contexts StatusContextConnection `json:"contexts"`
}

type StatusContextConnection struct {
	Nodes []StatusContextNode `json:"nodes"`
}

type StatusContextNode struct {
	Typename   string `json:"__typename,omitempty"`
	Name       string `json:"name,omitempty"`
	Context    string `json:"context,omitempty"`
	Status     string `json:"status,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	State      string `json:"state,omitempty"`
	DetailsURL string `json:"detailsUrl,omitempty"`
	TargetUrl  string `json:"targetUrl,omitempty"`
}

type ReviewConnection struct {
	Nodes []ReviewNode `json:"nodes"`
}

type ReviewNode struct {
	Author      *ActorNode              `json:"author"`
	State       string                  `json:"state"`
	SubmittedAt *string                 `json:"submittedAt"`
	Body        string                  `json:"body,omitempty"`
	Comments    ReviewCommentConnection `json:"comments"`
}

type ReviewCommentConnection struct {
	Nodes []ReviewCommentNode `json:"nodes"`
}

type ReviewCommentNode struct {
	Author       *ActorNode `json:"author"`
	Body         string     `json:"body"`
	CreatedAt    string     `json:"createdAt,omitempty"`
	Path         string     `json:"path"`
	Line         *int       `json:"line,omitempty"`
	OriginalLine *int       `json:"originalLine,omitempty"`
}

type FileConnection struct {
	Nodes []FileNode `json:"nodes"`
}

type FileNode struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type ReviewThreadConnection struct {
	TotalCount int                `json:"totalCount"`
	Nodes      []ReviewThreadNode `json:"nodes"`
}

type ReviewThreadNode struct {
	ID         string                        `json:"id"`
	Path       string                        `json:"path"`
	Line       int                           `json:"line"`
	IsResolved bool                          `json:"isResolved"`
	Comments   ReviewThreadCommentConnection `json:"comments"`
}

type ReviewThreadCommentConnection struct {
	Nodes []ReviewThreadCommentNode `json:"nodes"`
}

type ReviewThreadCommentNode struct {
	ID        string     `json:"id"`
	Author    *ActorNode `json:"author"`
	Body      string     `json:"body"`
	CreatedAt string     `json:"createdAt"`
}

type TimelineItemConnection struct {
	Nodes []TimelineItemNode `json:"nodes"`
}

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

type TimelineAuthor struct {
	User *ActorNode `json:"user"`
	Name string     `json:"name,omitempty"` // git author name, populated when User is nil
}

type TimelineCommit struct {
	OID             string          `json:"oid"`
	MessageHeadline string          `json:"messageHeadline,omitempty"`
	CommittedDate   string          `json:"committedDate,omitempty"`
	Author          *TimelineAuthor `json:"author,omitempty"`
}

type CommitsResponse struct {
	Data CommitsData `json:"data"`
}

type CommitsData struct {
	Repository RepositoryNode `json:"repository"`
}

type CommitDetailResponse struct {
	Data CommitDetailData `json:"data"`
}

type CommitDetailData struct {
	Repository struct {
		Object CommitNode `json:"object"`
	} `json:"repository"`
}
