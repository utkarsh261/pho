package pr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/cache"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	githubclient "github.com/utkarsh261/pho/internal/github"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
)

const (
	// Cache key prefixes.
	cacheKindPreview     = "preview"
	cacheKindDiff        = "diff"
	cacheKindDraftInline = "draft_inline"
	cacheVersion         = 1

	// Preview cache TTL (shared with dashboard).
	cacheTTL = 2 * time.Minute
)

// PRService loads PR detail metadata and diffs.
type PRService struct {
	Cache        *cache.Coordinator
	Client       githubclient.GitHubClient
	REST         *rest.Client
	Now          func() time.Time
	Owner        string // repo owner
	Repo         string // repo name
	Log          *pholog.Logger
	BackgroundFn func(func())
}

// NewService builds a PR service with sensible defaults.
func NewService(cacheCoordinator *cache.Coordinator, client githubclient.GitHubClient, restClient *rest.Client) *PRService {
	return &PRService{
		Cache:  cacheCoordinator,
		Client: client,
		REST:   restClient,
		Now:    time.Now,
	}
}

func (s *PRService) LoadDetail(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error) {
	defer s.logTimer("pr load detail", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	key := previewCacheKey(repo.Host, repoFullName(repo), number)

	var cached domain.PRPreviewSnapshot
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, func(string) {
			s.spawnBackground(func() {
				_, _, _ = s.LoadDetail(context.Background(), repo, number, true)
			})
		})
		if found {
			s.logDebug("pr detail cache hit", "key", key, "number", number)
			return cached, true, nil
		}
	} else {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	s.logDebug("pr detail cache miss, fetching", "key", key, "number", number)

	preview, err := s.Client.FetchPreview(ctx, repo, number)
	if err != nil {
		if found {
			s.logWarn("pr detail fetch failed, returning stale", "key", key, "number", number, "err", err)
			return cached, true, fmt.Errorf("refresh pr detail %s: %w", repo.FullName, err)
		}
		return domain.PRPreviewSnapshot{}, false, err
	}

	meta := previewMeta(key, repo, number, s.Now().UTC())
	if err := s.Cache.Write(ctx, key, preview, meta); err != nil {
		s.logWarn("cache write error", "key", key, "err", err)
	}

	return preview, false, nil
}

// PostComment posts a PR-level comment via the GitHub client.
func (s *PRService) PostComment(ctx context.Context, repo domain.Repository, prID string, body string) error {
	defer s.logTimer("pr post comment", "prID", prID, pholog.FieldHost, repo.Host)()
	s.logDebug("post comment", "prID", prID, "host", repo.Host)
	if err := s.Client.PostComment(ctx, repo.Host, prID, body); err != nil {
		s.logWarn("post comment failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// PostReviewComment submits a PR review with COMMENT decision via the GitHub client.
func (s *PRService) PostReviewComment(ctx context.Context, repo domain.Repository, prID string, body string) error {
	defer s.logTimer("pr post review comment", "prID", prID, pholog.FieldHost, repo.Host)()
	s.logDebug("post review comment", "prID", prID, "host", repo.Host)
	if err := s.Client.PostReviewComment(ctx, repo.Host, prID, body); err != nil {
		s.logWarn("post review comment failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// ApprovePR submits a PR review with APPROVE decision via the GitHub client.
func (s *PRService) ApprovePR(ctx context.Context, repo domain.Repository, prID string, body string) error {
	defer s.logTimer("pr approve", "prID", prID, pholog.FieldHost, repo.Host)()
	s.logDebug("approve pr", "prID", prID, "host", repo.Host)
	if err := s.Client.ApprovePullRequest(ctx, repo.Host, prID, body); err != nil {
		s.logWarn("approve pr failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// SubmitReviewWithComments submits a PR review with inline comments.
func (s *PRService) SubmitReviewWithComments(ctx context.Context, repo domain.Repository, prID, body, event string, comments []domain.DraftInlineComment) error {
	defer s.logTimer("pr submit review", "prID", prID, "event", event, "comments", len(comments), pholog.FieldHost, repo.Host)()
	s.logDebug("submit review with comments", "prID", prID, "event", event, "comments", len(comments), "host", repo.Host)
	if err := s.Client.SubmitReviewWithComments(ctx, repo.Host, prID, body, event, comments); err != nil {
		s.logWarn("submit review with comments failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// CheckMergeable fetches fresh mergeability state for a PR.
func (s *PRService) CheckMergeable(ctx context.Context, repo domain.Repository, number int) (domain.MergeableState, error) {
	defer s.logTimer("pr check mergeable", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	s.logDebug("check mergeable", "repo", repo.FullName, "number", number)
	state, err := s.Client.CheckMergeable(ctx, repo, number)
	if err != nil {
		s.logWarn("check mergeable failed", "repo", repo.FullName, "number", number, "err", err)
		return domain.MergeableState{}, err
	}
	return state, nil
}

// MergePR merges a PR using the specified method and invalidates related caches.
func (s *PRService) MergePR(ctx context.Context, repo domain.Repository, number int, prID string, headRefOID string, method string) error {
	defer s.logTimer("pr merge", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number, "method", method)()
	s.logDebug("merge pr", "repo", repo.FullName, "number", number, "method", method)
	if err := s.Client.MergePullRequest(ctx, repo.Host, prID, headRefOID, method); err != nil {
		s.logWarn("merge pr failed", "repo", repo.FullName, "number", number, "err", err)
		return err
	}
	// Invalidate preview cache for this PR so the merged state is visible.
	previewKey := previewCacheKey(repo.Host, repoFullName(repo), number)
	if delErr := s.Cache.Delete(ctx, previewKey); delErr != nil {
		s.logWarn("merge pr cache delete failed", "key", previewKey, "err", delErr)
	}
	return nil
}

// ClosePR closes a PR and invalidates related caches.
func (s *PRService) ClosePR(ctx context.Context, repo domain.Repository, number int, prID string) error {
	defer s.logTimer("pr close", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	s.logDebug("close pr", "repo", repo.FullName, "number", number)
	if err := s.Client.ClosePullRequest(ctx, repo.Host, prID); err != nil {
		s.logWarn("close pr failed", "repo", repo.FullName, "number", number, "err", err)
		return err
	}
	previewKey := previewCacheKey(repo.Host, repoFullName(repo), number)
	if delErr := s.Cache.Delete(ctx, previewKey); delErr != nil {
		s.logWarn("close pr cache delete failed", "key", previewKey, "err", delErr)
	}
	return nil
}

// ReopenPR reopens a closed PR and invalidates related caches.
func (s *PRService) ReopenPR(ctx context.Context, repo domain.Repository, number int, prID string) error {
	defer s.logTimer("pr reopen", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	s.logDebug("reopen pr", "repo", repo.FullName, "number", number)
	if err := s.Client.ReopenPullRequest(ctx, repo.Host, prID); err != nil {
		s.logWarn("reopen pr failed", "repo", repo.FullName, "number", number, "err", err)
		return err
	}
	previewKey := previewCacheKey(repo.Host, repoFullName(repo), number)
	if delErr := s.Cache.Delete(ctx, previewKey); delErr != nil {
		s.logWarn("reopen pr cache delete failed", "key", previewKey, "err", delErr)
	}
	return nil
}

// UpdatePR updates the title and/or body of a PR and invalidates the preview cache.
func (s *PRService) UpdatePR(ctx context.Context, repo domain.Repository, number int, prID string, title string, body string) error {
	defer s.logTimer("pr update", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	s.logDebug("update pr", "repo", repo.FullName, "number", number)
	if err := s.Client.UpdatePullRequest(ctx, repo.Host, prID, title, body); err != nil {
		s.logWarn("update pr failed", "repo", repo.FullName, "number", number, "err", err)
		return err
	}
	previewKey := previewCacheKey(repo.Host, repoFullName(repo), number)
	if delErr := s.Cache.Delete(ctx, previewKey); delErr != nil {
		s.logWarn("update pr cache delete failed", "key", previewKey, "err", delErr)
	}
	return nil
}

// SaveDraftComments persists draft inline comments for a PR.
func (s *PRService) SaveDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string, drafts []domain.DraftInlineComment) error {
	defer s.logTimer("pr save drafts", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	key := draftInlineCacheKey(repo.Host, repoFullName(repo), number, headSHA)
	meta := draftInlineMeta(key, repo, number, headSHA, s.Now().UTC())
	if err := s.Cache.Write(ctx, key, drafts, meta); err != nil {
		s.logWarn("save draft comments failed", "key", key, "err", err)
		return err
	}
	s.logDebug("draft comments saved", "key", key, "count", len(drafts))
	return nil
}

// LoadDraftComments loads draft inline comments for a PR.
// If headSHA is empty or doesn't match the stored SHA, returns empty slice.
func (s *PRService) LoadDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) ([]domain.DraftInlineComment, error) {
	defer s.logTimer("pr load drafts", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	if headSHA == "" {
		return nil, nil
	}
	key := draftInlineCacheKey(repo.Host, repoFullName(repo), number, headSHA)
	var drafts []domain.DraftInlineComment
	_, found, err := s.Cache.L2.Get(ctx, key, &drafts)
	if err != nil {
		s.logWarn("load draft comments failed", "key", key, "err", err)
		return nil, err
	}
	if !found {
		return nil, nil
	}
	s.logDebug("draft comments loaded", "key", key, "count", len(drafts))
	return drafts, nil
}

// DeleteDraftComments removes draft inline comments for a PR.
func (s *PRService) DeleteDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) error {
	defer s.logTimer("pr delete drafts", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	key := draftInlineCacheKey(repo.Host, repoFullName(repo), number, headSHA)
	if err := s.Cache.Delete(ctx, key); err != nil {
		s.logWarn("delete draft comments failed", "key", key, "err", err)
		return err
	}
	s.logDebug("draft comments deleted", "key", key)
	return nil
}

func (s *PRService) LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	defer s.logTimer("pr load diff", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	if headSHA == "" {
		// No SHA available — use a placeholder key. Validation will be skipped.
		return s.loadDiffInner(ctx, repo, number, "", force)
	}
	return s.loadDiffInner(ctx, repo, number, headSHA, force)
}

func (s *PRService) loadDiffInner(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	defer s.logTimer("pr load diff inner", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	key := diffCacheKey(repo.Host, repoFullName(repo), number, headSHA)

	var cached model.DiffModel
	found := false
	if !force && headSHA != "" {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
		if found {
			s.logDebug("diff cache hit", "key", key, "number", number)
			anchor.Generate(&cached, headSHA)
			return cached, true, nil
		}
	} else if force && headSHA != "" {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	s.logDebug("fetching raw diff", "key", key, "number", number, "host", repo.Host)

	rawDiff, err := s.REST.FetchRawDiff(ctx, s.ownerName(repo), s.RepoName(repo), number)
	if err != nil {
		if found && headSHA != "" {
			s.logWarn("diff fetch failed, returning stale", "key", key, "number", number, "err", err)
			anchor.Generate(&cached, headSHA)
			return cached, true, fmt.Errorf("refresh diff %s: %w", repo.FullName, err)
		}
		return model.DiffModel{}, false, fmt.Errorf("fetch raw diff: %w", err)
	}

	dm, err := parse.Parse(rawDiff)
	if err != nil {
		if found {
			s.logWarn("diff parse failed, returning stale", "key", key, "err", err)
			anchor.Generate(&cached, headSHA)
			return cached, true, fmt.Errorf("parse diff: %w", err)
		}
		return model.DiffModel{}, false, fmt.Errorf("parse diff: %w", err)
	}

	// Populate HeadSHA from the GraphQL result (not from the raw diff index line).
	dm.HeadSHA = headSHA
	dm.Repo = repoFullName(repo)
	dm.PRNumber = number

	// SHA validation.
	if headSHA != "" && dm.HeadSHA != "" && dm.HeadSHA != headSHA {
		s.logWarn("diff head SHA mismatch, refetching",
			"cached_sha", dm.HeadSHA, "expected_sha", headSHA, "number", number)
		// Discard the model — caller should refetch with force=true.
		return model.DiffModel{}, false, nil
	}

	// Generate anchors.
	anchor.Generate(dm, headSHA)

	// Precompute StartRow for file-level virtualization.
	cumulative := 0
	for i := range dm.Files {
		dm.Files[i].StartRow = cumulative
		cumulative += dm.Files[i].DisplayRows
	}

	// Cache the DiffModel.
	if headSHA != "" {
		meta := diffMeta(key, repo, number, s.Now().UTC())
		if err := s.Cache.Write(ctx, key, dm, meta); err != nil {
			s.logWarn("diff cache write error", "key", key, "err", err)
		}
	}

	return *dm, false, nil
}

// LoadPRCommits loads the commit list for a PR via GraphQL.
func (s *PRService) LoadPRCommits(ctx context.Context, repo domain.Repository, number int, force bool) ([]domain.Commit, error) {
	defer s.logTimer("pr load commits", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	key := commitsCacheKey(repo.Host, repoFullName(repo), number)

	var cached []domain.Commit
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
		if found {
			s.logDebug("commits cache hit", "key", key, "number", number)
			return cached, nil
		}
	}

	s.logDebug("commits cache miss, fetching", "key", key, "number", number)

	commits, err := s.Client.FetchCommits(ctx, repo, number)
	if err != nil {
		if found {
			s.logWarn("commits fetch failed, returning stale", "key", key, "number", number, "err", err)
			return cached, fmt.Errorf("fetch commits %s: %w", repo.FullName, err)
		}
		return nil, fmt.Errorf("fetch commits: %w", err)
	}

	meta := commitsMeta(key, repo, number, s.Now().UTC())
	if err := s.Cache.Write(ctx, key, commits, meta); err != nil {
		s.logWarn("commits cache write error", "key", key, "err", err)
	}

	return commits, nil
}

// LoadCommitDiff loads the raw diff for a single commit via REST.
func (s *PRService) LoadCommitDiff(ctx context.Context, repo domain.Repository, sha string, force bool) (model.DiffModel, error) {
	defer s.logTimer("pr load commit diff", pholog.FieldRepo, repo.FullName, "sha", sha)()
	key := commitDiffCacheKey(repo.Host, repoFullName(repo), sha)

	var cached model.DiffModel
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
		if found {
			s.logDebug("commit diff cache hit", "key", key, "sha", sha)
			return cached, nil
		}
	}

	s.logDebug("commit diff cache miss, fetching", "key", key, "sha", sha)

	rawDiff, err := s.REST.FetchCommitDiff(ctx, s.ownerName(repo), s.RepoName(repo), sha)
	if err != nil {
		if found {
			s.logWarn("commit diff fetch failed, returning stale", "key", key, "sha", sha, "err", err)
			return cached, fmt.Errorf("fetch commit diff %s: %w", repo.FullName, err)
		}
		return model.DiffModel{}, fmt.Errorf("fetch commit diff: %w", err)
	}

	dm, err := parse.Parse(rawDiff)
	if err != nil {
		if found {
			s.logWarn("commit diff parse failed, returning stale", "key", key, "sha", sha, "err", err)
			return cached, fmt.Errorf("parse commit diff: %w", err)
		}
		return model.DiffModel{}, fmt.Errorf("parse commit diff: %w", err)
	}

	dm.HeadSHA = sha
	dm.Repo = repoFullName(repo)

	anchor.Generate(dm, sha)

	// Precompute StartRow for file-level virtualization.
	cumulative := 0
	for i := range dm.Files {
		dm.Files[i].StartRow = cumulative
		cumulative += dm.Files[i].DisplayRows
	}

	meta := commitDiffMeta(key, repo, sha, s.Now().UTC())
	if err := s.Cache.Write(ctx, key, *dm, meta); err != nil {
		s.logWarn("commit diff cache write error", "key", key, "err", err)
	}

	return *dm, nil
}

func (s *PRService) RepoName(repo domain.Repository) string {
	if repo.Name != "" {
		return repo.Name
	}
	parts := strings.Split(repo.FullName, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return repo.FullName
}

func (s *PRService) ownerName(repo domain.Repository) string {
	if repo.Owner != "" {
		return repo.Owner
	}
	parts := strings.Split(repo.FullName, "/")
	if len(parts) == 2 {
		return parts[0]
	}
	return s.Owner
}

func (s *PRService) spawnBackground(fn func()) {
	if s.BackgroundFn != nil {
		s.BackgroundFn(fn)
	} else {
		go fn()
	}
}

func (s *PRService) logDebug(msg string, fields ...any) {
	if s.Log != nil {
		s.Log.Debug(msg, fields...)
	}
}

func (s *PRService) logWarn(msg string, fields ...any) {
	if s.Log != nil {
		s.Log.Warn(msg, fields...)
	}
}

func (s *PRService) logTimer(msg string, fields ...any) func() {
	if s.Log != nil {
		return s.Log.Timer(msg, fields...)
	}
	return func() {}
}

func repoFullName(repo domain.Repository) string {
	if repo.FullName != "" {
		return repo.FullName
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner + "/" + repo.Name
	}
	return repo.Name
}

func previewCacheKey(host, repo string, number int) string {
	return fmt.Sprintf("preview:v2:host=%s:repo=%s:pr=%d", host, repo, number)
}

func diffCacheKey(host, repo string, number int, sha string) string {
	return fmt.Sprintf("diff:v1:host=%s:repo=%s:pr=%d:sha=%s", host, repo, number, sha)
}

func previewMeta(key string, repo domain.Repository, number int, fetchedAt time.Time) domain.CacheMeta {
	return domain.CacheMeta{
		Key:       key,
		Kind:      cacheKindPreview,
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoFullName(repo),
		PRNumber:  &number,
		FetchedAt: fetchedAt,
		ExpiresAt: fetchedAt.Add(cacheTTL),
		Encoding:  "json",
	}
}

func diffMeta(key string, repo domain.Repository, number int, fetchedAt time.Time) domain.CacheMeta {
	// Diff cache is immutable — no expiry.
	farFuture := fetchedAt.Add(365 * 24 * time.Hour)
	return domain.CacheMeta{
		Key:       key,
		Kind:      cacheKindDiff,
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoFullName(repo),
		PRNumber:  &number,
		FetchedAt: fetchedAt,
		ExpiresAt: farFuture,
		Encoding:  "json",
	}
}

func commitsCacheKey(host, repo string, number int) string {
	return fmt.Sprintf("commits:v1:host=%s:repo=%s:pr=%d", host, repo, number)
}

func commitDiffCacheKey(host, repo, sha string) string {
	return fmt.Sprintf("commitdiff:v1:host=%s:repo=%s:sha=%s", host, repo, sha)
}

func commitsMeta(key string, repo domain.Repository, number int, fetchedAt time.Time) domain.CacheMeta {
	return domain.CacheMeta{
		Key:       key,
		Kind:      "commits",
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoFullName(repo),
		PRNumber:  &number,
		FetchedAt: fetchedAt,
		ExpiresAt: fetchedAt.Add(cacheTTL),
		Encoding:  "json",
	}
}

func commitDiffMeta(key string, repo domain.Repository, sha string, fetchedAt time.Time) domain.CacheMeta {
	// Commit diff cache is immutable — no expiry.
	farFuture := fetchedAt.Add(365 * 24 * time.Hour)
	return domain.CacheMeta{
		Key:       key,
		Kind:      cacheKindDiff,
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoFullName(repo),
		FetchedAt: fetchedAt,
		ExpiresAt: farFuture,
		Encoding:  "json",
	}
}

func draftInlineCacheKey(host, repo string, number int, sha string) string {
	return fmt.Sprintf("draft_inline:v1:host=%s:repo=%s:pr=%d:sha=%s", host, repo, number, sha)
}

func draftInlineMeta(key string, repo domain.Repository, number int, headSHA string, fetchedAt time.Time) domain.CacheMeta {
	// Draft comments persist indefinitely — no expiry.
	farFuture := fetchedAt.Add(365 * 24 * time.Hour)
	return domain.CacheMeta{
		Key:       key,
		Kind:      cacheKindDraftInline,
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoFullName(repo),
		PRNumber:  &number,
		FetchedAt: fetchedAt,
		ExpiresAt: farFuture,
		Encoding:  "json",
	}
}
