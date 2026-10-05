package prdetail

import (
	tea "github.com/charmbracelet/bubbletea"
	"slices"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	diffsearch "github.com/utkarsh261/pho/internal/diff/search"
)

// SearchStatusState returns the status-bar payload for the current search state.
// matchIndex is 1-indexed for display.
func (m *PRDetailModel) SearchStatusState() (query string, matchIndex int, matchCount int, active bool) {
	if !m.searchActive {
		return "", 0, 0, false
	}
	return m.searchQuery, m.searchCursor + 1, len(m.searchMatches), true
}

func (m *PRDetailModel) activateSearch() {
	if m.activeTab != TabDiff {
		return
	}
	m.searchActive = true
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
	m.searchCommit = false
	m.ensureSearchIndex()
}

func (m *PRDetailModel) clearSearch() {
	m.searchActive = false
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
	m.searchCommit = false
}

func (m *PRDetailModel) handleSearchKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "esc":
		m.clearSearch()
		return true
	case "enter":
		m.searchCommit = true
		if len(m.searchMatches) > 0 {
			m.searchCursor = 0
			m.scrollToSearchCursor()
		}
		return true
	case "backspace", "ctrl+h":
		m.searchBackspace()
		return true
	case "n":
		if m.searchCommit {
			if next, ok := diffsearch.NextMatch(m.searchMatches, m.searchCursor); ok {
				m.searchCursor = next
				m.scrollToSearchCursor()
			}
		} else {
			m.searchQuery += "n"
			m.refreshSearchMatches()
		}
		return true
	case "N":
		if m.searchCommit {
			if prev, ok := diffsearch.PrevMatch(m.searchMatches, m.searchCursor); ok {
				m.searchCursor = prev
				m.scrollToSearchCursor()
			}
		} else {
			m.searchQuery += "N"
			m.refreshSearchMatches()
		}
		return true
	case " ":
		m.searchQuery += " "
		m.refreshSearchMatches()
		return true
	case "/":
		// Restart search input while staying in search mode.
		m.activateSearch()
		return true
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		m.searchQuery += string(msg.Runes)
		m.refreshSearchMatches()
		return true
	}

	return false
}

func (m *PRDetailModel) searchBackspace() {
	r := []rune(m.searchQuery)
	if len(r) == 0 {
		m.searchMatches = nil
		m.searchCursor = 0
		return
	}
	m.searchQuery = string(r[:len(r)-1])
	m.refreshSearchMatches()
	m.searchCommit = false
}

func (m *PRDetailModel) ensureSearchIndex() {
	if m.searchIndex != nil || m.Diff == nil {
		return
	}
	m.normalizeDiffRows()
	m.searchIndex = diffsearch.Build(m.Diff)
}

func (m *PRDetailModel) refreshSearchMatches() {
	if !m.searchActive {
		return
	}
	if m.searchQuery == "" {
		m.searchMatches = nil
		m.searchCursor = 0
		m.searchCommit = false
		return
	}
	m.ensureSearchIndex()
	if m.searchIndex == nil {
		m.searchMatches = nil
		m.searchCursor = 0
		m.searchCommit = false
		return
	}
	m.searchMatches = m.searchIndex.Search(m.searchQuery)
	// Files too large to show can't be jumped to, so their matches are dropped.
	m.searchMatches = slices.DeleteFunc(m.searchMatches, func(mt diffsearch.Match) bool {
		return m.fileTooLarge(mt.FileIndex)
	})
	m.searchCursor = 0
	m.searchCommit = false
}

func (m *PRDetailModel) currentSearchMatch() (diffsearch.Match, bool) {
	if !m.searchActive || len(m.searchMatches) == 0 {
		return diffsearch.Match{}, false
	}
	if m.searchCursor < 0 || m.searchCursor >= len(m.searchMatches) {
		return diffsearch.Match{}, false
	}
	return m.searchMatches[m.searchCursor], true
}

// normalizeDiffRows keeps StartRow/DisplayRows aligned with the renderer's
// authoritative row layout.
func (m *PRDetailModel) normalizeDiffRows() {
	if m.Diff == nil {
		return
	}
	lay := m.rows()
	for i := range m.Diff.Files {
		m.Diff.Files[i].DisplayRows = lay.fileRows[i]
		m.Diff.Files[i].StartRow = lay.fileStart[i]
	}
}

func (m *PRDetailModel) scrollToSearchCursor() {
	if m.Diff == nil {
		return
	}

	match, ok := m.currentSearchMatch()
	if !ok {
		return
	}
	if match.FileIndex < 0 || match.FileIndex >= len(m.Diff.Files) {
		return
	}

	m.expandFileForJump(match.FileIndex)
	m.normalizeDiffRows()

	flatLineIndexWithinFile := m.matchDisplayOffsetWithinFile(match)
	matchDisplayRow := m.Diff.Files[match.FileIndex].StartRow + flatLineIndexWithinFile

	contentHeight := m.contentViewportHeight()
	m.switchTab(TabDiff)
	m.ContentScroll = clamp(matchDisplayRow-contentHeight/2, 0, m.maxContentScroll())

	// Sync diff cursor to the search match line.
	if fi, hi, li, found := m.firstDiffLineAtOrBelow(matchDisplayRow); found {
		m.setDiffCursor(diffCursorLine{FileIdx: fi, HunkIdx: hi, LineIdx: li})
	}
}

func (m *PRDetailModel) matchDisplayOffsetWithinFile(match diffsearch.Match) int {
	if m.Diff == nil || match.FileIndex < 0 || match.FileIndex >= len(m.Diff.Files) {
		return 0
	}
	flatLineIndexWithinFile := m.matchLineIndexWithinFile(match)
	return m.Diff.Files[match.FileIndex].LineDisplayRow(flatLineIndexWithinFile) + 2
}

func (m *PRDetailModel) matchLineIndexWithinFile(match diffsearch.Match) int {
	if m.Diff == nil {
		return 0
	}
	line := match.LineIndex
	limit := min(match.FileIndex, len(m.Diff.Files))
	for i := range limit {
		line -= diffFileLineCount(&m.Diff.Files[i])
	}
	return max(0, line)
}

func diffFileLineCount(f *diffmodel.DiffFile) int {
	total := 0
	for _, h := range f.Hunks {
		total += len(h.Lines)
	}
	return total
}
