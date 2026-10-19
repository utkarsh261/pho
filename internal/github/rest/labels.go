package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

// maxLabelPages caps how many pages of 100 labels are read for one listing.
const maxLabelPages = 10

// Label is a label as the REST API returns it.
type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// LabelOp says what a label request was doing, for error messages.
type LabelOp string

const (
	LabelOpCreate LabelOp = "create"
	LabelOpEdit   LabelOp = "edit"
)

// LabelPermissionError is returned when GitHub answers 403: the token can
// read the repo but can't change labels (editing needs triage, creating needs
// write access).
type LabelPermissionError struct {
	Op    LabelOp
	Owner string
	Repo  string
}

// Unwrap lets callers match the error with errors.Is(err, domain.ErrLabelPermission).
func (e *LabelPermissionError) Unwrap() error { return domain.ErrLabelPermission }

func (e *LabelPermissionError) Error() string {
	return fmt.Sprintf("no permission to %s labels in %s/%s", e.Op, e.Owner, e.Repo)
}

// ListRepoLabels returns the repository's labels, up to maxLabelPages pages.
// truncated is true when more labels exist beyond the cap.
//
//	GET /repos/{owner}/{repo}/labels
func (c *Client) ListRepoLabels(ctx context.Context, owner, repo string) (labels []Label, truncated bool, err error) {
	return c.listLabels(ctx, buildRepoLabelsURL(c.BaseURL, owner, repo))
}

// ListIssueLabels returns every label on an issue or pull request.
//
//	GET /repos/{owner}/{repo}/issues/{number}/labels
func (c *Client) ListIssueLabels(ctx context.Context, owner, repo string, number int) ([]Label, error) {
	labels, _, err := c.listLabels(ctx, buildIssueLabelsURL(c.BaseURL, owner, repo, number))
	return labels, err
}

func (c *Client) listLabels(ctx context.Context, base string) ([]Label, bool, error) {
	var all []Label
	for p := 1; p <= maxLabelPages; p++ {
		body, link, err := c.getJSON(ctx, pageURL(base, p))
		if err != nil {
			return nil, false, err
		}
		var page []Label
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, false, fmt.Errorf("rest: decode labels page %d: %w", p, err)
		}
		all = append(all, page...)
		if !nextPage(link) || len(page) < filesPerPage {
			return all, false, nil
		}
	}
	return all, true, nil
}

// AddLabels adds labels to an issue or pull request, keeping the ones it has.
//
//	POST /repos/{owner}/{repo}/issues/{number}/labels
func (c *Client) AddLabels(ctx context.Context, owner, repo string, number int, names []string) error {
	status, body, err := c.sendJSON(ctx, "rest add labels", http.MethodPost,
		buildIssueLabelsURL(c.BaseURL, owner, repo, number), map[string]any{"labels": names})
	if err != nil {
		return err
	}
	return labelStatusErr(status, body, http.StatusOK, LabelOpEdit, owner, repo)
}

// RemoveLabel removes one label from an issue or pull request. A label that
// is already gone (404) is not an error.
//
//	DELETE /repos/{owner}/{repo}/issues/{number}/labels/{name}
func (c *Client) RemoveLabel(ctx context.Context, owner, repo string, number int, name string) error {
	u := buildIssueLabelsURL(c.BaseURL, owner, repo, number) + "/" + url.PathEscape(name)
	status, body, err := c.sendJSON(ctx, "rest remove label", http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	return labelStatusErr(status, body, http.StatusOK, LabelOpEdit, owner, repo)
}

// CreateLabel creates a repository label. A label that already exists (422
// already_exists, e.g. someone created it meanwhile) is not an error.
//
//	POST /repos/{owner}/{repo}/labels
func (c *Client) CreateLabel(ctx context.Context, owner, repo, name, color string) error {
	status, body, err := c.sendJSON(ctx, "rest create label", http.MethodPost,
		buildRepoLabelsURL(c.BaseURL, owner, repo), map[string]any{"name": name, "color": color})
	if err != nil {
		return err
	}
	if status == http.StatusUnprocessableEntity && alreadyExists(body) {
		return nil
	}
	return labelStatusErr(status, body, http.StatusCreated, LabelOpCreate, owner, repo)
}

func labelStatusErr(status int, body []byte, want int, op LabelOp, owner, repo string) error {
	switch status {
	case want:
		return nil
	case http.StatusForbidden:
		return &LabelPermissionError{Op: op, Owner: owner, Repo: repo}
	case http.StatusUnprocessableEntity:
		if msg := validationMessage(body); msg != "" {
			return errors.New(msg)
		}
	}
	return fmt.Errorf("rest: unexpected status %d: %s", status, string(body))
}

// apiError is GitHub's error body: a message plus per-field validation errors.
type apiError struct {
	Message string `json:"message"`
	Errors  []struct {
		Code    string `json:"code"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"errors"`
}

func alreadyExists(body []byte) bool {
	var e apiError
	if json.Unmarshal(body, &e) != nil {
		return false
	}
	for _, fe := range e.Errors {
		if fe.Code == "already_exists" {
			return true
		}
	}
	return false
}

// validationMessage turns a 422 body into a short sentence, preferring the
// field-level message ("name is too long") over the generic "Validation Failed".
func validationMessage(body []byte) string {
	var e apiError
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	for _, fe := range e.Errors {
		switch {
		case fe.Message != "":
			return fe.Message
		case fe.Field != "" && fe.Code != "":
			return fmt.Sprintf("label %s is %s", fe.Field, fe.Code)
		}
	}
	return e.Message
}

// sendJSON performs an authenticated request with an optional JSON body and
// returns the status and (size-capped) response body.
func (c *Client) sendJSON(ctx context.Context, logMsg, method, u string, payload any) (int, []byte, error) {
	var statusCode int
	defer c.logRequest(logMsg, time.Now(), &statusCode)

	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("rest: marshal payload: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("rest: create request: %w", err)
	}
	req.Header.Set("Accept", acceptJSONHeader)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("Authorization", "token "+c.Token)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("rest: request failed: %w", err)
	}
	defer resp.Body.Close()
	statusCode = resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	return resp.StatusCode, body, nil
}
