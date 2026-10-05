package prdetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func (m *PRDetailModel) View() string {
	defer m.log().Timer("render pr detail")()
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}

	headerRow := m.renderHeader()

	bodyH := m.effectiveBodyH()

	var body string
	if m.compose.active && m.compose.status == composeStatusIdle && m.cachedBody != "" &&
		m.cachedBodyWidth == m.Width && m.cachedBodyHeight == bodyH {
		// Compose is open and nothing in the body has changed — reuse last render
		// so that text input navigation (arrow keys, backspace, etc.) is instant.
		body = m.cachedBody
	} else {
		if m.Width >= MinWidthForSidebar {
			rightWidth := max(m.Width-LeftPanelWidth-2, 10)
			leftView := m.leftPanel.View(bodyH, m.spinner.View())
			rightView := m.renderRightViewport(rightWidth, bodyH)
			body = lipgloss.JoinHorizontal(lipgloss.Top, leftView, "  ", rightView)
		} else {
			body = m.renderNarrowBody(m.Width, bodyH)
		}
		m.cachedBody = body
		m.cachedBodyWidth = m.Width
		m.cachedBodyHeight = bodyH
	}

	if m.compose.active {
		return headerRow + "\n" + body + "\n" + m.compose.View(m.Width)
	}
	return headerRow + "\n" + body
}

func (m *PRDetailModel) renderHeader() string {
	if m.CommitMode {
		return m.renderCommitHeader()
	}
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	innerW := max(m.Width-2, 1)
	contentW := max(innerW-2, 1) // 1-col padding each side

	// Line 1: "#9 Title" on the left, key hints on the right.
	hints := ""
	if m.Width >= 80 {
		hints = th.RenderHints("o: Browser | Esc: Back")
	}
	baseTitle := fmt.Sprintf("#%d %s", m.Summary.Number, m.Summary.Title)
	if m.Summary.Title == "" {
		baseTitle = fmt.Sprintf("Pull Request #%d", m.Summary.Number)
	}
	titleBudget := max(contentW-lipgloss.Width(hints)-2, 5)
	line1 := th.Header.Render(truncateText(baseTitle, titleBudget))
	if hints != "" {
		line1 += strings.Repeat(" ", max(contentW-lipgloss.Width(line1)-lipgloss.Width(hints), 1)) + hints
	}

	// Line 2: author, state (+ merge state, unresolved count), then reviewers.
	author := m.Summary.Author
	if author == "" {
		author = "unknown"
	}
	state := "OPEN"
	if m.Detail != nil {
		state = string(m.Detail.State)
	}
	mergeSuffix := ""
	if m.Detail != nil && m.Detail.Mergeable != "" && m.Detail.Mergeable != "MERGEABLE" && m.Detail.Mergeable != "UNKNOWN" {
		mergeSuffix = " · " + humanizeMergeState(m.Detail.MergeState)
	}
	if m.Detail != nil && m.Width >= 80 {
		if n := m.unresolvedThreadCount(); n > 0 {
			mergeSuffix += fmt.Sprintf(" · %d unresolved", n)
		}
	}
	var stateStr string
	switch state {
	case "OPEN":
		stateStr = lipgloss.NewStyle().Foreground(th.Secondary).Render("OPEN" + mergeSuffix)
	case "MERGED":
		stateStr = th.PrimaryTxt.Render("MERGED" + mergeSuffix)
	case "CLOSED":
		stateStr = th.ReviewChanges.Render("CLOSED" + mergeSuffix)
	default:
		stateStr = th.ReviewRequired.Render(state + mergeSuffix)
	}
	if m.Detail != nil && m.Detail.Mergeable == "CONFLICTING" {
		stateStr = th.ReviewChanges.Render(state + mergeSuffix)
	}
	line2 := truncateText(th.PrimaryTxt.Render(author)+" "+stateStr, contentW)
	if strip := m.renderHeaderReviewers(contentW - lipgloss.Width(line2) - 5); strip != "" {
		line2 += th.FaintTxt.Render("  │  ") + strip
	}

	content := lipgloss.NewStyle().Padding(0, 1).Width(innerW).Render(line1 + "\n" + line2)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		Render(content)
}

func (m *PRDetailModel) renderCommitHeader() string {
	sha := m.Commit.SHA
	if len(sha) > 7 {
		sha = sha[:7]
	}

	author := m.Commit.AuthorLogin
	if author == "" {
		author = m.Commit.AuthorName
	}
	relTime := relativeTime(m.Commit.CommittedAt)

	th := m.theme
	if th == nil {
		th = theme.Default()
	}

	hints := ""
	if m.Width >= 80 {
		hints = th.RenderHints("o: Browser | Esc: Back")
	}
	hintsLen := lipgloss.Width(hints)

	innerW := max(m.Width-2, 1)
	contentW := max(innerW-2, 1) // 1-col padding each side, matching the PR header

	title := fmt.Sprintf("Commit %s — %s", sha, m.Commit.MessageHeadline)
	meta := fmt.Sprintf("%s · %s", author, relTime)
	metaRendered := th.MutedTxt.Render(meta)

	leftPart := title + "  " + metaRendered
	if budget := contentW - hintsLen - 1; lipgloss.Width(leftPart) > budget {
		leftPart = truncateText(leftPart, max(budget, 5))
	}
	leftWidth := lipgloss.Width(leftPart)

	var finalHeader string
	if hintsLen > 0 {
		padWidth := max(contentW-leftWidth-hintsLen, 1)
		finalHeader = leftPart + strings.Repeat(" ", padWidth) + hints
	} else {
		finalHeader = leftPart + strings.Repeat(" ", max(0, contentW-leftWidth))
	}

	content := lipgloss.NewStyle().Padding(0, 1).Width(innerW).Render(th.Header.Render(finalHeader))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		Render(content)
}

func (m *PRDetailModel) renderRightViewport(width, height int) string {
	innerH := max(height-4, 1)
	innerW := max(width-2, 1)
	contentW := max(innerW-2, 1)
	contentH := max(innerH-2, 1)

	scroll := clamp(m.ContentScroll, 0, max(0, m.maxContentScroll()))

	// Render content based on active tab.
	var lines []string
	switch m.activeTab {
	case TabDescription:
		lines = m.renderDescriptionTab(scroll, contentH, contentW)
	case TabDiff:
		lines = m.renderDiffTab(scroll, contentH, contentW)
	case TabComments:
		lines = m.renderCommentsTab(scroll, contentH, contentW)
	case TabCommits:
		lines = m.renderCommitsTab(scroll, contentH, innerW-1)
	}

	// Apply left-padding (1 space) to each content line.
	for i, l := range lines {
		lines[i] = " " + l
	}
	contentStr := renderBlock(lines, innerW, contentH)

	// Build tab indicators based on active tab.
	tabsStr := " " + m.renderSectionTabs()
	if lipgloss.Width(tabsStr) > innerW {
		// Narrow panel: drop the padding so all four numbered tabs still fit.
		tabsStr = truncateText(m.renderSectionTabsCompact(), innerW)
	}

	var borderColor lipgloss.Color
	if m.theme != nil {
		borderColor = m.theme.Border
	} else {
		borderColor = theme.Default().Border
	}
	if m.leftPanel.Focus == FocusContent {
		if m.theme != nil {
			borderColor = m.theme.Primary
		} else {
			borderColor = theme.Default().Primary
		}
	}

	headBox := lipgloss.NewStyle().
		Border(panelHeadBorder).
		BorderForeground(borderColor).
		Width(innerW).
		Render(tabsStr)

	bodyBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH).
		Render(contentStr)

	return lipgloss.JoinVertical(lipgloss.Left, headBox, bodyBox)
}

// renderSectionTabs builds the "● Desc 2:Diff 3:Comments 4:Commits" indicator.
// The numbers double as key hints; the active tab is highlighted.
func (m *PRDetailModel) renderSectionTabs() string {
	return m.sectionTabs(1)
}

// renderSectionTabsCompact is the narrow-terminal fallback: the same numbered
// labels with no padding, single-space separated.
func (m *PRDetailModel) renderSectionTabsCompact() string {
	return m.sectionTabs(0)
}

func (m *PRDetailModel) sectionTabs(gap int) string {
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	active, inactive := th.SectionTabActive, th.TabInactive
	if gap == 0 {
		active, inactive = active.Padding(0), inactive.Padding(0)
	}

	if m.CommitMode {
		return active.Render("● Diff")
	}

	tabs := []struct {
		num  ContentTab
		key  string
		name string
	}{
		{TabDescription, "1", "Desc"},
		{TabDiff, "2", "Diff"},
		{TabComments, "3", "Comments"},
		{TabCommits, "4", "Commits"},
	}
	parts := make([]string, len(tabs))
	for i, td := range tabs {
		if m.activeTab == td.num {
			parts[i] = active.Render("● " + td.name)
		} else {
			parts[i] = inactive.Render(td.key + ":" + td.name)
		}
	}
	return strings.Join(parts, strings.Repeat(" ", max(gap, 1)))
}

// renderNarrowBody renders the body for terminals < 80 cols (no sidebar).
// Shows "N files changed" as the first line then the content viewport.
func (m *PRDetailModel) renderNarrowBody(width, height int) string {
	fileCount := 0
	if m.Diff != nil {
		fileCount = len(m.Diff.Files)
	} else if m.Detail != nil {
		fileCount = m.Detail.FileCount
	}

	var header string
	if m.Diff != nil {
		header = fmt.Sprintf("  %d files changed  +%d -%d",
			fileCount, m.Diff.Stats.TotalAdditions, m.Diff.Stats.TotalDeletions)
	} else {
		header = fmt.Sprintf("  %d files changed", fileCount)
	}
	if height <= 1 {
		return lipgloss.NewStyle().Width(width).Render(header)
	}
	top := lipgloss.NewStyle().Width(width).Render(header)
	body := m.renderRightViewport(width, height-1)
	return top + "\n" + body
}
