package graphql

import (
	"fmt"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/model"
)

func normalizeDashboardResponse(repo domain.Repository, resp model.DashboardData) ([]domain.PullRequestSummary, int, bool, string, error) {
	root := resp.Repository
	if root.NameWithOwner != "" && repo.FullName == "" {
		repo.FullName = root.NameWithOwner
	}
	total := root.PullRequests.TotalCount
	truncated := root.PullRequests.PageInfo.HasNextPage
	cursor := root.PullRequests.PageInfo.EndCursor
	out := make([]domain.PullRequestSummary, 0, len(root.PullRequests.Nodes))
	for _, node := range root.PullRequests.Nodes {
		summary, err := normalizePullRequestSummary(repo, node)
		if err != nil {
			return nil, 0, false, "", err
		}
		out = append(out, summary)
	}
	return out, total, truncated, cursor, nil
}

func normalizeInvolvingResponse(repo domain.Repository, resp model.InvolvingData) ([]domain.PullRequestSummary, int, bool, error) {
	root := resp.Search
	total := root.IssueCount
	truncated := root.PageInfo.HasNextPage
	out := make([]domain.PullRequestSummary, 0, len(root.Nodes))
	for _, node := range root.Nodes {
		summary, err := normalizePullRequestSummary(repo, node)
		if err != nil {
			return nil, 0, false, err
		}
		out = append(out, summary)
	}
	return out, total, truncated, nil
}

func normalizePreviewResponse(repo domain.Repository, number int, resp model.PreviewData) (domain.PRPreviewSnapshot, error) {
	root := resp.Repository
	if root.NameWithOwner != "" && repo.FullName == "" {
		repo.FullName = root.NameWithOwner
	}
	node := root.PullRequest
	if node == nil {
		return domain.PRPreviewSnapshot{}, fmt.Errorf("preview response missing pull request")
	}
	snapshot, err := normalizePreviewNode(repo, number, *node)
	if err != nil {
		return domain.PRPreviewSnapshot{}, err
	}
	return snapshot, nil
}

func normalizeAllPRsResponse(repo domain.Repository, resp model.DashboardData) ([]domain.PullRequestSummary, bool, string, error) {
	root := resp.Repository
	hasNext := root.PullRequests.PageInfo.HasNextPage
	cursor := root.PullRequests.PageInfo.EndCursor
	out := make([]domain.PullRequestSummary, 0, len(root.PullRequests.Nodes))
	for _, node := range root.PullRequests.Nodes {
		summary, err := normalizePullRequestSummary(repo, node)
		if err != nil {
			return nil, false, "", err
		}
		out = append(out, summary)
	}
	return out, hasNext, cursor, nil
}

func normalizePullRequestSummary(repo domain.Repository, node model.PullRequestNode) (domain.PullRequestSummary, error) {
	createdAt, err := parseGraphQLTime(node.CreatedAt)
	if err != nil {
		return domain.PullRequestSummary{}, err
	}
	updatedAt, err := parseGraphQLTime(node.UpdatedAt)
	if err != nil {
		return domain.PullRequestSummary{}, err
	}

	summary := domain.PullRequestSummary{
		ID:                node.ID,
		Repo:              repoIdentity(repo, node.Repository),
		Number:            node.Number,
		Title:             node.Title,
		Author:            actorLogin(node.Author),
		State:             parsePRState(node.State),
		IsDraft:           node.IsDraft,
		ReviewDecision:    parseReviewDecision(node.ReviewDecision),
		CIStatus:          parseCIStatus(rollupState(node)),
		UpdatedAt:         updatedAt,
		CreatedAt:         createdAt,
		HeadRefName:       node.HeadRefName,
		HeadRefOID:        node.HeadRefOid,
		BaseRefName:       node.BaseRefName,
		IsCrossRepository: repoIdentity(repo, node.Repository) != repo.FullName,
		CommentCount:      node.Comments.TotalCount,
		ReviewThreadCount: node.ReviewThreads.TotalCount,
		Additions:         node.Additions,
		Deletions:         node.Deletions,
		FileCount:         node.ChangedFiles,
	}

	summary.RequestedReviewers = requestedReviewers(node.ReviewRequests)
	summary.AssigneeLogins = assigneeLogins(node.Assignees)
	summary.LatestReviews = latestOpinionatedReviews(node.LatestOpinionatedReviews)

	return summary, nil
}

func normalizePreviewNode(repo domain.Repository, number int, node model.PullRequestNode) (domain.PRPreviewSnapshot, error) {
	createdAt, err := parseGraphQLTime(node.CreatedAt)
	if err != nil {
		return domain.PRPreviewSnapshot{}, err
	}
	updatedAt, err := parseGraphQLTime(node.UpdatedAt)
	if err != nil {
		return domain.PRPreviewSnapshot{}, err
	}

	snapshot := domain.PRPreviewSnapshot{
		ID:             node.ID,
		Repo:           repoIdentity(repo, node.Repository),
		Number:         numberOr(node.Number, number),
		Title:          node.Title,
		BodyExcerpt:    node.Body,
		Author:         actorLogin(node.Author),
		State:          parsePRState(node.State),
		IsDraft:        node.IsDraft,
		CIStatus:       parseCIStatus(rollupState(node)),
		ReviewDecision: parseReviewDecision(node.ReviewDecision),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		Reviewers:      previewReviewers(node.Reviews),
		ReviewThreads:  previewReviewThreads(node.ReviewThreads),
		Checks:         previewChecks(rollupContexts(node)),
		FileCount:      node.ChangedFiles,
		Additions:      node.Additions,
		Deletions:      node.Deletions,
		Labels:         previewLabels(node.Labels),
		Assignees:      assigneeLogins(node.Assignees),
		Mergeable:      node.Mergeable,
		MergeState:     node.MergeState,
		HeadRefOID:     node.HeadRefOid,
		LatestActivity: previewLatestActivity(repo, node.Number, node.TimelineItems.Nodes),
	}
	if len(node.Files.Nodes) > 0 {
		snapshot.TopFiles = make([]domain.PreviewFileStat, 0, len(node.Files.Nodes))
		for _, file := range node.Files.Nodes {
			snapshot.TopFiles = append(snapshot.TopFiles, domain.PreviewFileStat{
				Path:      file.Path,
				Additions: file.Additions,
				Deletions: file.Deletions,
			})
		}
	}
	snapshot.Comments = previewComments(node.Comments)
	return snapshot, nil
}

func rollupState(node model.PullRequestNode) string {
	if node.StatusCheckRollup != nil {
		return node.StatusCheckRollup.State
	}
	if len(node.Commits.Nodes) == 0 {
		return ""
	}
	if rollup := node.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
		return rollup.State
	}
	return ""
}

func rollupContexts(node model.PullRequestNode) []model.StatusContextNode {
	if node.StatusCheckRollup != nil {
		return node.StatusCheckRollup.Contexts.Nodes
	}
	if len(node.Commits.Nodes) == 0 {
		return nil
	}
	if rollup := node.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
		return rollup.Contexts.Nodes
	}
	return nil
}

func requestedReviewers(conn model.ReviewRequestConnection) []string {
	out := make([]string, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		if strings.EqualFold(node.RequestedReviewer.Typename, "User") && node.RequestedReviewer.Login != "" {
			out = append(out, node.RequestedReviewer.Login)
		}
	}
	return out
}

func assigneeLogins(conn model.AssigneeConnection) []string {
	out := make([]string, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		if node.Login != "" {
			out = append(out, node.Login)
		}
	}
	return out
}

func latestOpinionatedReviews(conn model.OpinionatedReviewConnection) []domain.ReviewSummary {
	out := make([]domain.ReviewSummary, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		review := domain.ReviewSummary{
			AuthorLogin: actorLogin(node.Author),
			State:       node.State,
			CommitSHA:   "",
		}
		if node.SubmittedAt != nil {
			if ts, err := parseGraphQLTime(*node.SubmittedAt); err == nil {
				review.SubmittedAt = ts
			}
		}
		if node.Commit != nil {
			review.CommitSHA = node.Commit.OID
		}
		out = append(out, review)
	}
	return out
}

func previewReviewers(conn model.ReviewConnection) []domain.PreviewReviewer {
	out := make([]domain.PreviewReviewer, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		login := actorLogin(node.Author)
		if login == "" {
			continue
		}
		avatar := ""
		if node.Author != nil {
			avatar = node.Author.AvatarURL
		}
		var submittedAt time.Time
		if node.SubmittedAt != nil {
			submittedAt, _ = time.Parse(time.RFC3339, *node.SubmittedAt)
		}
		out = append(out, domain.PreviewReviewer{
			Login:       login,
			State:       node.State,
			Avatar:      avatar,
			Body:        strings.TrimSpace(node.Body),
			SubmittedAt: submittedAt,
		})
	}
	return out
}

func previewReviewThreads(conn model.ReviewThreadConnection) []domain.PreviewReviewThread {
	if len(conn.Nodes) == 0 {
		return nil
	}
	out := make([]domain.PreviewReviewThread, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		if node.ID == "" {
			continue
		}
		line := 0
		if node.Line != nil {
			line = *node.Line
		} else if node.OriginalLine != nil {
			line = *node.OriginalLine
		}
		var comments []domain.PreviewThreadComment
		for _, c := range node.Comments.Nodes {
			login := actorLogin(c.Author)
			if login == "" {
				login = "ghost"
			}
			var createdAt time.Time
			if c.CreatedAt != "" {
				createdAt, _ = time.Parse(time.RFC3339, c.CreatedAt)
			}
			comments = append(comments, domain.PreviewThreadComment{
				ID:        c.ID,
				Login:     login,
				Body:      strings.TrimSpace(c.Body),
				CreatedAt: createdAt,
			})
		}
		if len(comments) == 0 {
			continue
		}
		out = append(out, domain.PreviewReviewThread{
			ID:         node.ID,
			Path:       node.Path,
			Line:       line,
			IsResolved: node.IsResolved,
			ResolvedBy: actorLogin(node.ResolvedBy),
			Comments:   comments,
		})
	}
	return out
}

func previewComments(conn model.IssueCommentConnection) []domain.PreviewComment {
	if len(conn.Nodes) == 0 {
		return nil
	}
	out := make([]domain.PreviewComment, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		login := actorLogin(node.Author)
		if login == "" {
			continue
		}
		var createdAt time.Time
		if node.CreatedAt != "" {
			createdAt, _ = time.Parse(time.RFC3339, node.CreatedAt)
		}
		out = append(out, domain.PreviewComment{
			ID:        node.ID,
			Login:     login,
			Body:      strings.TrimSpace(node.Body),
			CreatedAt: createdAt,
		})
	}
	return out
}

func previewChecks(nodes []model.StatusContextNode) []domain.PreviewCheckRow {
	out := make([]domain.PreviewCheckRow, 0, len(nodes))
	for _, node := range nodes {
		name := node.Name
		if name == "" {
			name = node.Context
		}
		state := node.Conclusion
		if state == "" {
			state = node.State
		}
		if state == "" {
			state = node.Status
		}
		context := node.Context
		if context == "" {
			context = node.DetailsURL
		}
		url := node.DetailsURL
		if url == "" {
			url = node.TargetUrl
		}
		out = append(out, domain.PreviewCheckRow{
			Name:    name,
			State:   state,
			Context: context,
			URL:     url,
		})
	}
	return out
}

func previewLabels(conn model.LabelConnection) []domain.Label {
	if len(conn.Nodes) == 0 {
		return nil
	}
	out := make([]domain.Label, 0, len(conn.Nodes))
	for _, node := range conn.Nodes {
		out = append(out, domain.Label{
			Name:  node.Name,
			Color: node.Color,
		})
	}
	return out
}

func previewLatestActivity(repo domain.Repository, prNumber int, nodes []model.TimelineItemNode) *domain.ActivitySnippet {
	if len(nodes) == 0 {
		return nil
	}
	n := nodes[0]
	var kind domain.ActivityKind
	var author, body string
	var ts time.Time

	switch n.Typename {
	case "PullRequestCommit":
		if n.Commit == nil {
			return nil
		}
		parsed, err := parseGraphQLTime(n.Commit.CommittedDate)
		if err != nil {
			return nil
		}
		kind = domain.ActivityKindCommit
		author = timelineCommitAuthor(n.Commit.Author)
		body = strings.TrimSpace(n.Commit.MessageHeadline)
		ts = parsed
	case "IssueComment":
		parsed, err := parseGraphQLTime(n.CreatedAt)
		if err != nil {
			return nil
		}
		kind = domain.ActivityKindComment
		author = actorLogin(n.Author)
		body = strings.TrimSpace(n.Body)
		ts = parsed
	case "PullRequestReview":
		kind = domain.ActivityKindReview
		author = actorLogin(n.Author)
		body = strings.TrimSpace(n.Body)
		if n.SubmittedAt != nil {
			parsed, err := parseGraphQLTime(*n.SubmittedAt)
			if err != nil {
				return nil
			}
			ts = parsed
		}
	case "MergedEvent":
		parsed, err := parseGraphQLTime(n.CreatedAt)
		if err != nil {
			return nil
		}
		kind = domain.ActivityKindMerged
		author = actorLogin(n.Actor)
		body = strings.TrimSpace(n.MergeRefName)
		ts = parsed
	default:
		return nil
	}

	return &domain.ActivitySnippet{
		Kind:      kind,
		Author:    author,
		Body:      body,
		OccuredAt: ts,
	}
}

func parseGraphQLTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err == nil {
		return ts, nil
	}
	ts, err = time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse GraphQL time %q: %w", raw, err)
	}
	return ts, nil
}

func parsePRState(raw string) domain.PRState {
	switch strings.ToUpper(raw) {
	case string(domain.PRStateOpen):
		return domain.PRStateOpen
	case string(domain.PRStateClosed):
		return domain.PRStateClosed
	case string(domain.PRStateMerged):
		return domain.PRStateMerged
	default:
		return domain.PRState(strings.ToUpper(raw))
	}
}

func parseReviewDecision(raw *string) domain.ReviewDecision {
	if raw == nil || *raw == "" {
		return domain.ReviewDecisionNone
	}
	return domain.ReviewDecision(strings.ToUpper(*raw))
}

func parseCIStatus(raw string) domain.CIStatus {
	switch strings.ToUpper(raw) {
	case string(domain.CIStatusPending):
		return domain.CIStatusPending
	case string(domain.CIStatusSuccess):
		return domain.CIStatusSuccess
	case string(domain.CIStatusFailure):
		return domain.CIStatusFailure
	case string(domain.CIStatusError):
		return domain.CIStatusError
	default:
		return domain.CIStatus(strings.ToUpper(raw))
	}
}

func actorLogin(node *model.ActorNode) string {
	if node == nil {
		return ""
	}
	return node.Login
}

func timelineCommitAuthor(author *model.TimelineAuthor) string {
	if author == nil {
		return ""
	}
	if author.User != nil {
		return author.User.Login
	}
	return author.Name
}

func repoIdentity(repo domain.Repository, ref *model.RepositoryRef) string {
	if ref != nil && ref.NameWithOwner != "" {
		return ref.NameWithOwner
	}
	if fn := repoFullName(repo); fn != "" {
		return fn
	}
	return ""
}

func numberOr(actual, fallback int) int {
	if actual != 0 {
		return actual
	}
	return fallback
}

func normalizeCommitsResponse(resp model.CommitsData) []domain.Commit {
	pr := resp.Repository.PullRequest
	if pr == nil {
		return nil
	}
	nodes := pr.Commits.Nodes
	// GraphQL returns oldest first, but we want newest first.
	// Reverse the slice in-place.
	out := make([]domain.Commit, 0, len(nodes))
	for i := len(nodes) - 1; i >= 0; i-- {
		c := nodes[i].Commit
		committedAt, _ := parseGraphQLTime(c.CommittedDate)
		login := ""
		name := ""
		if c.Author != nil {
			name = c.Author.Name
			if c.Author.User != nil {
				login = c.Author.User.Login
			}
		}
		out = append(out, domain.Commit{
			SHA:             c.OID,
			MessageHeadline: c.MessageHeadline,
			MessageBody:     c.MessageBody,
			AuthorName:      name,
			AuthorLogin:     login,
			CommittedAt:     committedAt,
		})
	}
	return out
}
