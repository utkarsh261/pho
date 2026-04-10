package dashboard

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
)

const timeLayout = "2006-01-02 15:04 MST"

var dashboardTabOrder = []domain.DashboardTab{
	domain.TabMyPRs,
	domain.TabNeedsReview,
	domain.TabInvolving,
	domain.TabRecent,
}

type SelectRepoMsg struct {
	Index int
	Repo  domain.Repository
}

type SelectPRMsg struct {
	Tab     domain.DashboardTab
	Index   int
	Repo    string
	Number  int
	Summary domain.PullRequestSummary
}

type ChangeTabMsg struct {
	Tab domain.DashboardTab
}

type PreviewFetchMsg struct {
	Repo       string
	Number     int
	Generation int
}

type PreviewLoadedMsg struct {
	Repo    string
	Number  int
	Preview domain.PRPreviewSnapshot
}

func tabLabel(tab domain.DashboardTab) string {
	switch tab {
	case domain.TabMyPRs:
		return "My PRs"
	case domain.TabNeedsReview:
		return "Needs Review"
	case domain.TabInvolving:
		return "Involving"
	case domain.TabRecent:
		return "Recent"
	default:
		return string(tab)
	}
}

func nextTab(tab domain.DashboardTab, delta int) domain.DashboardTab {
	if len(dashboardTabOrder) == 0 {
		return tab
	}
	idx := indexOfTab(tab)
	if idx < 0 {
		idx = 0
	}
	n := (idx + delta) % len(dashboardTabOrder)
	if n < 0 {
		n += len(dashboardTabOrder)
	}
	return dashboardTabOrder[n]
}

func indexOfTab(tab domain.DashboardTab) int {
	for i, candidate := range dashboardTabOrder {
		if candidate == tab {
			return i
		}
	}
	return -1
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return t.In(t.Location()).Format(timeLayout)
}

func truncateText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	return string(runes[:width-3]) + "..."
}

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

func fitLine(s string, width int) string {
	return padRight(truncateText(s, width), width)
}

func renderBlock(lines []string, width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out = append(out, fitLine(lines[i], width))
			continue
		}
		out = append(out, strings.Repeat(" ", width))
	}
	return strings.Join(out, "\n")
}

func joinVisible(lines []string, sep string) string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, sep)
}

func repoSortLess(a, b domain.Repository) bool {
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	ai := a.LastActivityAt
	bi := b.LastActivityAt
	switch {
	case ai == nil && bi != nil:
		return false
	case ai != nil && bi == nil:
		return true
	case ai != nil && bi != nil && !ai.Equal(*bi):
		return ai.After(*bi)
	}
	return strings.ToLower(a.FullName) < strings.ToLower(b.FullName)
}

func sortRepos(repos []domain.Repository) {
	sort.SliceStable(repos, func(i, j int) bool {
		return repoSortLess(repos[i], repos[j])
	})
}

func makeDerivedPreview(summary domain.PullRequestSummary) domain.PRPreviewSnapshot {
	return domain.PRPreviewSnapshot{
		Repo:           summary.Repo,
		Number:         summary.Number,
		Title:          summary.Title,
		Author:         summary.Author,
		State:          summary.State,
		IsDraft:        summary.IsDraft,
		CIStatus:       summary.CIStatus,
		ReviewDecision: summary.ReviewDecision,
		CreatedAt:      summary.CreatedAt,
		UpdatedAt:      summary.UpdatedAt,
	}
}

func ciIcon(status domain.CIStatus) string {
	switch status {
	case domain.CIStatusSuccess:
		return "✓"
	case domain.CIStatusFailure, domain.CIStatusError:
		return "!"
	case domain.CIStatusPending:
		return "…"
	default:
		return "·"
	}
}

func reviewIcon(decision domain.ReviewDecision, isDraft bool) string {
	if isDraft {
		return "D"
	}
	switch decision {
	case domain.ReviewDecisionApproved:
		return "✓"
	case domain.ReviewDecisionChangesRequested:
		return "!"
	case domain.ReviewDecisionReviewRequired:
		return "?"
	default:
		return "·"
	}
}

func reviewLabel(decision domain.ReviewDecision, isDraft bool) string {
	if isDraft {
		return "draft"
	}
	switch decision {
	case domain.ReviewDecisionApproved:
		return "approved"
	case domain.ReviewDecisionChangesRequested:
		return "changes requested"
	case domain.ReviewDecisionReviewRequired:
		return "review required"
	default:
		return "none"
	}
}

func ciLabel(status domain.CIStatus) string {
	switch status {
	case domain.CIStatusSuccess:
		return "success"
	case domain.CIStatusFailure:
		return "failure"
	case domain.CIStatusError:
		return "error"
	case domain.CIStatusPending:
		return "pending"
	default:
		return "n/a"
	}
}

func stateLabel(state domain.PRState, isDraft bool) string {
	if isDraft {
		return "draft"
	}
	return strings.ToLower(string(state))
}

func activityLabel(kind domain.ActivityKind) string {
	switch kind {
	case domain.ActivityKindCommit:
		return "commit"
	case domain.ActivityKindComment:
		return "comment"
	case domain.ActivityKindReview:
		return "review"
	case domain.ActivityKindReviewRequested:
		return "review requested"
	case domain.ActivityKindMerged:
		return "merged"
	case domain.ActivityKindClosed:
		return "closed"
	case domain.ActivityKindReopened:
		return "reopened"
	case domain.ActivityKindLabeled:
		return "labeled"
	default:
		return "activity"
	}
}

func selectRepoCmd(index int, repo domain.Repository) tea.Cmd {
	return func() tea.Msg {
		return SelectRepoMsg{Index: index, Repo: repo}
	}
}

func selectPRCmd(tab domain.DashboardTab, index int, summary domain.PullRequestSummary) tea.Cmd {
	return func() tea.Msg {
		return SelectPRMsg{
			Tab:     tab,
			Index:   index,
			Repo:    summary.Repo,
			Number:  summary.Number,
			Summary: summary,
		}
	}
}

func changeTabCmd(tab domain.DashboardTab) tea.Cmd {
	return func() tea.Msg {
		return ChangeTabMsg{Tab: tab}
	}
}

func previewFetchCmd(repo string, number, generation int) tea.Cmd {
	return func() tea.Msg {
		return PreviewFetchMsg{Repo: repo, Number: number, Generation: generation}
	}
}

func previewLoadedCmd(repo string, number int, preview domain.PRPreviewSnapshot) tea.Cmd {
	return func() tea.Msg {
		return PreviewLoadedMsg{Repo: repo, Number: number, Preview: preview}
	}
}
