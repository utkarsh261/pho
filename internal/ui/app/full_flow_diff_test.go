package app

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

// TestFullFlow500FileDiff verifies that a 500-file diff loads, renders, and
// scrolls without panicking. This exercises the file-level virtualization path
// under realistic large-diff conditions.
func TestFullFlow500FileDiff(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/large")
	summary := pr(repo.FullName, 42, "Large refactor")
	prs := []domain.PullRequestSummary{summary}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)

	// Open PR detail.
	m.focus = domain.FocusPRListPanel
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab:     domain.TabMyPRs,
		Index:   0,
		Repo:    repo.FullName,
		Number:  42,
		Summary: summary,
	})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view after Enter, got %s", m.currentView())
	}

	// Build a 500-file diff. Each file has 1 hunk with 3 lines.
	files := make([]diffmodel.DiffFile, 500)
	for i := range files {
		files[i] = diffmodel.DiffFile{
			OldPath: fmt.Sprintf("pkg/module%d/file.go", i),
			NewPath: fmt.Sprintf("pkg/module%d/file.go", i),
			Status:  "modified",
			Hunks: []diffmodel.DiffHunk{
				{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []diffmodel.DiffLine{
						{Kind: "context", Raw: " unchanged"},
						{Kind: "deletion", Raw: "-old line"},
						{Kind: "addition", Raw: "+new line"},
					},
				},
			},
		}
	}
	diff := diffmodel.DiffModel{
		Repo:     repo.FullName,
		PRNumber: 42,
		Files:    files,
		Stats: diffmodel.DiffStats{
			TotalFiles:     500,
			TotalAdditions: 500,
			TotalDeletions: 500,
		},
	}

	// Deliver the diff through the root Update path (same as real production flow).
	_, _ = m.Update(cmds.DiffLoaded{Diff: diff})

	if m.prDetail == nil {
		t.Fatal("expected prDetail to be non-nil after DiffLoaded")
	}
	if m.prDetail.Diff == nil {
		t.Fatal("expected Diff to be set on prDetail after DiffLoaded")
	}
	if len(m.prDetail.Diff.Files) != 500 {
		t.Fatalf("expected 500 files in diff, got %d", len(m.prDetail.Diff.Files))
	}

	// Render — must not panic and must return a non-empty string.
	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty View() output for 500-file diff")
	}

	// Move focus to content viewport via Tab, then scroll down 200 rows.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	for i := 0; i < 200; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}

	// View must still render without panic after scrolling.
	view = m.View()
	if view == "" {
		t.Fatal("expected non-empty View() after scrolling 200 rows")
	}

	// Scroll to bottom (G key) and back to top (g g).
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	view = m.View()
	if view == "" {
		t.Fatal("expected non-empty View() after G (scroll to bottom)")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	view = m.View()
	if view == "" {
		t.Fatal("expected non-empty View() after g g (scroll to top)")
	}
}
