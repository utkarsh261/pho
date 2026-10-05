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

// commitGraphWidth is the width of the git-graph column drawn left of each commit.
const commitGraphWidth = 3

// renderCommitsTab renders the Commits tab content as a vertical timeline:
// a "●" node on each commit's headline row joined by "│" down to the next.
// availW is the available content width (innerW - 1, accounting for the left-pad
// space that renderRightViewport adds to every line).
func (m *PRDetailModel) renderCommitsTab(scroll, contentH, availW int) []string {
	if m.commitsLoading || len(m.commits) == 0 || availW <= commitGraphWidth+10 {
		return m.renderCommitRows(scroll, contentH, availW)
	}
	rows := m.renderCommitRows(scroll, contentH, availW-commitGraphWidth)

	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	rail := th.FaintTxt
	node := th.DimTxt
	for i := range rows {
		r := scroll + i
		idx, off := r/3, r%3
		var g string
		switch {
		case idx >= len(m.commits):
			g = "   "
		case off == 0 && idx == m.commitCursor:
			g = " " + th.PrimaryTxt.Render("◉") + " "
		case off == 0:
			g = " " + node.Render("●") + " "
		case idx < len(m.commits)-1:
			g = " " + rail.Render("│") + " "
		default:
			g = "   "
		}
		rows[i] = g + rows[i]
	}
	return rows
}

// renderCommitRows renders the commit rows (headline + metadata + gap) at width availW.
func (m *PRDetailModel) renderCommitRows(scroll, contentH, availW int) []string {
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
			headline := lipgloss.NewStyle().Bold(true).Foreground(th.TextBright).
				Render(truncateText(c.MessageHeadline, max(cw-shaW-gapW, 1)))
			line1 := theme.FillBg(th.Highlight, cw, shaStyled+gap+headline)

			authorStyled := th.DimTxt.Render(author)
			relStyled := th.MutedTxt.Render(relTime)
			padding := max(cw-lipgloss.Width(authorStyled)-lipgloss.Width(relStyled), 0)
			line2 := theme.FillBg(th.Highlight, cw, authorStyled+strings.Repeat(" ", padding)+relStyled)

			parts := []string{line1, line2}
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
			mutedStyle := th.MutedTxt

			globalRow1 := rowStart - localStart
			if globalRow1 >= 0 && globalRow1 < contentH {
				headlineMax := cw - shaW - gapW
				out[globalRow1] = shaStyled + gap + truncateText(c.MessageHeadline, headlineMax)
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
