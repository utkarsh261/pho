package prdetail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// TestViewFillsTerminalExactly guards the layout contract: across terminal
// sizes, tabs, and with the compose pane open, the PR detail view is exactly
// as tall as the terminal and no line is wider than it.
func TestViewFillsTerminalExactly(t *testing.T) {
	t.Parallel()
	for _, w := range []int{60, 79, 80, 100, 120, 200} {
		for _, h := range []int{20, 30, 40} {
			m := makePRDetail(w, h, makeFiles("a/b.go", "c.go"), nil)
			m.Detail = makeDetailWithBody("body")
			m.SetTheme(theme.Default())
			for _, tab := range []ContentTab{TabDescription, TabDiff, TabComments, TabCommits} {
				m.switchTab(tab)
				v := m.View()
				if got := lipgloss.Height(v); got != h {
					t.Errorf("w=%d h=%d tab=%d: view height %d", w, h, tab, got)
				}
				for i, l := range strings.Split(v, "\n") {
					if lw := lipgloss.Width(l); lw > w {
						t.Errorf("w=%d h=%d tab=%d line %d width %d", w, h, tab, i, lw)
					}
				}
			}
			m.compose.Open(composeModeNew, commentEntry{}, 0)
			if got := lipgloss.Height(m.View()); got != h {
				t.Errorf("w=%d h=%d compose: view height %d", w, h, got)
			}
		}
	}
}

// TestCommitHeaderLongHeadlineDoesNotWrap guards the commit-detail header:
// a very long headline is truncated instead of wrapping and growing the view.
func TestCommitHeaderLongHeadlineDoesNotWrap(t *testing.T) {
	t.Parallel()
	for _, w := range []int{60, 80, 120} {
		m := makePRDetail(w, 30, makeFiles("a.go"), nil)
		m.SetTheme(theme.Default())
		m.CommitMode = true
		m.Commit = domain.Commit{SHA: "abc1234def", MessageHeadline: strings.Repeat("very long headline ", 20), AuthorLogin: "alice"}
		if got := lipgloss.Height(m.renderHeader()); got != 3 {
			t.Errorf("w=%d: commit header height %d, want 3", w, got)
		}
		if got := lipgloss.Height(m.View()); got != 30 {
			t.Errorf("w=%d: view height %d, want 30", w, got)
		}
	}
}

// TestCommentRowCountMatchesRenderAcrossWidths sweeps content widths so a
// collapsed resolved thread nested under a review, and a comment containing a
// word longer than the card (a URL), hit every wrap boundary. The row layout
// used for scrolling must match what commentLines actually renders.
func TestCommentRowCountMatchesRenderAcrossWidths(t *testing.T) {
	t.Parallel()
	threadAt := time.Date(2024, 3, 10, 9, 0, 0, 0, time.UTC)
	m := makePRDetail(160, 40, nil, nil)
	m.SetTheme(theme.Default())
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "carol", State: "COMMENTED", Body: "Overall looks fine", SubmittedAt: threadAt.Add(2 * time.Minute)},
		},
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "dave", CreatedAt: threadAt.Add(time.Hour),
				Body: "See https://github.com/utkarsh261/pho/blob/main/internal/ui/views/prdetail/comments.go#L120-L180 for context"},
		},
		ReviewThreads: []domain.PreviewReviewThread{{
			ID: "t1", Path: "internal/adapters/telegram/handlers_batch.go", Line: 120,
			IsResolved: true, ResolvedBy: "alice",
			Comments: []domain.PreviewThreadComment{
				{ID: "c1", Login: "bob", Body: "nit", CreatedAt: threadAt},
				{ID: "c2", Login: "alice", Body: "done", CreatedAt: threadAt.Add(time.Minute)},
			},
		}},
	}
	entries := m.commentEntries()
	var summaryIdx = -1
	for i, e := range entries {
		if e.isResolvedSummary && e.indentByParentReview {
			summaryIdx = i
		}
	}
	if summaryIdx < 0 {
		t.Fatal("fixture should produce an indented resolved summary")
	}
	for cw := 30; cw <= 120; cw++ {
		starts := m.commentEntryStartRows(cw)
		last := len(entries) - 1
		want := starts[last] + m.entryRenderHeight(entries[last], cw, entries, last) + 1 // trailing blank
		if got := len(m.commentLines(cw, -1)); got != want {
			t.Errorf("cw=%d: commentLines has %d rows, layout expects %d", cw, got, want)
		}
	}
}

// TestSelectedFileStaysVisibleWhileScrolling walks the file cursor down a list
// longer than the panel and checks the selected file is drawn after every
// step, with and without CI checks and with the compose pane open.
func TestSelectedFileStaysVisibleWhileScrolling(t *testing.T) {
	t.Parallel()
	names := make([]string, 60)
	for i := range names {
		names[i] = fmt.Sprintf("pkg/f%03d.go", i)
	}
	checks := []domain.PreviewCheckRow{{Name: "build", State: "SUCCESS"}, {Name: "lint", State: "SUCCESS"}}
	for _, tc := range []struct {
		name    string
		checks  []domain.PreviewCheckRow
		compose bool
	}{{"no_ci", nil, false}, {"ci", checks, false}, {"ci_compose", checks, true}} {
		for _, h := range []int{24, 30, 40} {
			m := makePRDetail(120, h, makeFiles(names...), tc.checks)
			m.SetTheme(theme.Default())
			m.leftPanel.Focus = FocusFiles
			if tc.compose {
				m.compose.Open(composeModeNew, commentEntry{}, 0)
			}
			for step := range names {
				if step > 0 {
					m.leftPanel.Cursor++
					m.ensureFileVisible()
				}
				want := names[m.leftPanel.Cursor][len("pkg/"):]
				m.cachedBody = "" // compose reuses the last body render; force a fresh one
				if !strings.Contains(descStripANSI(m.View()), want) {
					t.Fatalf("%s h=%d: selected %s not drawn after %d steps", tc.name, h, want, step)
				}
			}
		}
	}
}
