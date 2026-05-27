// Package rest provides a minimal GitHub REST client for fetching raw diffs.
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/utkarsh261/pho/internal/domain"
	pholog "github.com/utkarsh261/pho/internal/log"
)

const (
	acceptDiffHeader = "application/vnd.github.v3.diff"
	acceptJSONHeader = "application/vnd.github+json"

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

// RepoInfo holds metadata about a GitHub repository.
type RepoInfo struct {
	DefaultBranch string `json:"default_branch"`
	Fork          bool   `json:"fork"`
	Parent        *struct {
		FullName string `json:"full_name"`
	} `json:"parent,omitempty"`
}

// PullRequestResponse holds the REST API response for creating a pull request.
type PullRequestResponse struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	Head    struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	IsDraft   bool   `json:"draft"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
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

// FetchRepoInfo retrieves metadata for a repository.
func (c *Client) FetchRepoInfo(ctx context.Context, owner, repo string) (RepoInfo, error) {
	var statusCode int
	defer c.log.Timer("rest repo info", pholog.FieldHost, c.BaseURL, pholog.FieldStatusCode, statusCode)()

	url := buildRepoURL(c.BaseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("rest: create request: %w", err)
	}

	req.Header.Set("Accept", acceptJSONHeader)
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return RepoInfo{}, fmt.Errorf("rest: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return RepoInfo{}, fmt.Errorf("rest: decode response: %w", err)
	}

	return info, nil
}

// CreatePullRequest creates a new pull request via the GitHub REST API.
func (c *Client) CreatePullRequest(ctx context.Context, owner, repo string, params domain.CreatePRParams) (PullRequestResponse, error) {
	var statusCode int
	defer c.log.Timer("rest create pr", pholog.FieldHost, c.BaseURL, pholog.FieldStatusCode, statusCode)()

	url := buildCreatePRURL(c.BaseURL, owner, repo)

	payload := map[string]any{
		"title": params.Title,
		"head":  params.Head,
		"base":  params.Base,
		"draft": params.Draft,
	}
	if params.Body != "" {
		payload["body"] = params.Body
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PullRequestResponse{}, fmt.Errorf("rest: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return PullRequestResponse{}, fmt.Errorf("rest: create request: %w", err)
	}

	req.Header.Set("Accept", acceptJSONHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return PullRequestResponse{}, fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return PullRequestResponse{}, fmt.Errorf("rest: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var pr PullRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return PullRequestResponse{}, fmt.Errorf("rest: decode response: %w", err)
	}

	return pr, nil
}
