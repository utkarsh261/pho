// Package rest provides a minimal GitHub REST client for fetching raw diffs.
package rest

import (
	"context"
	"fmt"
	"io"
	"net/http"

	pholog "github.com/utkarsh261/pho/internal/log"
)

const (
	acceptDiffHeader = "application/vnd.github.v3.diff"

	userAgentHeader = "pho/1.0"
)

type Client struct {
	HTTPClient *http.Client

	BaseURL string

	Token string

	log *pholog.Logger
}

// NewClient creates a new REST client with the given base URL, token, and logger.
func NewClient(baseURL, token string, logger *pholog.Logger) *Client {
	if logger == nil {
		logger = pholog.NewNop()
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		log:     logger,
	}
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
	var statusCode int
	defer c.log.Timer("rest diff fetch", pholog.FieldHost, c.BaseURL, pholog.FieldStatusCode, statusCode)()

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
	statusCode = resp.StatusCode

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

// FetchCommitDiff retrieves the raw unified diff for a single commit.
//
// It uses the GitHub REST API endpoint:
//
//	GET /repos/{owner}/{repo}/commits/{sha}
//	with Accept: application/vnd.github.v3.diff
//
// Auth header: Authorization: token <token>
func (c *Client) FetchCommitDiff(ctx context.Context, owner, repo, sha string) (string, error) {
	var statusCode int
	defer c.log.Timer("rest diff fetch", pholog.FieldHost, c.BaseURL, pholog.FieldStatusCode, statusCode)()

	url := buildCommitDiffURL(c.BaseURL, owner, repo, sha)

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
	statusCode = resp.StatusCode

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
