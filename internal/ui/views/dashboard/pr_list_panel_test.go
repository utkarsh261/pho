package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
)

func TestPRListPanelRenderTabsAndRows(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	m.SetTabSnapshot(domain.TabMyPRs, []domain.PullRequestSummary{
		makePR(1, "Fix login", "feature/login"),
		makePR(2, "Add tests", "feature/tests"),
		makePR(3, "Refine UI", "feature/ui"),
	}, 3, false)
	m.SetTabSnapshot(domain.TabNeedsReview, []domain.PullRequestSummary{
		makePR(10, "Needs review", "review/me"),
	}, 1, false)
	m.SetTabSnapshot(domain.TabInvolving, nil, 0, false)
	m.SetTabSnapshot(domain.TabRecent, nil, 0, false)
	m.SetActiveTab(domain.TabMyPRs)
	m.SetRect(80, 12)

	view := m.View()
	if !strings.Contains(view, "My PRs(3)") || !strings.Contains(view, "Needs Review(1)") {
		t.Fatalf("expected tab bar counts, got %q", view)
	}
	if strings.Count(view, "#1") != 1 || strings.Count(view, "#2") != 1 || strings.Count(view, "#3") != 1 {
		t.Fatalf("expected three PR rows, got %q", view)
	}
	if !strings.Contains(view, "feature/login") || !strings.Contains(view, "feature/tests") {
		t.Fatalf("expected branch rows, got %q", view)
	}
}

func TestPRListPanelTabSwitchIntent(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	m.SetTabSnapshot(domain.TabMyPRs, []domain.PullRequestSummary{makePR(1, "One", "branch")}, 1, false)
	m.SetTabSnapshot(domain.TabNeedsReview, []domain.PullRequestSummary{makePR(2, "Two", "branch")}, 1, false)
	m.SetActiveTab(domain.TabMyPRs)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("expected tab switch command")
	}
	msg := cmd()
	chg, ok := msg.(ChangeTabMsg)
	if !ok {
		t.Fatalf("expected ChangeTabMsg, got %T", msg)
	}
	if chg.Tab != domain.TabNeedsReview {
		t.Fatalf("expected next tab needs_review, got %s", chg.Tab)
	}
}

func TestPRListPanelTruncationFooter(t *testing.T) {
	t.Parallel()

	m := NewPRListPanelModel()
	prs := make([]domain.PullRequestSummary, 0, 100)
	for i := 0; i < 100; i++ {
		prs = append(prs, makePR(i+1, fmt.Sprintf("PR %d", i+1), "branch"))
	}
	m.SetTabSnapshot(domain.TabMyPRs, prs, 234, true)
	m.SetActiveTab(domain.TabMyPRs)
	m.SetRect(60, 12)

	view := m.View()
	if !strings.Contains(view, "Showing 100 of 234 open PRs") {
		t.Fatalf("expected truncation footer, got %q", view)
	}
}

func makePR(number int, title, branch string) domain.PullRequestSummary {
	return domain.PullRequestSummary{
		Repo:           "org/repo",
		Number:         number,
		Title:          title,
		Author:         "alice",
		State:          domain.PRStateOpen,
		CIStatus:       domain.CIStatusSuccess,
		ReviewDecision: domain.ReviewDecisionApproved,
		HeadRefName:    branch,
		CreatedAt:      time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 9, 13, 0, 0, 0, time.UTC),
	}
}
