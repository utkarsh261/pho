package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/utk/git-term/internal/domain"
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
		"+120/-12 cmd/main.go",
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
