package pr

import (
	"context"
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	"github.com/utk/git-term/internal/cache"
	"github.com/utk/git-term/internal/diff/anchor"
	"github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/diff/parse"
	"github.com/utk/git-term/internal/domain"
	githubclient "github.com/utk/git-term/internal/github"
	"github.com/utk/git-term/internal/github/rest"
)

const (
	// Cache key prefixes.
	cacheKindPreview = "preview"
	cacheKindDiff    = "diff"
	cacheVersion     = 1

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
	key := previewCacheKey(s.Host, repoFullName(repo), number)

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

func (s *PRService) LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	if headSHA == "" {
		// No SHA available — use a placeholder key. Validation will be skipped.
		return s.loadDiffInner(ctx, repo, number, "", force)
	}
	return s.loadDiffInner(ctx, repo, number, headSHA, force)
}

func (s *PRService) loadDiffInner(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (model.DiffModel, bool, error) {
	key := diffCacheKey(s.Host, repoFullName(repo), number, headSHA)

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

	s.logDebug("fetching raw diff", "key", key, "number", number, "host", s.Host)

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
