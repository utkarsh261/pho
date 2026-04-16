package mocks

import (
	"context"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

func TestMockGitHubClient_UnsetFnPanics(t *testing.T) {
	m := &MockGitHubClient{}

	t.Run("FetchViewer panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic but did not get one")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic, got %T: %v", r, r)
			}
			want := "MockGitHubClient.FetchViewer called but FetchViewerFn is nil"
			if msg != want {
				t.Errorf("expected panic message %q, got %q", want, msg)
			}
		}()
		_, _ = m.FetchViewer(context.Background(), "github.com")
	})

	t.Run("FetchDashboardPRs panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic but did not get one")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic, got %T: %v", r, r)
			}
			want := "MockGitHubClient.FetchDashboardPRs called but FetchDashboardPRsFn is nil"
			if msg != want {
				t.Errorf("expected panic message %q, got %q", want, msg)
			}
		}()
		_, _, _, _, _ = m.FetchDashboardPRs(context.Background(), testutil.Repo("alice/myrepo"))
	})

	t.Run("FetchInvolvingPRs panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic but did not get one")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic, got %T: %v", r, r)
			}
			want := "MockGitHubClient.FetchInvolvingPRs called but FetchInvolvingPRsFn is nil"
			if msg != want {
				t.Errorf("expected panic message %q, got %q", want, msg)
			}
		}()
		_, _, _, _ = m.FetchInvolvingPRs(context.Background(), testutil.Repo("alice/myrepo"), "alice")
	})

	t.Run("FetchRecentActivity panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic but did not get one")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic, got %T: %v", r, r)
			}
			want := "MockGitHubClient.FetchRecentActivity called but FetchRecentActivityFn is nil"
			if msg != want {
				t.Errorf("expected panic message %q, got %q", want, msg)
			}
		}()
		_, _ = m.FetchRecentActivity(context.Background(), testutil.Repo("alice/myrepo"))
	})

	t.Run("FetchPreview panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic but did not get one")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic, got %T: %v", r, r)
			}
			want := "MockGitHubClient.FetchPreview called but FetchPreviewFn is nil"
			if msg != want {
				t.Errorf("expected panic message %q, got %q", want, msg)
			}
		}()
		_, _ = m.FetchPreview(context.Background(), testutil.Repo("alice/myrepo"), 1)
	})
}

func TestMockGitHubClient_FetchDashboardPRs_CounterAndResult(t *testing.T) {
	repo := testutil.Repo("alice/myrepo")
	pr1 := testutil.PR(1)
	pr2 := testutil.PR(2)

	m := &MockGitHubClient{
		FetchDashboardPRsFn: func(ctx context.Context, r domain.Repository) ([]domain.PullRequestSummary, int, bool, string, error) {
			return []domain.PullRequestSummary{pr1, pr2}, 2, false, "cursor_xyz", nil
		},
	}

	if m.FetchDashboardPRsCalls != 0 {
		t.Errorf("expected counter=0 before call, got %d", m.FetchDashboardPRsCalls)
	}

	summaries, total, hasMore, cursor, err := m.FetchDashboardPRs(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("expected 2 summaries, got %d", len(summaries))
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if hasMore {
		t.Error("expected hasMore=false")
	}
	if cursor != "cursor_xyz" {
		t.Errorf("expected cursor=cursor_xyz, got %q", cursor)
	}
	if m.FetchDashboardPRsCalls != 1 {
		t.Errorf("expected counter=1 after first call, got %d", m.FetchDashboardPRsCalls)
	}

	// Call again to verify counter increments.
	_, _, _, _, _ = m.FetchDashboardPRs(context.Background(), repo)
	if m.FetchDashboardPRsCalls != 2 {
		t.Errorf("expected counter=2 after second call, got %d", m.FetchDashboardPRsCalls)
	}
}

func TestMockGitHubClient_FetchPreview_Counter(t *testing.T) {
	repo := testutil.Repo("alice/myrepo")
	snap := domain.PRPreviewSnapshot{Number: 42, Title: "test"}

	m := &MockGitHubClient{
		FetchPreviewFn: func(ctx context.Context, r domain.Repository, number int) (domain.PRPreviewSnapshot, error) {
			return snap, nil
		},
	}

	if m.FetchPreviewCalls != 0 {
		t.Errorf("expected counter=0 before call, got %d", m.FetchPreviewCalls)
	}

	result, err := m.FetchPreview(context.Background(), repo, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Number != 42 {
		t.Errorf("expected Number=42, got %d", result.Number)
	}
	if m.FetchPreviewCalls != 1 {
		t.Errorf("expected counter=1 after call, got %d", m.FetchPreviewCalls)
	}
}
