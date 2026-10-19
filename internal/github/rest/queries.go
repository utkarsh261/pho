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

// buildUpdateBranchURL constructs the REST API URL for updating a PR branch.
//
// Format: {baseURL}/repos/{owner}/{repo}/pulls/{number}/update-branch
func buildUpdateBranchURL(baseURL, owner, repo string, number int) string {
	return fmt.Sprintf("%s/repos/%s/%s/pulls/%d/update-branch", baseURL, owner, repo, number)
}

// buildRepoLabelsURL constructs the REST API URL for a repository's labels.
//
// Format: {baseURL}/repos/{owner}/{repo}/labels
func buildRepoLabelsURL(baseURL, owner, repo string) string {
	return fmt.Sprintf("%s/repos/%s/%s/labels", baseURL, owner, repo)
}

// buildIssueLabelsURL constructs the REST API URL for an issue's or pull
// request's labels (pull requests share the issues endpoint).
//
// Format: {baseURL}/repos/{owner}/{repo}/issues/{number}/labels
func buildIssueLabelsURL(baseURL, owner, repo string, number int) string {
	return fmt.Sprintf("%s/repos/%s/%s/issues/%d/labels", baseURL, owner, repo, number)
}
