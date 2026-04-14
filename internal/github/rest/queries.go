package rest

import "fmt"

// buildDiffURL constructs the REST API URL for fetching a raw PR diff.
//
// Format: {baseURL}/repos/{owner}/{repo}/pulls/{number}
func buildDiffURL(baseURL, owner, repo string, number int) string {
	return fmt.Sprintf("%s/repos/%s/%s/pulls/%d", baseURL, owner, repo, number)
}
