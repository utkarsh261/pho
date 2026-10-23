package rest

import (
	"context"
	"encoding/json"
	"fmt"
)

// comparePageSize is how many commits pho asks for on the single compare
// page it reads. GitHub sends the changed files only on the first page.
const comparePageSize = 100

// Compare is the "Compare two commits" response, trimmed to what pho uses.
type Compare struct {
	// Status is "ahead", "behind", "identical" or "diverged": how the head
	// commit relates to the base commit.
	Status       string          `json:"status"`
	AheadBy      int             `json:"ahead_by"`
	TotalCommits int             `json:"total_commits"`
	Commits      []CompareCommit `json:"commits"`
	Files        []ChangedFile   `json:"files"`
}

// CompareCommit is one commit in a comparison.
type CompareCommit struct {
	SHA     string `json:"sha"`
	Parents []struct {
		SHA string `json:"sha"`
	} `json:"parents"`
}

// FetchCompare compares two commits (base...head). Only the first page of
// commits is read, which is enough to spot merges in a typical re-review.
func (c *Client) FetchCompare(ctx context.Context, owner, repo, base, head string) (Compare, error) {
	u := fmt.Sprintf("%s?per_page=%d", buildCompareURL(c.BaseURL, owner, repo, base, head), comparePageSize)
	body, _, err := c.getJSON(ctx, "rest compare fetch", u)
	if err != nil {
		return Compare{}, err
	}
	var cmp Compare
	if err := json.Unmarshal(body, &cmp); err != nil {
		return Compare{}, fmt.Errorf("rest: decode compare: %w", err)
	}
	return cmp, nil
}

// FetchCompareDiff retrieves the raw unified diff between two commits.
func (c *Client) FetchCompareDiff(ctx context.Context, owner, repo, base, head string) (string, error) {
	return c.fetchDiff(ctx, buildCompareURL(c.BaseURL, owner, repo, base, head))
}
