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

// buildRepoURL constructs the REST API URL for fetching repo metadata.
//
// Format: {baseURL}/repos/{owner}/{repo}
func buildRepoURL(baseURL, owner, repo string) string {
	return fmt.Sprintf("%s/repos/%s/%s", baseURL, owner, repo)
}

// buildCreatePRURL constructs the REST API URL for creating a pull request.
//
// Format: {baseURL}/repos/{owner}/{repo}/pulls
func buildCreatePRURL(baseURL, owner, repo string) string {
	return fmt.Sprintf("%s/repos/%s/%s/pulls", baseURL, owner, repo)
}
