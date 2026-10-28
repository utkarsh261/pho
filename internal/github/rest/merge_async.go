package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	pholog "github.com/utkarsh261/pho/internal/log"
)

// MergeAsyncResult is GitHub's answer to an asynchronous merge: Status is
// "pending", "merged", "enqueued" or "failed".
type MergeAsyncResult struct {
	Status  string `json:"status"`
	Details struct {
		UUID    string `json:"uuid"`
		Message string `json:"message"`
	} `json:"details"`
}

func buildMergeAsyncURL(baseURL, owner, repo string, number int) string {
	return fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge-async", baseURL, owner, repo, number)
}

// MergeAsync starts merging a PR in the background, the only way GitHub
// merges a PR that is part of a stack. method is MERGE, SQUASH or REBASE.
func (c *Client) MergeAsync(ctx context.Context, owner, repo string, number int, expectedHeadSHA, method string) (MergeAsyncResult, error) {
	defer c.log.Timer("rest merge async", pholog.FieldHost, c.BaseURL)()
	payload := map[string]any{}
	if expectedHeadSHA != "" {
		payload["sha"] = expectedHeadSHA
	}
	if method != "" {
		payload["merge_method"] = strings.ToLower(method)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return MergeAsyncResult{}, fmt.Errorf("rest: marshal payload: %w", err)
	}
	// 409 means a merge is already running for this PR; follow that one.
	return c.mergeAsyncCall(ctx, http.MethodPut, buildMergeAsyncURL(c.BaseURL, owner, repo, number), body,
		http.StatusOK, http.StatusAccepted, http.StatusConflict)
}

// MergeAsyncStatus reads the result of a merge started by MergeAsync.
func (c *Client) MergeAsyncStatus(ctx context.Context, owner, repo string, number int, uuid string) (MergeAsyncResult, error) {
	url := buildMergeAsyncURL(c.BaseURL, owner, repo, number) + "/" + uuid
	return c.mergeAsyncCall(ctx, http.MethodGet, url, nil, http.StatusOK)
}

func (c *Client) mergeAsyncCall(ctx context.Context, method, url string, body []byte, ok ...int) (MergeAsyncResult, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return MergeAsyncResult{}, fmt.Errorf("rest: create request: %w", err)
	}
	req.Header.Set("Accept", acceptJSONHeader)
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return MergeAsyncResult{}, fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	accepted := false
	for _, code := range ok {
		accepted = accepted || resp.StatusCode == code
	}
	if !accepted {
		return MergeAsyncResult{}, fmt.Errorf("rest: unexpected status %d: %s", resp.StatusCode, string(data))
	}
	var out MergeAsyncResult
	if err := json.Unmarshal(data, &out); err != nil {
		return MergeAsyncResult{}, fmt.Errorf("rest: decode response: %w", err)
	}
	return out, nil
}
