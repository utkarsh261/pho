package dashboard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
)

type tabSnapshot struct {
	PRs        []domain.PullRequestSummary
	TotalCount int
	Truncated  bool
}

type PRListPanelModel struct {
	Tabs   map[domain.DashboardTab]tabSnapshot
	Active domain.DashboardTab
	Cursor int
	Scroll int
	Width  int
	Height int
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
		switch msg.String() {
		case "j", "down":
			if m.moveCursor(1) {
				return m, m.selectCurrentCmd()
			}
		case "k", "up":
			if m.moveCursor(-1) {
				return m, m.selectCurrentCmd()
			}
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
	lines := []string{fitLine(m.renderTabBar(), m.Width)}
	rows := m.visibleRows()
	for _, row := range rows {
		lines = append(lines, fitLine(row.line1, m.Width))
		lines = append(lines, fitLine(row.line2, m.Width))
	}
	if footer := m.footerLine(); footer != "" {
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
	rowWidth := m.Width - 1
	if rowWidth < 0 {
		rowWidth = 0
	}

	meta := fmt.Sprintf("%s %s", ciIcon(pr.CIStatus), reviewIcon(pr.ReviewDecision, pr.IsDraft))
	prefix := fmt.Sprintf("#%d ", pr.Number)
	leftWidth := rowWidth - len([]rune(meta)) - len([]rune(prefix)) - 1
	if leftWidth < 0 {
		leftWidth = 0
	}
	title := truncateText(pr.Title, leftWidth)
	line1 := strings.TrimRight(fmt.Sprintf("%s%s%s", bar, prefix, title), " ")
	if rowWidth > len([]rune(line1)) {
		padding := rowWidth - len([]rune(line1)) - len([]rune(meta))
		if padding < 1 {
			padding = 1
		}
		line1 += strings.Repeat(" ", padding) + meta
	} else {
		line1 = truncateText(line1, rowWidth)
	}

	branch := pr.HeadRefName
	if branch == "" {
		branch = pr.BaseRefName
	}
	line2 := strings.TrimRight(bar+" "+branch, " ")
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
	if m.Height <= 2 {
		return 0
	}
	available := m.Height - 2
	if available < 2 {
		return 0
	}
	return available / 2
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
