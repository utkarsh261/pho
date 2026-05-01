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
// availW is the available content width (innerW - 1, accounting for the left-pad
// space that renderRightViewport adds to every line).
func (m *PRDetailModel) renderCommitsTab(scroll, contentH, availW int) []string {
	out := make([]string, contentH)
	cw := max(availW, 1)

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
		shaStyled := th.BoxPRNum.Render(shortSHA)
		shaW := lipgloss.Width(shaStyled)
		gap := "  "
		gapW := 2

		// Line 2: author (right-pad) + relative time
		author := c.AuthorLogin
		if author == "" {
			author = c.AuthorName
		}
		relTime := relativeTime(c.CommittedAt)

		if isSelected {
			line1 := shortSHA + gap + c.MessageHeadline
			line1 = truncateString(line1, cw)

			line2 := author
			padding := cw - lipgloss.Width(line2) - lipgloss.Width(relTime)
			if padding > 0 {
				line2 += strings.Repeat(" ", padding)
			}
			line2 += relTime

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
			authorStyle := th.BoxPRAuthor
			mutedStyle := lipgloss.NewStyle().Foreground(th.Muted)

			globalRow1 := rowStart - localStart
			if globalRow1 >= 0 && globalRow1 < contentH {
				headlineMax := cw - shaW - gapW
				out[globalRow1] = shaStyled + gap + truncateString(c.MessageHeadline, headlineMax)
			}
			globalRow2 := rowStart + 1 - localStart
			if globalRow2 >= 0 && globalRow2 < contentH {
				authorStyled := authorStyle.Render(author)
				relStyled := mutedStyle.Render(relTime)
				padding := cw - lipgloss.Width(authorStyled) - lipgloss.Width(relStyled)
				if padding < 0 {
					padding = 0
				}
				out[globalRow2] = authorStyled + strings.Repeat(" ", padding) + relStyled
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
