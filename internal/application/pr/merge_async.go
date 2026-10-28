package pr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	pholog "github.com/utkarsh261/pho/internal/log"
)

// mergeAsyncPoll and mergeAsyncWait bound how long a merge waits on GitHub
// before it's reported as still running.
var (
	mergeAsyncPoll = time.Second
	mergeAsyncWait = 30 * time.Second
)

// MergeStackedPR merges a PR in a native GitHub stack, which GitHub only
// allows through its async merge API, then invalidates its preview.
func (s *PRService) MergeStackedPR(ctx context.Context, repo domain.Repository, number int, headRefOID string, method string) error {
	defer s.logTimer("pr merge stacked", pholog.FieldRepo, repo.FullName, pholog.FieldPRNumber, number, "method", method)()
	if err := s.mergeAsync(ctx, repo, number, headRefOID, method); err != nil {
		s.logWarn("merge stacked pr failed", "repo", repo.FullName, "number", number, "err", err)
		return err
	}
	s.dropPreview(ctx, repo, number)
	return nil
}

// mergeAsync merges through GitHub's async merge API and waits for the
// result: nil once merged (or queued), GitHub's reason when it fails, and
// domain.ErrMergeInProgress when it is still running at the deadline.
func (s *PRService) mergeAsync(ctx context.Context, repo domain.Repository, number int, headRefOID, method string) error {
	restClient, err := s.restFor(repo.Host)
	if err != nil {
		return err
	}
	owner, name := s.ownerName(repo), s.RepoName(repo)
	res, err := restClient.MergeAsync(ctx, owner, name, number, headRefOID, method)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(mergeAsyncWait)
	for {
		switch res.Status {
		case "merged", "enqueued":
			return nil
		case "failed":
			return errors.New(firstNonEmpty(res.Details.Message, "GitHub couldn't merge this pull request"))
		}
		uuid := res.Details.UUID
		if uuid == "" {
			// Nothing to follow, e.g. a 409 that names no running merge.
			return errors.New(firstNonEmpty(res.Details.Message, res.Message, "GitHub didn't start the merge"))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("#%d: %w", number, domain.ErrMergeInProgress)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for merge: %w", ctx.Err())
		case <-time.After(mergeAsyncPoll):
		}
		if res, err = restClient.MergeAsyncStatus(ctx, owner, name, number, uuid); err != nil {
			return err
		}
		if res.Details.UUID == "" {
			res.Details.UUID = uuid
		}
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
