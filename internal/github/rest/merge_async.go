package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MergeAsyncResult is GitHub's answer to an asynchronous merge: Status is
// "pending", "merged", "enqueued" or "failed".
type MergeAsyncResult struct {
	Status  string `json:"status"`
	Details struct {
		UUID    string `json:"uuid"`
		Message string `json:"message"`
	} `json:"details"`
	Message string `json:"message"` // set on error responses
}

func buildMergeAsyncURL(baseURL, owner, repo string, number int) string {
	return fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge-async", baseURL, owner, repo, number)
}

// MergeAsync starts merging a PR in the background, the only way GitHub
// merges a PR that is part of a stack. method is MERGE, SQUASH or REBASE.
func (c *Client) MergeAsync(ctx context.Context, owner, repo string, number int, expectedHeadSHA, method string) (MergeAsyncResult, error) {
	payload := map[string]any{}
	if expectedHeadSHA != "" {
		payload["sha"] = expectedHeadSHA
	}
	if method != "" {
		payload["merge_method"] = strings.ToLower(method)
	}
	// 409 means a merge is already running for this PR; its body names it.
	return c.mergeAsyncCall(ctx, "rest merge async", http.MethodPut, buildMergeAsyncURL(c.BaseURL, owner, repo, number), payload,
		http.StatusOK, http.StatusAccepted, http.StatusConflict)
}

// MergeAsyncStatus reads the result of a merge started by MergeAsync.
func (c *Client) MergeAsyncStatus(ctx context.Context, owner, repo string, number int, uuid string) (MergeAsyncResult, error) {
	url := buildMergeAsyncURL(c.BaseURL, owner, repo, number) + "/" + uuid
	return c.mergeAsyncCall(ctx, "rest merge async status", http.MethodGet, url, nil, http.StatusOK, http.StatusAccepted)
}

func (c *Client) mergeAsyncCall(ctx context.Context, logMsg, method, url string, payload any, ok ...int) (MergeAsyncResult, error) {
	status, body, err := c.sendJSON(ctx, logMsg, method, url, payload)
	if err != nil {
		return MergeAsyncResult{}, err
	}
	accepted := false
	for _, code := range ok {
		accepted = accepted || status == code
	}
	if !accepted {
		return MergeAsyncResult{}, fmt.Errorf("rest: unexpected status %d: %s", status, string(body))
	}
	var out MergeAsyncResult
	if err := json.Unmarshal(body, &out); err != nil {
		return MergeAsyncResult{}, fmt.Errorf("rest: decode response: %w", err)
	}
	return out, nil
}
