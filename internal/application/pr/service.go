package pr

import (
	"context"
	"encoding/gob"
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
)

const (
	// Cache key prefixes.
	cacheKindPreview      = "preview"
	cacheKindDiff         = "diff"
	cacheKindDraftInline  = "draft_inline"
	cacheVersion          = 1

	// Preview cache TTL (shared with dashboard).
	cacheTTL = 2 * time.Minute
)

// PRService loads PR detail metadata and diffs.
type PRService struct {
	Cache        *cache.Coordinator
	Client       githubclient.GitHubClient
	REST         *rest.Client
	Now          func() time.Time
	Host         string // e.g. "github.com"
	Owner        string // repo owner
	Repo         string // repo name
	Log          logger
	BackgroundFn func(func())
}

type logger interface {
	Debug(msg string, fields ...any)
	Warn(msg string, fields ...any)
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
func (s *PRService) PostComment(ctx context.Context, prID string, body string) error {
	s.logDebug("post comment", "prID", prID)
	if err := s.Client.PostComment(ctx, s.Host, prID, body); err != nil {
		s.logWarn("post comment failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// PostReviewComment submits a PR review with COMMENT decision via the GitHub client.
func (s *PRService) PostReviewComment(ctx context.Context, prID string, body string) error {
	s.logDebug("post review comment", "prID", prID)
	if err := s.Client.PostReviewComment(ctx, s.Host, prID, body); err != nil {
		s.logWarn("post review comment failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// ApprovePR submits a PR review with APPROVE decision via the GitHub client.
func (s *PRService) ApprovePR(ctx context.Context, prID string, body string) error {
	s.logDebug("approve pr", "prID", prID)
	if err := s.Client.ApprovePullRequest(ctx, s.Host, prID, body); err != nil {
		s.logWarn("approve pr failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// SubmitReviewWithComments submits a PR review with inline comments.
func (s *PRService) SubmitReviewWithComments(ctx context.Context, prID, body, event string, comments []domain.DraftInlineComment) error {
	s.logDebug("submit review with comments", "prID", prID, "event", event, "comments", len(comments))
	if err := s.Client.SubmitReviewWithComments(ctx, s.Host, prID, body, event, comments); err != nil {
		s.logWarn("submit review with comments failed", "prID", prID, "err", err)
		return err
	}
	return nil
}

// SaveDraftComments persists draft inline comments for a PR.
func (s *PRService) SaveDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string, drafts []domain.DraftInlineComment) error {
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
	key := draftInlineCacheKey(repo.Host, repoFullName(repo), number, headSHA)
	if err := s.Cache.Delete(ctx, key); err != nil {
		s.logWarn("delete draft comments failed", "key", key, "err", err)
		return err
	}
	s.logDebug("draft comments deleted", "key", key)
	return nil
}

func (s *PRService) LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	if headSHA == "" {
		// No SHA available — use a placeholder key. Validation will be skipped.
		return s.loadDiffInner(ctx, repo, number, "", force)
	}
	return s.loadDiffInner(ctx, repo, number, headSHA, force)
}

func (s *PRService) loadDiffInner(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	key := diffCacheKey(repo.Host, repoFullName(repo), number, headSHA)

	var cached model.DiffModel
	found := false
	if !force && headSHA != "" {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
		if found {
			s.logDebug("diff cache hit", "key", key, "number", number)
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
			return cached, true, fmt.Errorf("refresh diff %s: %w", repo.FullName, err)
		}
		return model.DiffModel{}, false, fmt.Errorf("fetch raw diff: %w", err)
	}

	dm, err := parse.Parse(rawDiff)
	if err != nil {
		if found {
			s.logWarn("diff parse failed, returning stale", "key", key, "err", err)
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
		Encoding:  "gob",
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

func init() {
	gob.Register(model.DiffModel{})
	gob.Register(model.DiffFile{})
	gob.Register(model.DiffHunk{})
	gob.Register(model.DiffLine{})
	gob.Register([]model.DiffFile{})
	gob.Register([]model.DiffHunk{})
	gob.Register([]model.DiffLine{})
	gob.Register([]model.LineAnchor{})
	gob.Register(model.LineAnchor{})
}
