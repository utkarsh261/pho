package prdetail

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/ui/theme"
)

// commitsSectionRowCount returns the number of display rows for the Commits section.
func (m *PRDetailModel) commitsSectionRowCount() int {
	if m.commitsLoading {
		return 1
	}
	if len(m.commits) == 0 {
		return 1
	}
	// Each commit is 2 rows (headline + metadata) plus 1 blank row between.
	return len(m.commits)*3 - 1
}

// renderCommitsTab renders the Commits tab content.
func (m *PRDetailModel) renderCommitsTab(scroll, contentH, contentWidth int) []string {
	out := make([]string, contentH)
	cw := max(contentWidth, 1)

	if m.commitsLoading {
		msg := "Loading commits…"
		if m.theme != nil {
			msg = m.theme.MutedTxt.Render(msg)
		}
		centerStyle := lipgloss.NewStyle().Width(cw).Align(lipgloss.Center)
		out[0] = centerStyle.Render(msg)
		for i := 1; i < contentH; i++ {
			out[i] = ""
		}
		return out
	}

	if len(m.commits) == 0 {
		msg := "No commits"
		if m.theme != nil {
			msg = m.theme.MutedTxt.Render(msg)
		}
		centerStyle := lipgloss.NewStyle().Width(cw).Align(lipgloss.Center)
		out[0] = centerStyle.Render(msg)
		for i := 1; i < contentH; i++ {
			out[i] = ""
		}
		return out
	}

	localStart := scroll
	localEnd := scroll + contentH

	th := m.theme
	if th == nil {
		th = theme.Default()
	}

	outIdx := 0
	for i, c := range m.commits {
		rowStart := i * 3
		rowEnd := rowStart + 2
		if rowEnd <= localStart || rowStart >= localEnd {
			continue
		}

		isSelected := i == m.commitCursor

		// Line 1: SHA + message headline
		shortSHA := c.SHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		line1 := shortSHA + "  " + c.MessageHeadline
		line1 = truncateString(line1, cw-4)

		// Line 2: author (right-pad) + relative time
		author := c.AuthorLogin
		if author == "" {
			author = c.AuthorName
		}
		relTime := relativeTime(c.CommittedAt)
		line2 := author
		padding := cw - 4 - lipgloss.Width(line2) - lipgloss.Width(relTime)
		if padding > 0 {
			line2 += strings.Repeat(" ", padding)
		}
		line2 += relTime

		if isSelected {
			fullRow := line1 + "\n" + line2
			rendered := th.BoxSelected.Width(cw).Render(fullRow)
			parts := strings.Split(rendered, "\n")
			for pi, p := range parts {
				if outIdx >= contentH {
					break
				}
				globalRow := rowStart + pi - localStart
				if globalRow >= 0 && globalRow < contentH {
					out[globalRow] = p
				}
				outIdx++
			}
		} else {
			shaStyle := th.BoxPRNum
			authorStyle := th.BoxPRAuthor
			mutedStyle := lipgloss.NewStyle().Foreground(th.Muted)

			globalRow1 := rowStart - localStart
			if globalRow1 >= 0 && globalRow1 < contentH {
				out[globalRow1] = "  " + shaStyle.Render(shortSHA) + "  " + truncateString(c.MessageHeadline, cw-lipgloss.Width(shortSHA)-6)
			}
			globalRow2 := rowStart + 1 - localStart
			if globalRow2 >= 0 && globalRow2 < contentH {
				out[globalRow2] = "  " + authorStyle.Render(author) + strings.Repeat(" ", max(1, cw-4-lipgloss.Width(author)-lipgloss.Width(relTime))) + mutedStyle.Render(relTime)
			}
		}
	}

	// Fill remaining rows with blanks.
	for i := range out {
		if out[i] == "" {
			out[i] = strings.Repeat(" ", cw)
		}
	}
	return out
}

func (m *PRDetailModel) emitOpenCommitDetail() tea.Cmd {
	if m.commitCursor < 0 || m.commitCursor >= len(m.commits) {
		return nil
	}
	return func() tea.Msg {
		return OpenCommitDetail{
			Repo:   m.Repo,
			Commit: m.commits[m.commitCursor],
		}
	}
}

func (m *PRDetailModel) moveCommitCursor(delta int) {
	if len(m.commits) == 0 || m.commitsLoading {
		return
	}
	m.commitCursor += delta
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
	if m.commitCursor >= len(m.commits) {
		m.commitCursor = len(m.commits) - 1
	}
	// Each commit is 3 rows (2 content + 1 blank).
	cursorRow := m.commitCursor * 3
	vh := m.contentViewportHeight()
	if cursorRow < m.ContentScroll {
		m.ContentScroll = cursorRow
	} else if cursorRow >= m.ContentScroll+vh {
		m.ContentScroll = cursorRow - vh + 1
	}
	m.clampContentScroll()
}

func (m *PRDetailModel) emitCopyCommitSHA() tea.Cmd {
	if m.commitCursor < 0 || m.commitCursor >= len(m.commits) {
		return nil
	}
	sha := m.commits[m.commitCursor].SHA
	return func() tea.Msg {
		return CopyCommitSHA{SHA: sha}
	}
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
