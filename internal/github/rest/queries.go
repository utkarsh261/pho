package rest

import "fmt"

// buildDiffURL constructs the REST API URL for fetching a raw PR diff.
//
// Format: {baseURL}/repos/{owner}/{repo}/pulls/{number}
func buildDiffURL(baseURL, owner, repo string, number int) string {
	return fmt.Sprintf("%s/repos/%s/%s/pulls/%d", baseURL, owner, repo, number)
}

// buildCommitDiffURL constructs the REST API URL for fetching a raw commit diff.
//
// Format: {baseURL}/repos/{owner}/{repo}/commits/{sha}
func buildCommitDiffURL(baseURL, owner, repo, sha string) string {
	return fmt.Sprintf("%s/repos/%s/%s/commits/%s", baseURL, owner, repo, sha)
}
