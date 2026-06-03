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

	author := m.Summary.Author
	if author == "" {
		author = "unknown"
	}

	state := "OPEN"
	if m.Detail != nil {
		state = string(m.Detail.State)
	}

	var authorStr string
	var stateStr string
	mergeSuffix := ""
	if m.Detail != nil && m.Detail.Mergeable != "" && m.Detail.Mergeable != "MERGEABLE" && m.Detail.Mergeable != "UNKNOWN" {
		mergeSuffix = " · " + humanizeMergeState(m.Detail.MergeState)
	}
	if m.theme != nil {
		authorStr = m.theme.PrimaryTxt.Render(author)
		switch state {
		case "OPEN":
			stateStr = lipgloss.NewStyle().Foreground(m.theme.Secondary).Render("OPEN" + mergeSuffix)
		case "MERGED":
			stateStr = m.theme.PrimaryTxt.Render("MERGED" + mergeSuffix)
		case "CLOSED":
			stateStr = m.theme.ReviewChanges.Render("CLOSED" + mergeSuffix)
		default:
			stateStr = m.theme.ReviewRequired.Render(state + mergeSuffix)
		}
		// Override color for conflicting state.
		if m.Detail != nil && m.Detail.Mergeable == "CONFLICTING" {
			stateStr = m.theme.ReviewChanges.Render(state + mergeSuffix)
		}
	} else {
		authorStr = author
		stateStr = state + mergeSuffix
	}

	metaStr := authorStr + " " + stateStr
	metaLen := lipgloss.Width(metaStr)

	hints := "[o: Browser | Esc: Back]"
	if m.Width < 80 {
		hints = ""
	}
	hintsLen := lipgloss.Width(hints)

	innerW := max(m.Width-2, 1)

	reservedSpace := metaLen
	if hintsLen > 0 {
		reservedSpace += 1 + hintsLen
	}

	baseTitle := fmt.Sprintf("#%d %s", m.Summary.Number, m.Summary.Title)
	if m.Summary.Title == "" {
		baseTitle = fmt.Sprintf("Pull Request #%d", m.Summary.Number)
	}

	titleBudget := innerW - reservedSpace - 2 // -2 just for padding
	if titleBudget < 5 {
		titleBudget = 5
	}

	truncTitle := baseTitle
	if lipgloss.Width(baseTitle) > titleBudget {
		truncTitle = truncateText(baseTitle, titleBudget)
	}

	leftPart := truncTitle + " " + metaStr

	var finalHeader string
	if hintsLen > 0 {
		leftWidth := lipgloss.Width(leftPart)
		padWidth := max(innerW-leftWidth-hintsLen, 1)
		finalHeader = leftPart + strings.Repeat(" ", padWidth) + hints
	} else {
		finalHeader = leftPart + strings.Repeat(" ", max(0, innerW-lipgloss.Width(leftPart)))
	}

	var content string
	var borderColor lipgloss.Color
	if m.theme != nil {
		content = m.theme.Header.Width(innerW).Render(finalHeader)
		borderColor = m.theme.Border
	} else {
		content = lipgloss.NewStyle().Width(innerW).Render(finalHeader)
		borderColor = theme.Default().Border
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Width(innerW).
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

	hints := "[o: Browser | Esc: Back]"
	if m.Width < 80 {
		hints = ""
	}
	hintsLen := lipgloss.Width(hints)

	innerW := max(m.Width-2, 1)

	title := fmt.Sprintf("Commit %s — %s", sha, m.Commit.MessageHeadline)
	meta := fmt.Sprintf("%s · %s", author, relTime)
	metaRendered := th.MutedTxt.Render(meta)

	leftPart := title + "  " + metaRendered
	leftWidth := lipgloss.Width(leftPart)

	var finalHeader string
	if hintsLen > 0 {
		padWidth := max(innerW-leftWidth-hintsLen, 1)
		finalHeader = leftPart + strings.Repeat(" ", padWidth) + hints
	} else {
		finalHeader = leftPart + strings.Repeat(" ", max(0, innerW-leftWidth))
	}

	var content string
	content = th.Header.Width(innerW).Render(finalHeader)

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(th.Border).
		Width(innerW).
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
	tabsStr := m.renderSectionTabs()
	tabsStr = " " + tabsStr

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
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH).
		Render(contentStr)

	return lipgloss.JoinVertical(lipgloss.Left, headBox, bodyBox)
}

// renderSectionTabs builds the "1:Desc 2:Diff 3:Comments" indicator string.
// Active tab is highlighted.
func (m *PRDetailModel) renderSectionTabs() string {
	th := m.theme
	if th == nil {
		th = theme.Default()
	}

	if m.CommitMode {
		return th.TabActive.Render("● Diff")
	}

	type tabDef struct {
		num  ContentTab
		key  string
		name string
	}
	tabs := []tabDef{
		{TabDescription, "1", "Desc"},
		{TabDiff, "2", "Diff"},
		{TabComments, "3", "Comments"},
		{TabCommits, "4", "Commits"},
	}

	parts := make([]string, len(tabs))
	for i, td := range tabs {
		var rendered string
		if m.activeTab == td.num {
			rendered = th.TabActive.Render("● " + td.name)
		} else {
			rendered = th.TabInactive.Render(td.key + ":" + td.name)
		}
		parts[i] = rendered
	}
	return strings.Join(parts, " ")
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
