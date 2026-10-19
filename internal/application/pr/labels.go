package pr

import (
	"context"
	"fmt"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	pholog "github.com/utkarsh261/pho/internal/log"
)

const (
	cacheKindRepoLabels = "repo_labels"

	// Repo labels change rarely; the picker reads them often.
	repoLabelsTTL = 10 * time.Minute
)

// LoadRepoLabels returns the repository's labels, from cache while fresh.
// A failed fetch falls back to a stale cached copy when there is one.
func (s *PRService) LoadRepoLabels(ctx context.Context, repo domain.Repository) (domain.RepoLabelList, error) {
	defer s.logTimer("pr load repo labels", pholog.FieldRepo, repo.FullName)()
	key := repoLabelsCacheKey(repo.Host, repoFullName(repo))

	var cached domain.RepoLabelList
	_, freshness, found, _ := s.Cache.StaleWhileRevalidate(ctx, key, &cached, nil)
	if found && freshness == domain.FreshnessFresh {
		return cached, nil
	}

	restClient, err := s.restFor(repo.Host)
	if err != nil {
		return domain.RepoLabelList{}, err
	}
	list, truncated, err := restClient.ListRepoLabels(ctx, s.ownerName(repo), s.RepoName(repo))
	if err != nil {
		if found {
			s.logWarn("repo labels fetch failed, returning stale", "key", key, "err", err)
			return cached, nil
		}
		return domain.RepoLabelList{}, fmt.Errorf("load labels for %s: %w", repoFullName(repo), err)
	}

	out := domain.RepoLabelList{Labels: make([]domain.RepoLabel, len(list)), Truncated: truncated}
	for i, l := range list {
		out.Labels[i] = domain.RepoLabel{Label: domain.Label{Name: l.Name, Color: l.Color}, Description: l.Description}
	}
	now := s.Now().UTC()
	meta := domain.CacheMeta{
		Key:       key,
		Kind:      cacheKindRepoLabels,
		Version:   cacheVersion,
		Host:      repo.Host,
		Repo:      repoFullName(repo),
		FetchedAt: now,
		ExpiresAt: now.Add(repoLabelsTTL),
		Encoding:  "json",
	}
	if err := s.Cache.Write(ctx, key, out, meta); err != nil {
		s.logWarn("repo labels cache write error", "key", key, "err", err)
	}
	return out, nil
}

// LoadPRLabels returns every label on a pull request. The preview snapshot
// holds only the first page, so the picker uses this for PRs with more.
func (s *PRService) LoadPRLabels(ctx context.Context, repo domain.Repository, number int) ([]domain.Label, error) {
	defer s.logTimer("pr load pr labels", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	restClient, err := s.restFor(repo.Host)
	if err != nil {
		return nil, err
	}
	list, err := restClient.ListIssueLabels(ctx, s.ownerName(repo), s.RepoName(repo), number)
	if err != nil {
		return nil, fmt.Errorf("load labels for %s#%d: %w", repoFullName(repo), number, err)
	}
	out := make([]domain.Label, len(list))
	for i, l := range list {
		out[i] = domain.Label{Name: l.Name, Color: l.Color}
	}
	return out, nil
}

// ApplyLabels creates any new repository labels, then adds and removes
// labels on the pull request, stopping at the first failure. Labels are
// changed one call at a time (never replaced wholesale), so labels the
// caller didn't know about are left alone.
func (s *PRService) ApplyLabels(ctx context.Context, repo domain.Repository, number int, change domain.LabelChange) error {
	defer s.logTimer("pr apply labels", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number)()
	restClient, err := s.restFor(repo.Host)
	if err != nil {
		return err
	}
	owner, name := s.ownerName(repo), s.RepoName(repo)

	// Some of the change may land before a failure, so caches are dropped
	// whatever happens.
	defer func() {
		previewKey := previewCacheKey(repo.Host, repoFullName(repo), number)
		if delErr := s.Cache.Delete(ctx, previewKey); delErr != nil {
			s.logWarn("apply labels cache delete failed", "key", previewKey, "err", delErr)
		}
		if len(change.Create) > 0 {
			labelsKey := repoLabelsCacheKey(repo.Host, repoFullName(repo))
			if delErr := s.Cache.Delete(ctx, labelsKey); delErr != nil {
				s.logWarn("apply labels cache delete failed", "key", labelsKey, "err", delErr)
			}
		}
	}()

	for _, l := range change.Create {
		if err := restClient.CreateLabel(ctx, owner, name, l.Name, l.Color); err != nil {
			return s.labelErr("create label "+l.Name, repo, number, err)
		}
	}
	if len(change.Add) > 0 {
		if err := restClient.AddLabels(ctx, owner, name, number, change.Add); err != nil {
			return s.labelErr("add labels", repo, number, err)
		}
	}
	for _, l := range change.Remove {
		if err := restClient.RemoveLabel(ctx, owner, name, number, l); err != nil {
			return s.labelErr("remove label "+l, repo, number, err)
		}
	}
	return nil
}

func (s *PRService) labelErr(step string, repo domain.Repository, number int, err error) error {
	s.logWarn("apply labels failed", "repo", repo.FullName, "number", number, "step", step, "err", err)
	return err
}

func repoLabelsCacheKey(host, repo string) string {
	return fmt.Sprintf("labels:v1:host=%s:repo=%s", host, repo)
}
