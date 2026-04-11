package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/ui/theme"
)

func TestPreviewPanelRenderSnapshot(t *testing.T) {
	t.Parallel()

	m := NewPreviewPanelModel()
	snap := domain.PRPreviewSnapshot{
		Repo:           "org/repo",
		Number:         42,
		Title:          "Improve dashboard rendering",
		BodyExcerpt:    "This preview text is long enough to force truncation and show the marker",
		Author:         "alice",
		State:          domain.PRStateOpen,
		CIStatus:       domain.CIStatusSuccess,
		ReviewDecision: domain.ReviewDecisionApproved,
		CreatedAt:      time.Date(2026, 4, 9, 12, 30, 0, 0, time.FixedZone("IST", 5*60*60+30*60)),
		UpdatedAt:      time.Date(2026, 4, 9, 13, 45, 0, 0, time.FixedZone("IST", 5*60*60+30*60)),
		TopFiles: []domain.PreviewFileStat{
			{Path: "cmd/main.go", Additions: 120, Deletions: 12},
			{Path: "internal/ui/views/dashboard/preview_panel.go", Additions: 40, Deletions: 3},
		},
		LatestActivity: &domain.ActivitySnippet{
			Kind:      domain.ActivityKindComment,
			Author:    "bob",
			Body:      "Looks good to me",
			OccuredAt: time.Date(2026, 4, 9, 14, 0, 0, 0, time.UTC),
		},
		Checks: makeChecks(10),
	}
	m.preview = &snap
	m.SetRect(100, 30)

	view := m.View()
	checks := []string{
		"Improve dashboard rendering",
		"org/repo  #42",
		"Author: alice",
		"State: open",
		"CI: success",
		"Review: approved",
		"2026-04-09 12:30 IST",
		"2026-04-09 13:45 IST",
		"...",
		"+120 -12 cmd/main.go",
		"Latest activity:",
		"comment by bob",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in view, got %q", want, view)
		}
	}
}

func TestPreviewPanelCICap(t *testing.T) {
	t.Parallel()

	m := NewPreviewPanelModel()
	snap := domain.PRPreviewSnapshot{
		Repo:   "org/repo",
		Number: 1,
		Title:  "PR",
		Author: "alice",
		Checks: makeChecks(10),
	}
	m.preview = &snap
	m.SetRect(80, 30)

	view := m.View()
	if !strings.Contains(view, "+4 more") {
		t.Fatalf("expected ci cap footer, got %q", view)
	}
	if strings.Count(view, "check-") < 6 {
		t.Fatalf("expected at least 6 rendered checks, got %q", view)
	}
}

func TestPreviewPanelDebounce(t *testing.T) {
	t.Parallel()

	m := NewPreviewPanelModel()
	summary := makePR(7, "Debounce", "feature/debounce")
	var commands int
	for i := 0; i < 5; i++ {
		_, cmd := m.Update(SelectPRMsg{
			Repo:    summary.Repo,
			Number:  summary.Number,
			Summary: summary,
		})
		if cmd != nil {
			commands++
		}
	}
	if commands != 1 {
		t.Fatalf("expected at most one fetch command, got %d", commands)
	}
}

// TestPreviewPanelNarrowWidth verifies that at a width where the old single-line
// "Author | State | CI | Review" metadata would have been truncated before the
// Review field, all four pieces of metadata are still visible.
func TestPreviewPanelNarrowWidth(t *testing.T) {
	t.Parallel()

	m := NewPreviewPanelModel()
	m.SetTheme(theme.Default())
	snap := domain.PRPreviewSnapshot{
		Repo:           "utkarsh261/hisaab",
		Number:         13,
		Title:          "feat: multi-intent v2",
		Author:         "utkarsh261",
		State:          domain.PRStateOpen,
		CIStatus:       domain.CIStatusSuccess,
		ReviewDecision: domain.ReviewDecisionReviewRequired,
		CreatedAt:      time.Date(2026, 4, 3, 20, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 1, 20, 0, 0, 0, time.UTC),
	}
	m.preview = &snap
	// Width 50: old one-liner "Author: utkarsh261 | State: open | CI: success | Review: review required"
	// is ~74 chars and would have been truncated before "Review:".
	m.SetRect(50, 20)

	view := m.View()

	for _, want := range []string{"Author:", "State:", "CI:", "Review:", "Created:", "Updated:"} {
		if !strings.Contains(view, want) {
			t.Errorf("width=50: expected %q to be visible in view, but it was not\n%s", want, view)
		}
	}
}

// TestPreviewPanelNoLineOverflow verifies that no rendered line exceeds the
// panel content width, across a range of widths.
func TestPreviewPanelNoLineOverflow(t *testing.T) {
	t.Parallel()

	snap := domain.PRPreviewSnapshot{
		Repo:           "utkarsh261/hisaab",
		Number:         13,
		Title:          "feat: multi-intent v2 — parse rules and transactions from a single message",
		Author:         "utkarsh261",
		State:          domain.PRStateOpen,
		CIStatus:       domain.CIStatusSuccess,
		ReviewDecision: domain.ReviewDecisionChangesRequested,
		BodyExcerpt:    "## Summary - Multi-intent routing: new v2 LLM prompt routes a single user message",
		CreatedAt:      time.Date(2026, 4, 3, 20, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 1, 20, 0, 0, 0, time.UTC),
		TopFiles: []domain.PreviewFileStat{
			{Path: "internal/adapters/telegram/handlers_batch.go", Additions: 552, Deletions: 0},
			{Path: "internal/adapters/telegram/handlers_intent_.go", Additions: 634, Deletions: 0},
		},
		Checks: makeChecks(8),
	}

	for _, width := range []int{20, 30, 40, 50, 60, 80, 120} {
		width := width
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			t.Parallel()
			m := NewPreviewPanelModel()
			m.SetTheme(theme.Default())
			m.preview = &snap
			m.SetRect(width, 40)

			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				// Use lipgloss.Width to measure visible chars (strips ANSI).
				if got := lipgloss.Width(line); got > width {
					t.Errorf("line %d is %d chars wide, exceeds panel width %d:\n  %q", i, got, width, line)
				}
			}
		})
	}
}

// TestPreviewPanelFilePathEndVisible verifies that when file paths are long, the
// filename (end of path) stays visible rather than being chopped off on the right.
func TestPreviewPanelFilePathEndVisible(t *testing.T) {
	t.Parallel()

	m := NewPreviewPanelModel()
	m.SetTheme(theme.Default())
	m.preview = &domain.PRPreviewSnapshot{
		Repo:   "org/repo",
		Number: 1,
		Title:  "PR",
		Author: "alice",
		TopFiles: []domain.PreviewFileStat{
			// Path that is definitely longer than any reasonable narrow panel.
			{Path: "internal/adapters/telegram/handlers_batch.go", Additions: 552, Deletions: 0},
		},
	}
	// Width 40: path (44 chars) + prefix (+552 -0 = 8 chars) + indent (2) = 54 chars total.
	// The filename "handlers_batch.go" (17 chars) must still be visible.
	m.SetRect(40, 20)

	view := m.View()
	if !strings.Contains(view, "handlers_batch.go") {
		t.Errorf("filename not visible at width=40; view:\n%s", view)
	}
}

// TestPreviewPanelTimestampsSeparateLines verifies Created/Updated are on
// separate lines (not jammed together on one line).
func TestPreviewPanelTimestampsSeparateLines(t *testing.T) {
	t.Parallel()

	m := NewPreviewPanelModel()
	snap := domain.PRPreviewSnapshot{
		Repo:      "org/repo",
		Number:    1,
		Title:     "PR",
		Author:    "alice",
		CreatedAt: time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 9, 13, 45, 0, 0, time.UTC),
	}
	m.preview = &snap
	m.SetRect(80, 20)

	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		stripped := strings.TrimSpace(line)
		// No single line should contain both "Created:" and "Updated:".
		if strings.Contains(stripped, "Created:") && strings.Contains(stripped, "Updated:") {
			t.Errorf("Created and Updated timestamps are on the same line:\n  %q", stripped)
		}
	}
}

func makeChecks(n int) []domain.PreviewCheckRow {
	checks := make([]domain.PreviewCheckRow, 0, n)
	for i := 0; i < n; i++ {
		checks = append(checks, domain.PreviewCheckRow{
			Name:  fmt.Sprintf("check-%d", i+1),
			State: "success",
		})
	}
	return checks
}

func TestPreviewPanelVimNavigation(t *testing.T) {
	t.Parallel()

	snap := domain.PRPreviewSnapshot{
		Repo:        "org/repo",
		Number:      1,
		Title:       "PR",
		Author:      "alice",
		BodyExcerpt: strings.Repeat("line of text. ", 100),
		TopFiles:    makeFileStat(10),
		Checks:      makeChecks(6),
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	newPanel := func() *PreviewPanelModel {
		m := NewPreviewPanelModel()
		m.preview = &snap
		m.SetRect(80, 20)
		return m
	}

	t.Run("gg goes to top", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Scroll = 10
		m.Update(key("g"))
		m.Update(key("g"))
		if m.Scroll != 0 {
			t.Fatalf("gg: expected scroll=0, got %d", m.Scroll)
		}
	})

	t.Run("single g does not jump", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Scroll = 5
		m.Update(key("g"))
		if m.Scroll != 5 {
			t.Fatalf("single g: expected scroll unchanged at 5, got %d", m.Scroll)
		}
	})

	t.Run("G goes to bottom", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Scroll = 0
		m.Update(key("G"))
		if m.Scroll == 0 {
			t.Fatalf("G: expected scroll to advance to bottom, got 0")
		}
	})

	t.Run("ctrl+d scrolls down half page", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Scroll = 0
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		if m.Scroll != m.Height/2 {
			t.Fatalf("ctrl+d: expected scroll=%d, got %d", m.Height/2, m.Scroll)
		}
	})

	t.Run("ctrl+u scrolls up half page", func(t *testing.T) {
		t.Parallel()
		m := newPanel()
		m.Scroll = 10
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		if m.Scroll != 10-m.Height/2 {
			t.Fatalf("ctrl+u: expected scroll=%d, got %d", 10-m.Height/2, m.Scroll)
		}
	})
}

func makeFileStat(n int) []domain.PreviewFileStat {
	out := make([]domain.PreviewFileStat, n)
	for i := range out {
		out[i] = domain.PreviewFileStat{Path: fmt.Sprintf("internal/pkg/file_%d.go", i), Additions: i * 10, Deletions: i}
	}
	return out
}
