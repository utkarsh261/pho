package graphql

import (
	"fmt"
	"strings"

	"github.com/utk/git-term/internal/domain"
	githubpkg "github.com/utk/git-term/internal/github"
)

func buildViewerQuery() string {
	return `query ViewerQuery {
  viewer {
    login
  }
}`
}

func buildDashboardQuery(profile githubpkg.GitHubHostProfile) string {
	return fmt.Sprintf(`query DashboardPRsQuery($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    pullRequests(first: 100, states: OPEN, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        %s
      }
    }
  }
}`, pullRequestSummarySelection(profile))
}

func buildInvolvingQuery(profile githubpkg.GitHubHostProfile) string {
	return fmt.Sprintf(`query InvolvingPRsQuery($query: String!) {
  search(type: ISSUE, query: $query, first: 100) {
    issueCount
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      ... on PullRequest {
        %s
      }
    }
  }
}`, pullRequestSummarySelection(profile))
}

func buildRecentActivityQuery() string {
	return `query RecentActivityQuery($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    pullRequests(first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        number
        timelineItems(last: 50) {
          nodes {
            ... on PullRequestCommit {
              __typename
              id
              commit {
                oid
                messageHeadline
                committedDate
                author {
                  user {
                    login
                  }
                }
              }
            }
            ... on IssueComment {
              __typename
              id
              body
              createdAt
              author {
                login
              }
            }
            ... on PullRequestReview {
              __typename
              id
              state
              body
              submittedAt
              author {
                login
              }
            }
            ... on MergedEvent {
              __typename
              id
              createdAt
              actor {
                login
              }
              commit {
                oid
              }
              mergeRefName
            }
          }
        }
      }
    }
  }
}`
}

func buildPreviewQuery(profile githubpkg.GitHubHostProfile) string {
	return fmt.Sprintf(`query PreviewQuery($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    pullRequest(number: $number) {
      %s
    }
  }
}`, pullRequestPreviewSelection(profile))
}

func buildInvolvingSearchQuery(repo domain.Repository, viewer string) string {
	if full := repoFullName(repo); full != "" {
		return strings.TrimSpace(fmt.Sprintf("repo:%s is:pr is:open involves:%s", full, viewer))
	}
	return strings.TrimSpace(fmt.Sprintf("repo:%s/%s is:pr is:open involves:%s", repoOwner(repo), repoName(repo), viewer))
}

func pullRequestSummarySelection(profile githubpkg.GitHubHostProfile) string {
	var fields []string
	fields = append(fields,
		"id",
		"number",
		"title",
		"state",
		"isDraft",
		"createdAt",
		"updatedAt",
		"headRefName",
		"baseRefName",
		"additions",
		"deletions",
		"changedFiles",
		"comments { totalCount }",
		"reviewThreads { totalCount }",
		"author { __typename login }",
		"assignees(first: 10) { nodes { login } }",
		"reviewRequests(first: 10) { nodes { requestedReviewer { __typename ... on User { login } ... on Team { slug organization { login } } } } }",
		"latestOpinionatedReviews(first: 20) { nodes { state submittedAt author { __typename login } commit { oid } } }",
		"repository { nameWithOwner }",
	)
	if profile.SupportsHeadRefOID {
		fields = append(fields, "headRefOid")
	}
	fields = append(fields, rollupSelection(profile, false))
	return strings.Join(fields, "\n        ")
}

func pullRequestPreviewSelection(profile githubpkg.GitHubHostProfile) string {
	var fields []string
	fields = append(fields,
		"id",
		"number",
		"title",
		"body",
		"state",
		"isDraft",
		"createdAt",
		"updatedAt",
		"headRefName",
		"baseRefName",
		"additions",
		"deletions",
		"changedFiles",
		"reviewDecision",
		"author { login }",
		"reviews(first: 20) { nodes { author { login avatarUrl } state submittedAt body } }",
		"files(first: 20) { nodes { path additions deletions } }",
		"timelineItems(last: 1) { nodes { ... on PullRequestCommit { __typename id commit { oid messageHeadline committedDate author { user { login } name } } } ... on IssueComment { __typename id body createdAt author { login } } ... on PullRequestReview { __typename id state body submittedAt author { login } } ... on MergedEvent { __typename id createdAt actor { login } commit { oid } mergeRefName } } }",
		"repository { nameWithOwner }",
	)
	if profile.SupportsHeadRefOID {
		fields = append(fields, "headRefOid")
	}
	fields = append(fields, rollupSelection(profile, true))
	return strings.Join(fields, "\n      ")
}

func rollupSelection(profile githubpkg.GitHubHostProfile, detailed bool) string {
	rollupFields := []string{"state"}
	if detailed {
		rollupFields = append(rollupFields, "contexts(first: 20) { nodes { __typename ... on CheckRun { name status conclusion detailsUrl } ... on StatusContext { context state description targetUrl } } }")
	}
	rollup := "statusCheckRollup {\n          " + strings.Join(rollupFields, "\n          ") + "\n        }"
	if profile.SupportsTopLevelRollup {
		return rollup
	}
	return "commits(last: 1) {\n          nodes {\n            commit {\n              " + rollup + "\n            }\n          }\n        }"
}
