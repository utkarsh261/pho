package dashboard

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/utk/git-term/internal/cache"
	"github.com/utk/git-term/internal/domain"
	githubclient "github.com/utk/git-term/internal/github"
)

const (
	cacheVersion             = 1
	cacheTTL                 = 2 * time.Minute
	cacheKindDashboardPRs    = "dashboard_prs"
	cacheKindDashboardInvolv = "dashboard_involving"
	cacheKindDashboardRecent = "dashboard_recent"
	cacheKindPreview         = "preview"
	defaultPreviewHost       = "github.com"
)

// DashboardService is the Phase 1 application service contract.
type DashboardService interface {
	SelectInitialRepo(repos []domain.Repository, cwd string) (*domain.Repository, error)
	LoadRepo(ctx context.Context, repo domain.Repository, force bool) (domain.DashboardSnapshot, error)
	LoadInvolving(ctx context.Context, repo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error)
	LoadRecent(ctx context.Context, repo domain.Repository, force bool) (domain.RecentSnapshot, error)
	LoadPreview(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error)
}

// Service implements DashboardService using the T2 cache coordinator and GitHub client.
type Service struct {
	Cache        *cache.Coordinator
	Client       githubclient.GitHubClient
	Classifier   SummaryTabClassifier
	DefaultHost  string
	Now          func() time.Time
	BackgroundFn func(func())
}

// NewService builds a dashboard service with sensible defaults.
func NewService(cacheCoordinator *cache.Coordinator, client githubclient.GitHubClient) *Service {
	return &Service{
		Cache:       cacheCoordinator,
		Client:      client,
		Classifier:  DefaultSummaryTabClassifier{},
		DefaultHost: defaultPreviewHost,
		Now:         time.Now,
	}
}

// SelectInitialRepo picks the cwd repo when present, otherwise the first repo.
func (s *Service) SelectInitialRepo(repos []domain.Repository, cwd string) (*domain.Repository, error) {
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories available")
	}

	if cwd != "" {
		want := cleanPath(cwd)
		for i := range repos {
			if cleanPath(repos[i].LocalPath) == want {
				selected := repos[i]
				return &selected, nil
			}
		}
	}

	selected := repos[0]
	return &selected, nil
}

// LoadRepo loads the dashboard summary snapshot for a repo.
func (s *Service) LoadRepo(ctx context.Context, repo domain.Repository, force bool) (domain.DashboardSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return domain.DashboardSnapshot{}, err
	}
	repo = normalizeRepository(repo)
	key := dashboardCacheKey(repo, "prs")

	var cached domain.DashboardSnapshot
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, func(string) {
			s.spawn(func() {
				_, _ = s.LoadRepo(context.Background(), repo, true)
			})
		})
		if found {
			return cached, nil
		}
	} else {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	prs, total, truncated, cursor, err := s.Client.FetchDashboardPRs(ctx, repo)
	if err != nil {
		if found {
			return cached, fmt.Errorf("refresh dashboard %s: %w", repo.FullName, err)
		}
		return domain.DashboardSnapshot{}, err
	}

	out := domain.DashboardSnapshot{
		Repo:       repo,
		PRs:        prs,
		TotalCount: total,
		Truncated:  truncated,
		EndCursor:  cursor,
		FetchedAt:  s.now().UTC(),
	}

	meta := dashboardMeta(key, repo, cacheKindDashboardPRs, nil, out.FetchedAt)
	if err := s.Cache.Write(ctx, key, out, meta); err != nil {
		return out, err
	}
	return out, nil
}

// LoadInvolving loads the involving snapshot for a repo and viewer.
func (s *Service) LoadInvolving(ctx context.Context, repo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return domain.InvolvingSnapshot{}, err
	}
	repo = normalizeRepository(repo)
	key := dashboardCacheKey(repo, "involving")

	var cached domain.InvolvingSnapshot
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, func(string) {
			s.spawn(func() {
				_, _ = s.LoadInvolving(context.Background(), repo, viewer, true)
			})
		})
		if found {
			return cached, nil
		}
	} else {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	prs, total, truncated, err := s.Client.FetchInvolvingPRs(ctx, repo, viewer)
	if err != nil {
		if found {
			return cached, fmt.Errorf("refresh involving %s: %w", repo.FullName, err)
		}
		return domain.InvolvingSnapshot{}, err
	}

	out := domain.InvolvingSnapshot{
		Repo:       repo,
		PRs:        prs,
		TotalCount: total,
		Truncated:  truncated,
		FetchedAt:  s.now().UTC(),
	}
	meta := dashboardMeta(key, repo, cacheKindDashboardInvolv, nil, out.FetchedAt)
	if err := s.Cache.Write(ctx, key, out, meta); err != nil {
		return out, err
	}
	return out, nil
}

// LoadRecent loads the recent activity snapshot for a repo.
func (s *Service) LoadRecent(ctx context.Context, repo domain.Repository, force bool) (domain.RecentSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return domain.RecentSnapshot{}, err
	}
	repo = normalizeRepository(repo)
	key := dashboardCacheKey(repo, "recent")

	var cached domain.RecentSnapshot
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, func(string) {
			s.spawn(func() {
				_, _ = s.LoadRecent(context.Background(), repo, true)
			})
		})
		if found {
			return cached, nil
		}
	} else {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	items, err := s.Client.FetchRecentActivity(ctx, repo)
	if err != nil {
		if found {
			return cached, fmt.Errorf("refresh recent %s: %w", repo.FullName, err)
		}
		return domain.RecentSnapshot{}, err
	}

	out := domain.RecentSnapshot{
		Repo:      repo,
		Items:     items,
		FetchedAt: s.now().UTC(),
	}
	meta := dashboardMeta(key, repo, cacheKindDashboardRecent, nil, out.FetchedAt)
	if err := s.Cache.Write(ctx, key, out, meta); err != nil {
		return out, err
	}
	return out, nil
}

// LoadPreview loads the richer preview snapshot for a PR.
func (s *Service) LoadPreview(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return domain.PRPreviewSnapshot{}, err
	}
	parsedRepo, err := s.resolvePreviewRepo(repo)
	if err != nil {
		return domain.PRPreviewSnapshot{}, err
	}
	return s.loadPreview(ctx, parsedRepo, number, false)
}

func (s *Service) loadPreview(ctx context.Context, parsedRepo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, error) {
	key := previewCacheKey(parsedRepo, number)

	var cached domain.PRPreviewSnapshot
	found := false
	if !force {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, func(string) {
			s.spawn(func() {
				_, _ = s.loadPreview(context.Background(), parsedRepo, number, true)
			})
		})
		if found {
			return cached, nil
		}
	} else {
		_, _, found, _ = s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	}

	preview, err := s.Client.FetchPreview(ctx, parsedRepo, number)
	if err != nil {
		if found {
			return cached, fmt.Errorf("refresh preview %s#%d: %w", parsedRepo.FullName, number, err)
		}
		return domain.PRPreviewSnapshot{}, err
	}

	out := preview
	out.Repo = parsedRepo.FullName
	out.Number = number

	meta := dashboardMeta(key, parsedRepo, cacheKindPreview, &number, s.now().UTC())
	if err := s.Cache.Write(ctx, key, out, meta); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) resolvePreviewRepo(raw string) (domain.Repository, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return domain.Repository{}, fmt.Errorf("preview repo is empty")
	}

	host := s.DefaultHost
	fullName := value
	if parts := strings.Split(value, "/"); len(parts) == 3 && looksLikeHost(parts[0]) {
		host = parts[0]
		fullName = parts[1] + "/" + parts[2]
	}

	owner, name, ok := strings.Cut(fullName, "/")
	if !ok || owner == "" || name == "" {
		return domain.Repository{Host: host, FullName: fullName}, nil
	}

	return domain.Repository{
		Host:     host,
		Owner:    owner,
		Name:     name,
		FullName: fullName,
	}, nil
}

func (s *Service) ensureReady() error {
	if s.Cache == nil {
		return fmt.Errorf("dashboard cache is nil")
	}
	if s.Client == nil {
		return fmt.Errorf("dashboard client is nil")
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.DefaultHost == "" {
		s.DefaultHost = defaultPreviewHost
	}
	return nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) spawn(fn func()) {
	if s.BackgroundFn != nil {
		s.BackgroundFn(fn)
		return
	}
	go fn()
}

func dashboardCacheKey(repo domain.Repository, kind string) string {
	return fmt.Sprintf("dashboard:v1:host=%s:repo=%s:kind=%s", repo.Host, repoIdentity(repo), kind)
}

func previewCacheKey(repo domain.Repository, number int) string {
	return fmt.Sprintf("preview:v1:host=%s:repo=%s:pr=%d", repo.Host, repoIdentity(repo), number)
}

func dashboardMeta(key string, repo domain.Repository, kind string, number *int, fetchedAt time.Time) domain.CacheMeta {
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      kind,
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoIdentity(repo),
		FetchedAt: fetchedAt,
		ExpiresAt: fetchedAt.Add(cacheTTL),
	}
	if number != nil {
		meta.PRNumber = number
	}
	return meta
}

func repoIdentity(repo domain.Repository) string {
	if repo.FullName != "" {
		return repo.FullName
	}
	if repo.Owner != "" && repo.Name != "" {
		return repo.Owner + "/" + repo.Name
	}
	return repo.Name
}

func normalizeRepository(repo domain.Repository) domain.Repository {
	if repo.FullName == "" {
		repo.FullName = repoIdentity(repo)
	}
	if repo.FullName != "" && (repo.Owner == "" || repo.Name == "") {
		if owner, name, ok := strings.Cut(repo.FullName, "/"); ok {
			if repo.Owner == "" {
				repo.Owner = owner
			}
			if repo.Name == "" {
				repo.Name = name
			}
		}
	}
	return repo
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func looksLikeHost(v string) bool {
	return strings.Contains(v, ".") || strings.Contains(v, ":") || v == "localhost"
}
