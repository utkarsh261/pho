package prdetail

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

// TestLoadErrorPanelRenders drives a failed initial detail load through the
// real Update path and snapshots the rendered Description tab. This is what a
// `pho pr <n>` deep link shows when the PR does not exist.
func TestLoadErrorPanelRenders(t *testing.T) {
	t.Parallel()

	m := makePRDetail(100, 30, nil, nil)
	m.Summary.Number = 999
	m.DetailLoading = true
	m, _ = m.Update(cmds.PRDetailLoaded{
		Host:   "github.com",
		Repo:   "owner/repo",
		Number: 999,
		Err:    errors.New("GraphQL: Could not resolve to a PullRequest with the number 999"),
	})

	if m.LoadErr == nil {
		t.Fatal("expected LoadErr to be set after a failed load")
	}
	if m.DetailLoading {
		t.Fatal("expected DetailLoading to be false after a failed load")
	}

	got := descStripANSI(strings.Join(m.renderDescriptionTab(0, 10, 80), "\n"))
	goldenPath := filepath.Join("testdata", "golden", "load_error_panel.txt")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to generate): %v", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for load error panel\ngot:\n%s\nwant:\n%s", got, string(want))
	}
}

// TestLoadErrorRetryRefiresLoad verifies `r` clears the error and re-issues
// the detail load, and esc leaves for the dashboard straight from the error
// state.
func TestLoadErrorRetryRefiresLoad(t *testing.T) {
	t.Parallel()

	m := makePRDetail(100, 30, nil, nil)
	m.Summary.Number = 999
	m.LoadErr = errors.New("boom")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected back-to-dashboard command on esc in error state")
	}

	retry := makePRDetail(100, 30, nil, nil)
	retry.Summary.Number = 999
	retry.LoadErr = errors.New("boom")

	updated, _ := retry.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if updated.LoadErr != nil {
		t.Fatalf("expected LoadErr cleared after retry, got %v", updated.LoadErr)
	}
	if !updated.DetailLoading {
		t.Fatal("expected DetailLoading true after retry")
	}
}

// TestPRDetailLoadedBackfillsEmptySummary verifies that a bare {repo, number}
// summary (deep-link start) gets its header fields from the loaded snapshot,
// while a populated summary is left untouched.
func TestPRDetailLoadedBackfillsEmptySummary(t *testing.T) {
	t.Parallel()

	detail := domain.PRPreviewSnapshot{
		Title:  "Fix login",
		Author: "octocat",
		State:  domain.PRStateOpen,
	}

	m := NewModel(domain.PullRequestSummary{Repo: "owner/repo", Number: 7}, domain.Repository{FullName: "owner/repo"}, nil)
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 7, Detail: detail})
	if m.Summary.Title != "Fix login" {
		t.Errorf("expected empty summary title backfilled, got %q", m.Summary.Title)
	}
	if m.Summary.Author != "octocat" {
		t.Errorf("expected empty summary author backfilled, got %q", m.Summary.Author)
	}
	if m.Summary.State != domain.PRStateOpen {
		t.Errorf("expected empty summary state backfilled, got %q", m.Summary.State)
	}

	m = NewModel(domain.PullRequestSummary{Repo: "owner/repo", Number: 7, Title: "Local title", Author: "me", State: domain.PRStateClosed}, domain.Repository{FullName: "owner/repo"}, nil)
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 7, Detail: detail})
	if m.Summary.Title != "Local title" || m.Summary.Author != "me" || m.Summary.State != domain.PRStateClosed {
		t.Errorf("expected populated summary untouched, got %+v", m.Summary)
	}
}
