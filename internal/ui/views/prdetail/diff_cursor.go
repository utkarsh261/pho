package prdetail

// validVisualState reports whether m.visual indices are in bounds for the
// current m.Diff. Call before accessing m.Diff.Files[FileIdx] or f.Hunks[HunkIdx].
func (m *PRDetailModel) validVisualState() bool {
	if m.Diff == nil || !m.visual.Active {
		return false
	}
	if m.visual.FileIdx < 0 || m.visual.FileIdx >= len(m.Diff.Files) {
		return false
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	if m.visual.HunkIdx < 0 || m.visual.HunkIdx >= len(f.Hunks) {
		return false
	}
	h := &f.Hunks[m.visual.HunkIdx]
	if m.visual.StartLine < 0 || m.visual.StartLine >= len(h.Lines) {
		return false
	}
	if m.visual.EndLine < 0 || m.visual.EndLine >= len(h.Lines) {
		return false
	}
	return true
}

// expandVisualSelectionDown grows the selection by one line downward within the hunk.
func (m *PRDetailModel) expandVisualSelectionDown() {
	if !m.validVisualState() {
		return
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	if m.visual.EndLine+1 < len(h.Lines) {
		m.visual.EndLine++
		// Auto-scroll to keep selection visible with scrollPadding.
		endRow := m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.EndLine)
		vh := m.contentViewportHeight()
		pad := min(scrollPadding, vh/2)
		if endRow >= m.ContentScroll+vh-pad {
			m.ContentScroll = max(0, endRow-vh+1+pad)
			m.clampContentScroll()
		}
	}
}

// shrinkVisualSelectionUp shrinks the selection by one line upward.
// If selection is single-line, exits visual mode.
func (m *PRDetailModel) shrinkVisualSelectionUp() {
	if !m.visual.Active {
		return
	}
	if !m.validVisualState() {
		m.exitVisualMode()
		return
	}
	if m.visual.EndLine > m.visual.StartLine {
		m.visual.EndLine--
		// Auto-scroll to keep selection visible with scrollPadding.
		startRow := m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine)
		pad := min(scrollPadding, m.contentViewportHeight()/2)
		if startRow < m.ContentScroll+pad {
			m.ContentScroll = max(0, startRow-pad)
			m.clampContentScroll()
		}
	} else {
		m.exitVisualMode()
	}
}

func (m *PRDetailModel) validDiffCursor() bool {
	if m.navIdx < 0 || m.navIdx >= len(m.navigableLines) {
		return false
	}
	// Defensive: ensure diffCursor matches navIdx.
	if m.diffCursor != m.navigableLines[m.navIdx] {
		return false
	}
	return m.navigableRows[m.navIdx] < maxDiffDisplayRows
}

func (m *PRDetailModel) invalidateDiffCursor() {
	m.diffCursor = diffCursorLine{FileIdx: -1}
	m.navIdx = -1
}

// buildNavigableIndex creates a flat ordered slice of every actual diff line,
// skipping binary files. A parallel display-row slice and a reverse map from
// (file,hunk,line) → flat index are also built so cursor movement is O(1).
func (m *PRDetailModel) buildNavigableIndex() {
	m.navigableLines = m.navigableLines[:0]
	m.navigableRows = m.navigableRows[:0]
	m.navIdxMap = make(map[diffCursorLine]int)
	m.navIdx = -1
	if m.Diff == nil {
		return
	}
	for fi, f := range m.Diff.Files {
		if f.IsBinary {
			continue
		}
		for hi, h := range f.Hunks {
			for li := range h.Lines {
				cursor := diffCursorLine{FileIdx: fi, HunkIdx: hi, LineIdx: li}
				m.navIdxMap[cursor] = len(m.navigableLines)
				m.navigableLines = append(m.navigableLines, cursor)
				m.navigableRows = append(m.navigableRows, m.diffLineToDisplayRow(fi, hi, li))
			}
		}
	}
}

func (m *PRDetailModel) invalidateNavigableIndex() {
	m.navigableLines = nil
	m.navigableRows = nil
	m.navIdxMap = nil
	m.navIdx = -1
}

// setDiffCursor updates diffCursor and keeps navIdx in sync.
func (m *PRDetailModel) setDiffCursor(cursor diffCursorLine) {
	m.diffCursor = cursor
	if m.navIdxMap != nil {
		if idx, ok := m.navIdxMap[cursor]; ok {
			m.navIdx = idx
			return
		}
	}
	m.navIdx = -1
}

func (m *PRDetailModel) ensureDiffCursor() {
	if m.validDiffCursor() {
		return
	}
	// Try to sync navIdx from the current diffCursor.
	m.setDiffCursor(m.diffCursor)
	if m.validDiffCursor() {
		return
	}
	// Find first navigable line at or below the current scroll position.
	targetRow := m.ContentScroll
	for i, row := range m.navigableRows {
		if row >= targetRow {
			m.setDiffCursor(m.navigableLines[i])
			return
		}
	}
	// Fallback to last navigable line.
	if len(m.navigableLines) > 0 {
		m.setDiffCursor(m.navigableLines[len(m.navigableLines)-1])
		return
	}
	m.invalidateDiffCursor()
}

// moveCursorDown moves the diff cursor to the next navigable diff line.
func (m *PRDetailModel) moveCursorDown() {
	if len(m.navigableLines) == 0 {
		return
	}
	if m.navIdx >= 0 && m.navIdx < len(m.navigableLines)-1 {
		m.navIdx++
		m.setDiffCursor(m.navigableLines[m.navIdx])
		m.syncFilePanelToCursor()
	}
}

// moveCursorUp moves the diff cursor to the previous navigable diff line.
func (m *PRDetailModel) moveCursorUp() {
	if len(m.navigableLines) == 0 {
		return
	}
	if m.navIdx > 0 {
		m.navIdx--
		m.setDiffCursor(m.navigableLines[m.navIdx])
		m.syncFilePanelToCursor()
	}
}

// moveCursorBy moves the diff cursor by delta lines (negative = up).
// Clamps at boundaries; does nothing if delta is 0.
func (m *PRDetailModel) moveCursorBy(delta int) {
	if delta == 0 || len(m.navigableLines) == 0 || m.navIdx < 0 {
		return
	}
	m.navIdx += delta
	if m.navIdx < 0 {
		m.navIdx = 0
	} else if m.navIdx >= len(m.navigableLines) {
		m.navIdx = len(m.navigableLines) - 1
	}
	m.setDiffCursor(m.navigableLines[m.navIdx])
	m.syncFilePanelToCursor()
}

// scrollToCursor adjusts ContentScroll so the cursor stays within
// padding lines of the viewport edges. Uses min(padding, vh/2)
// so tiny viewports don't over-scroll.
func (m *PRDetailModel) scrollToCursor(padding int) {
	if !m.validDiffCursor() {
		return
	}
	row := m.navigableRows[m.navIdx]
	vh := m.contentViewportHeight()
	pad := min(padding, vh/2)
	if row < m.ContentScroll+pad {
		m.ContentScroll = max(0, row-pad)
	}
	if row >= m.ContentScroll+vh-pad {
		m.ContentScroll = max(0, row-vh+1+pad)
	}
	m.clampContentScroll()
}

// syncFilePanelToCursor updates leftPanel.FileIndex to match the cursor's
// current file, and scrolls the file panel so it's visible.
func (m *PRDetailModel) syncFilePanelToCursor() {
	if m.Diff == nil {
		return
	}
	if m.diffCursor.FileIdx >= 0 && m.diffCursor.FileIdx < len(m.Diff.Files) {
		m.leftPanel.Cursor = m.diffCursor.FileIdx
		m.ensureFileVisible()
	}
}

// jumpToCommentCode switches to the Diff tab and scrolls to the code line
// referenced by the focused comment entry.
func (m *PRDetailModel) jumpToCommentCode() {
	if m.commentCursor < 0 {
		return
	}
	entries := m.commentEntries()
	if m.commentCursor >= len(entries) {
		return
	}
	entry := entries[m.commentCursor]
	if entry.path == "" || entry.line <= 0 {
		return
	}
	// Find the diff line matching (path, line).
	if fi, hi, li, ok := m.findDiffLineAnchorAnySide(entry.path, entry.line); ok {
		m.switchTab(TabDiff)
		m.setDiffCursor(diffCursorLine{FileIdx: fi, HunkIdx: hi, LineIdx: li})
		m.scrollToCursor(scrollPadding)
	}
}
