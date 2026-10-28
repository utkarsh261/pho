package pr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

// mergeAsyncPoll and mergeAsyncWait bound how long a merge waits on GitHub
// before it's reported as accepted rather than done.
var (
	mergeAsyncPoll = time.Second
	mergeAsyncWait = 30 * time.Second
)

// mergeAsync merges through GitHub's async merge API and waits for the
// result: nil once merged (or queued), an error with GitHub's reason when it
// fails.
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
			if res.Details.Message != "" {
				return errors.New(res.Details.Message)
			}
			return errors.New("GitHub couldn't merge this pull request")
		}
		if res.Details.UUID == "" || time.Now().After(deadline) {
			s.logWarn("async merge still pending", "repo", repo.FullName, "number", number)
			return nil // accepted; a refresh shows the outcome
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for merge: %w", ctx.Err())
		case <-time.After(mergeAsyncPoll):
		}
		uuid := res.Details.UUID
		if res, err = restClient.MergeAsyncStatus(ctx, owner, name, number, uuid); err != nil {
			return err
		}
		if res.Details.UUID == "" {
			res.Details.UUID = uuid
		}
	}
}
