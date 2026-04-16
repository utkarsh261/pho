// Package rest provides a minimal GitHub REST client for fetching raw diffs.
package rest

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const (
	// acceptDiffHeader is the Accept header required to get raw unified diff output.
	acceptDiffHeader = "application/vnd.github.v3.diff"
	// userAgentHeader is a standard User-Agent for GitHub API requests.
	userAgentHeader = "pho/1.0"
)

// Client fetches raw diff content from the GitHub REST API.
type Client struct {
	// HTTPClient is used for making requests. If nil, http.DefaultClient is used.
	HTTPClient *http.Client
	// BaseURL is the GitHub REST API base (e.g. "https://api.github.com").
	BaseURL string
	// Token is the authentication token (without "token " prefix).
	Token string
}

// FetchRawDiff retrieves the raw unified diff for a specific PR.
//
// It uses the GitHub REST API endpoint:
//
//	GET /repos/{owner}/{repo}/pulls/{number}
//	with Accept: application/vnd.github.v3.diff
//
// Auth header: Authorization: token <token>
func (c *Client) FetchRawDiff(ctx context.Context, owner, repo string, number int) (string, error) {
	url := buildDiffURL(c.BaseURL, owner, repo, number)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("rest: create request: %w", err)
	}

	req.Header.Set("Accept", acceptDiffHeader)
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("rest: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("rest: read body: %w", err)
	}

	return string(raw), nil
}
