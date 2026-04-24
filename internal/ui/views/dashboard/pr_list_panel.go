package dashboard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

type tabSnapshot struct {
	PRs        []domain.PullRequestSummary
	TotalCount int
	Truncated  bool
}

type PRListPanelModel struct {
	Tabs    map[domain.DashboardTab]tabSnapshot
	Active  domain.DashboardTab
	Cursor  int
	Scroll  int
	Width   int
	Height  int
	theme   *theme.Theme
	lastKey string
}

func NewPRListPanelModel() *PRListPanelModel {
	return &PRListPanelModel{
		Tabs:   make(map[domain.DashboardTab]tabSnapshot),
		Active: domain.TabMyPRs,
	}
}

func (m *PRListPanelModel) Init() tea.Cmd { return nil }

func (m *PRListPanelModel) SetRect(width, height int) {
	m.Width = width
	m.Height = height
	m.ensureVisible()
}

func (m *PRListPanelModel) SetTheme(th *theme.Theme) {
	m.theme = th
}

func (m *PRListPanelModel) SetTabSnapshot(tab domain.DashboardTab, prs []domain.PullRequestSummary, totalCount int, truncated bool) {
	if m.Tabs == nil {
		m.Tabs = make(map[domain.DashboardTab]tabSnapshot)
	}
	m.Tabs[tab] = tabSnapshot{
		PRs:        append([]domain.PullRequestSummary(nil), prs...),
		TotalCount: totalCount,
		Truncated:  truncated,
	}
	m.ensureVisible()
}

func (m *PRListPanelModel) SetActiveTab(tab domain.DashboardTab) {
	m.Active = tab
	m.Cursor = 0
	m.Scroll = 0
	m.ensureVisible()
}

func (m *PRListPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetRect(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		prevKey := m.lastKey
		if msg.String() != "g" {
			m.lastKey = ""
		}
		switch msg.String() {
		case "j", "down":
			if m.moveCursor(1) {
				return m, m.selectCurrentCmd()
			}
		case "k", "up":
			if m.moveCursor(-1) {
				return m, m.selectCurrentCmd()
			}
		case "g":
			if prevKey == "g" {
				if m.moveCursorTo(0) {
					return m, m.selectCurrentCmd()
				}
			} else {
				m.lastKey = "g"
			}
			return m, nil
		case "G":
			prs := m.currentPRs()
			if m.moveCursorTo(len(prs) - 1) {
				return m, m.selectCurrentCmd()
			}
			return m, nil
		case "ctrl+d":
			if m.moveCursor(m.visibleItemCount() / 2) {
				return m, m.selectCurrentCmd()
			}
			return m, nil
		case "ctrl+u":
			if m.moveCursor(-(m.visibleItemCount() / 2)) {
				return m, m.selectCurrentCmd()
			}
			return m, nil
		case "h", "left":
			next := nextTab(m.Active, -1)
			if next != m.Active {
				m.Active = next
				m.Cursor = 0
				m.Scroll = 0
				m.ensureVisible()
				return m, changeTabCmd(next)
			}
		case "l", "right":
			next := nextTab(m.Active, 1)
			if next != m.Active {
				m.Active = next
				m.Cursor = 0
				m.Scroll = 0
				m.ensureVisible()
				return m, changeTabCmd(next)
			}
		case "enter":
			if cmd := m.selectCurrentCmd(); cmd != nil {
				return m, cmd
			}
		}
	case SelectRepoMsg:
		return m, nil
	}
	return m, nil
}

func (m *PRListPanelModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}
	header := "▸ PRs"
	if m.theme != nil {
		header = m.theme.Header.Width(m.Width).Render(header)
	} else {
		header = fitLine(header, m.Width)
	}
	lines := []string{
		header,
		fitLine("", m.Width),
		fitLine(m.renderTabBarThemed(), m.Width),
		fitLine("", m.Width),
	}
	rows := m.visibleRows()
	if len(rows) == 0 {
		empty := "No PRs in this tab"
		if m.theme != nil {
			empty = m.theme.MutedTxt.Render(empty)
		}
		lines = append(lines, fitLine(empty, m.Width))
		lines = append(lines, fitLine("", m.Width))
		return renderBlock(lines, m.Width, m.Height)
	}
	for i, row := range rows {
		lines = append(lines, fitLine(row.line1, m.Width))
		lines = append(lines, fitLine(row.line2, m.Width))
		if i < len(rows)-1 {
			lines = append(lines, fitLine("", m.Width))
		}
	}
	if footer := m.footerLine(); footer != "" {
		lines = append(lines, fitLine("", m.Width))
		lines = append(lines, fitLine(footer, m.Width))
	} else {
		lines = append(lines, fitLine("", m.Width))
	}
	return renderBlock(lines, m.Width, m.Height)
}

func (m *PRListPanelModel) moveCursor(delta int) bool {
	prs := m.currentPRs()
	if len(prs) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return false
	}
	next := m.Cursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(prs) {
		next = len(prs) - 1
	}
	changed := next != m.Cursor
	m.Cursor = next
	m.ensureVisible()
	return changed
}

func (m *PRListPanelModel) moveCursorTo(pos int) bool {
	prs := m.currentPRs()
	if len(prs) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return false
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(prs) {
		pos = len(prs) - 1
	}
	changed := pos != m.Cursor
	m.Cursor = pos
	m.ensureVisible()
	return changed
}

func (m *PRListPanelModel) currentSnapshot() tabSnapshot {
	if m.Tabs == nil {
		return tabSnapshot{}
	}
	snap, ok := m.Tabs[m.Active]
	if !ok {
		return tabSnapshot{}
	}
	return snap
}

func (m *PRListPanelModel) currentPRs() []domain.PullRequestSummary {
	return m.currentSnapshot().PRs
}

func (m *PRListPanelModel) currentSelected() (domain.PullRequestSummary, bool) {
	prs := m.currentPRs()
	if len(prs) == 0 || m.Cursor < 0 || m.Cursor >= len(prs) {
		return domain.PullRequestSummary{}, false
	}
	return prs[m.Cursor], true
}

func (m *PRListPanelModel) selectCurrentCmd() tea.Cmd {
	pr, ok := m.currentSelected()
	if !ok {
		return nil
	}
	return selectPRCmd(m.Active, m.Cursor, pr)
}

func (m *PRListPanelModel) visibleRows() []prRow {
	prs := m.currentPRs()
	if len(prs) == 0 || m.Width <= 0 || m.Height <= 0 {
		return nil
	}
	maxRows := m.visibleItemCount()
	if maxRows <= 0 {
		return nil
	}
	start := m.Scroll
	if start < 0 {
		start = 0
	}
	if start > len(prs) {
		start = len(prs)
	}
	end := start + maxRows
	if end > len(prs) {
		end = len(prs)
	}
	rows := make([]prRow, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, m.renderRow(prs[i], i))
	}
	return rows
}

func (m *PRListPanelModel) renderRow(pr domain.PullRequestSummary, index int) prRow {
	selected := index == m.Cursor
	bar := " "
	if selected {
		bar = "▌"
	}

	meta := fmt.Sprintf("%s %s", m.ciIconStyled(pr.CIStatus), m.reviewIconStyled(pr.ReviewDecision, pr.IsDraft))
	prefix := m.prNumberStyled(pr.Number)
	minGap := 2
	metaW := lipgloss.Width(meta)
	titleMax := m.Width - lipgloss.Width(bar) - lipgloss.Width(prefix) - 1 - metaW - minGap
	if titleMax < 1 {
		titleMax = 1
	}
	title := truncateText(pr.Title, titleMax)
	if selected && m.theme != nil {
		title = m.theme.Bold.Render(title)
	}
	line1 := fmt.Sprintf("%s%s %s  %s", bar, prefix, title, meta)

	if selected && m.theme != nil {
		line1 = m.theme.SelectedRow.Render(line1)
	}

	branch := pr.HeadRefName
	if branch == "" {
		branch = pr.BaseRefName
	}
	line2 := strings.TrimRight(bar+" "+branch, " ")
	if selected && m.theme != nil {
		line2 = m.theme.SelectedRow.Render(m.theme.MutedTxt.Render(line2))
	}
	return prRow{line1: line1, line2: line2}
}

func (m *PRListPanelModel) renderTabBar() string {
	parts := make([]string, 0, len(dashboardTabOrder))
	for _, tab := range dashboardTabOrder {
		count := len(m.currentSnapshotFor(tab).PRs)
		label := fmt.Sprintf("%s(%d)", tabLabel(tab), count)
		if tab == m.Active {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " | ")
}

func (m *PRListPanelModel) renderTabBarThemed() string {
	if m.theme == nil {
		return m.renderTabBar()
	}
	parts := make([]string, 0, len(dashboardTabOrder))
	for i, tab := range dashboardTabOrder {
		count := len(m.currentSnapshotFor(tab).PRs)
		label := fmt.Sprintf("%s(%d)", tabLabel(tab), count)
		if tab == m.Active {
			parts = append(parts, m.theme.TabActive.Render(label))
		} else {
			parts = append(parts, m.theme.TabInactive.Render(label))
		}
		if i < len(dashboardTabOrder)-1 {
			parts = append(parts, " ")
		}
	}
	return strings.Join(parts, "")
}

func (m *PRListPanelModel) ciIconStyled(status domain.CIStatus) string {
	icon := ciIcon(status)
	if m.theme == nil {
		return icon
	}
	switch status {
	case domain.CIStatusSuccess:
		return m.theme.CISuccess.Render(icon)
	case domain.CIStatusFailure, domain.CIStatusError:
		return m.theme.CIFailure.Render(icon)
	case domain.CIStatusPending:
		return m.theme.CIPending.Render(icon)
	default:
		return m.theme.CIMuted.Render(icon)
	}
}

func (m *PRListPanelModel) reviewIconStyled(decision domain.ReviewDecision, isDraft bool) string {
	icon := reviewIcon(decision, isDraft)
	if m.theme == nil {
		return icon
	}
	if isDraft {
		return m.theme.ReviewDraft.Render(icon)
	}
	switch decision {
	case domain.ReviewDecisionApproved:
		return m.theme.ReviewApproved.Render(icon)
	case domain.ReviewDecisionChangesRequested:
		return m.theme.ReviewChanges.Render(icon)
	case domain.ReviewDecisionReviewRequired:
		return m.theme.ReviewRequired.Render(icon)
	default:
		return m.theme.ReviewMuted.Render(icon)
	}
}

func (m *PRListPanelModel) prNumberStyled(number int) string {
	s := fmt.Sprintf("#%d ", number)
	if m.theme != nil {
		return m.theme.Number.Render(s)
	}
	return s
}

func (m *PRListPanelModel) currentSnapshotFor(tab domain.DashboardTab) tabSnapshot {
	if m.Tabs == nil {
		return tabSnapshot{}
	}
	return m.Tabs[tab]
}

func (m *PRListPanelModel) footerLine() string {
	snap := m.currentSnapshot()
	if !snap.Truncated {
		return ""
	}
	total := snap.TotalCount
	if total < len(snap.PRs) {
		total = len(snap.PRs)
	}
	return fmt.Sprintf("Showing %d of %d open PRs", len(snap.PRs), total)
}

func (m *PRListPanelModel) visibleItemCount() int {
	if m.Height <= 6 {
		return 0
	}
	available := m.Height - 6
	if available < 2 {
		return 0
	}
	return (available + 1) / 3
}

func (m *PRListPanelModel) ensureVisible() {
	prs := m.currentPRs()
	if len(prs) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(prs) {
		m.Cursor = len(prs) - 1
	}
	visible := m.visibleItemCount()
	if visible <= 0 {
		m.Scroll = 0
		return
	}
	if m.Cursor < m.Scroll {
		m.Scroll = m.Cursor
	}
	if m.Cursor >= m.Scroll+visible {
		m.Scroll = m.Cursor - visible + 1
	}
	maxScroll := len(prs) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.Scroll > maxScroll {
		m.Scroll = maxScroll
	}
	if m.Scroll < 0 {
		m.Scroll = 0
	}
}

type prRow struct {
	line1 string
	line2 string
}
