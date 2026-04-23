package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	githubpkg "github.com/utkarsh261/pho/internal/github"
	"github.com/utkarsh261/pho/internal/github/model"
	gitlog "github.com/utkarsh261/pho/internal/log"
)

// Client implements github.GitHubClient over GitHub GraphQL.
type Client struct {
	httpClient *http.Client
	log        *gitlog.Logger

	mu    sync.RWMutex
	hosts map[string]*hostState
}

type hostState struct {
	profile githubpkg.GitHubHostProfile
}

// NewClient builds a GraphQL transport from host profiles.
func NewClient(profiles []githubpkg.GitHubHostProfile, httpClient *http.Client, logger *gitlog.Logger) *Client {
	if logger == nil {
		logger = gitlog.NewNop()
	}
	hosts := make(map[string]*hostState, len(profiles))
	for _, profile := range profiles {
		p := profile
		if p.GraphQLURL == "" {
			p.GraphQLURL = githubpkg.DefaultGraphQLURL(p.Host)
		}
		if p.RESTURL == "" {
			p.RESTURL = githubpkg.DefaultRESTURL(p.Host)
		}
		hosts[p.Host] = &hostState{profile: p}
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{httpClient: httpClient, log: logger, hosts: hosts}
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
	Path    []any  `json:"path,omitempty"`
}

type graphQLResponse[T any] struct {
	Data   T              `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

// PostComment posts a PR-level comment using the GraphQL addComment mutation.
func (c *Client) PostComment(ctx context.Context, host, pullRequestID, body string) error {
	_, err := queryGraphQL[model.AddCommentData](c, ctx, host, func(_ githubpkg.GitHubHostProfile) string {
		return buildAddCommentMutation()
	}, map[string]any{
		"subjectId": pullRequestID,
		"body":      body,
	})
	return err
}

// ApprovePullRequest submits a PR review with APPROVE decision via GraphQL.
func (c *Client) ApprovePullRequest(ctx context.Context, host, pullRequestID, body string) error {
	vars := map[string]any{
		"pullRequestId": pullRequestID,
		"body":          body,
	}
	if body == "" {
		vars["body"] = nil
	}
	_, err := queryGraphQL[model.AddPullRequestReviewData](c, ctx, host, func(_ githubpkg.GitHubHostProfile) string {
		return buildApprovePullRequestMutation()
	}, vars)
	return err
}

// FetchViewer resolves the current viewer login for a host.
func (c *Client) FetchViewer(ctx context.Context, host string) (string, error) {
	c.log.Debug("fetch viewer", "host", host)
	resp, err := queryGraphQL[model.ViewerData](c, ctx, host, func(profile githubpkg.GitHubHostProfile) string {
		return buildViewerQuery()
	}, nil)
	if err != nil {
		return "", err
	}
	return resp.Data.Viewer.Login, nil
}

// FetchDashboardPRs loads the repo dashboard PR list.
func (c *Client) FetchDashboardPRs(ctx context.Context, repo domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
	c.log.Debug("fetch dashboard", "repo", repo.FullName, "host", repo.Host)
	resp, err := queryGraphQL[model.DashboardData](c, ctx, repo.Host, func(profile githubpkg.GitHubHostProfile) string {
		return buildDashboardQuery(profile)
	}, map[string]any{
		"owner": repoOwner(repo),
		"name":  repoName(repo),
	})
	if err != nil {
		return nil, 0, false, "", err
	}
	summaries, total, truncated, cursor, err := normalizeDashboardResponse(repo, resp.Data)
	if err != nil {
		return nil, 0, false, "", err
	}
	return summaries, total, truncated, cursor, nil
}

// FetchInvolvingPRs loads the repo-scoped involving search results.
func (c *Client) FetchInvolvingPRs(ctx context.Context, repo domain.Repository, viewer string) ([]domain.PullRequestSummary, int, bool, error) {
	c.log.Debug("fetch involving", "repo", repo.FullName, "host", repo.Host, "viewer", viewer)
	resp, err := queryGraphQL[model.InvolvingData](c, ctx, repo.Host, func(profile githubpkg.GitHubHostProfile) string {
		return buildInvolvingQuery(profile)
	}, map[string]any{
		"query": buildInvolvingSearchQuery(repo, viewer),
	})
	if err != nil {
		return nil, 0, false, err
	}
	summaries, total, truncated, err := normalizeInvolvingResponse(repo, resp.Data)
	if err != nil {
		return nil, 0, false, err
	}
	return summaries, total, truncated, nil
}

// FetchRecentActivity loads recent activity rows for the repository.
func (c *Client) FetchRecentActivity(ctx context.Context, repo domain.Repository) ([]domain.ActivityItem, error) {
	c.log.Debug("fetch recent", "repo", repo.FullName, "host", repo.Host)
	resp, err := queryGraphQL[model.RecentData](c, ctx, repo.Host, func(profile githubpkg.GitHubHostProfile) string {
		return buildRecentActivityQuery()
	}, map[string]any{
		"owner": repoOwner(repo),
		"name":  repoName(repo),
	})
	if err != nil {
		return nil, err
	}
	items, err := normalizeRecentResponse(repo, resp.Data)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// FetchAllPRs loads one page of all PRs (any state) for the jump index.
func (c *Client) FetchAllPRs(ctx context.Context, repo domain.Repository, cursor string) ([]domain.PullRequestSummary, bool, string, error) {
	c.log.Debug("fetch all prs", "repo", repo.FullName, "host", repo.Host, "cursor", cursor)
	resp, err := queryGraphQL[model.DashboardData](c, ctx, repo.Host, func(_ githubpkg.GitHubHostProfile) string {
		return buildAllPRsQuery(repoOwner(repo), repoName(repo), cursor)
	}, nil)
	if err != nil {
		return nil, false, "", err
	}
	summaries, hasMore, nextCursor, err := normalizeAllPRsResponse(repo, resp.Data)
	if err != nil {
		return nil, false, "", err
	}
	return summaries, hasMore, nextCursor, nil
}

// FetchPreview loads the richer PR preview snapshot.
func (c *Client) FetchPreview(ctx context.Context, repo domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
	c.log.Debug("fetch preview", "repo", repo.FullName, "host", repo.Host, "number", number)
	resp, err := queryGraphQL[model.PreviewData](c, ctx, repo.Host, func(profile githubpkg.GitHubHostProfile) string {
		return buildPreviewQuery(profile)
	}, map[string]any{
		"owner":  repoOwner(repo),
		"name":   repoName(repo),
		"number": number,
	})
	if err != nil {
		return domain.PRPreviewSnapshot{}, err
	}
	return normalizePreviewResponse(repo, number, resp.Data)
}

func queryGraphQL[T any](c *Client, ctx context.Context, host string, build func(githubpkg.GitHubHostProfile) string, vars map[string]any) (graphQLResponse[T], error) {
	profile := c.profileForHost(host)
	query := build(profile)

	queryType := query
	if idx := strings.IndexAny(query, " \t\n{("); idx > 0 {
		queryType = query[:idx]
	}
	c.log.Debug("graphql request", "host", host, "graphql_url", profile.GraphQLURL, "query_type", queryType)

	start := time.Now()
	resp, err := doGraphQLQuery[T](c, ctx, profile, query, vars)
	if err != nil {
		c.log.Error("graphql request failed", "host", host, "graphql_url", profile.GraphQLURL, "err", err)
		var zero graphQLResponse[T]
		return zero, err
	}
	ms := time.Since(start).Milliseconds()
	if len(resp.Errors) == 0 {
		c.log.Debug("graphql response ok", "host", host, "graphql_url", profile.GraphQLURL, gitlog.FieldDurationMS, ms)
		return resp, nil
	}

	if c.applyCapabilityFallback(host, resp.Errors) {
		profile = c.profileForHost(host)
		query = build(profile)
		resp2, err := doGraphQLQuery[T](c, ctx, profile, query, vars)
		if err != nil {
			c.log.Error("graphql request failed", "host", host, "err", err)
			var zero graphQLResponse[T]
			return zero, err
		}
		if len(resp2.Errors) > 0 {
			return graphQLResponse[T]{}, graphQLErrorList(resp2.Errors)
		}
		return resp2, nil
	}

	return graphQLResponse[T]{}, graphQLErrorList(resp.Errors)
}

func doGraphQLQuery[T any](c *Client, ctx context.Context, profile githubpkg.GitHubHostProfile, query string, vars map[string]any) (graphQLResponse[T], error) {
	var out graphQLResponse[T]

	payload, err := json.Marshal(graphQLRequest{
		Query:     query,
		Variables: vars,
	})
	if err != nil {
		return out, fmt.Errorf("marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, profile.GraphQLURL, bytes.NewReader(payload))
	if err != nil {
		return out, fmt.Errorf("build GraphQL request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if profile.Token != "" {
		req.Header.Set("Authorization", "bearer "+profile.Token)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("execute GraphQL request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return out, fmt.Errorf("read GraphQL response: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return out, fmt.Errorf("GraphQL HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("decode GraphQL response: %w", err)
	}
	return out, nil
}

func (c *Client) profileForHost(host string) githubpkg.GitHubHostProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if hs, ok := c.hosts[host]; ok {
		return hs.profile
	}
	// No authenticated profile found - using default (no token)
	c.log.Debug("using default host profile (not authenticated)", "host", host, "graphql_url", githubpkg.DefaultGraphQLURL(host))
	return defaultProfileForHost(host)
}

func (c *Client) applyCapabilityFallback(host string, errs []graphQLError) bool {
	needsRollupFallback := false
	needsHeadRefOIDFallback := false
	for _, err := range errs {
		msg := strings.ToLower(err.Message)
		if strings.Contains(msg, "statuscheckrollup") {
			needsRollupFallback = true
		}
		if strings.Contains(msg, "headrefoid") {
			needsHeadRefOIDFallback = true
		}
	}
	if !needsRollupFallback && !needsHeadRefOIDFallback {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.hosts[host]
	if !ok {
		state = &hostState{profile: defaultProfileForHost(host)}
		c.hosts[host] = state
	}
	changed := false
	if needsRollupFallback && state.profile.SupportsTopLevelRollup {
		state.profile.SupportsTopLevelRollup = false
		changed = true
	}
	if needsHeadRefOIDFallback && state.profile.SupportsHeadRefOID {
		state.profile.SupportsHeadRefOID = false
		changed = true
	}
	return changed
}

type graphQLErrorList []graphQLError

func (e graphQLErrorList) Error() string {
	if len(e) == 0 {
		return "graphql query failed"
	}
	msgs := make([]string, 0, len(e))
	for _, item := range e {
		msgs = append(msgs, item.Message)
	}
	return "graphql query failed: " + strings.Join(msgs, "; ")
}

func defaultProfileForHost(host string) githubpkg.GitHubHostProfile {
	return githubpkg.GitHubHostProfile{
		Host:                   host,
		GraphQLURL:             githubpkg.DefaultGraphQLURL(host),
		RESTURL:                githubpkg.DefaultRESTURL(host),
		SupportsTopLevelRollup: true,
		SupportsHeadRefOID:     true,
	}
}

func repoOwner(repo domain.Repository) string {
	if repo.Owner != "" {
		return repo.Owner
	}
	if parts := strings.SplitN(repo.FullName, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func repoName(repo domain.Repository) string {
	if repo.Name != "" {
		return repo.Name
	}
	if parts := strings.SplitN(repo.FullName, "/", 2); len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func repoFullName(repo domain.Repository) string {
	if repo.FullName != "" {
		return repo.FullName
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner + "/" + repo.Name
	}
	return ""
}
