package prdetail

import (
	"context"
	"time"
	"github.com/utkarsh261/pho/internal/domain"
)

// diffLineToDisplayRow returns the display row for a diff line relative to the
// start of the Diff tab.
func (m *PRDetailModel) diffLineToDisplayRow(fileIdx, hunkIdx, lineIdx int) int {
	if m.Diff == nil {
		return 0
	}
	row := 0
	for i := range fileIdx {
		row += diffFileDisplayRows(&m.Diff.Files[i])
	}
	row += diffFileHeaderRows // blank + separator + header
	f := &m.Diff.Files[fileIdx]
	for i := range hunkIdx {
		row += 1 + len(f.Hunks[i].Lines)
	}
	row += 1 + lineIdx // hunk header + line offset
	return row
}

// firstDiffLineAtOrBelow finds the first actual DiffLine at or after targetRow,
// where targetRow is relative to the start of the Diff tab.
// Binary files are skipped; if targetRow lands inside a binary file, the search
// continues to subsequent files. The target is clamped to maxDiffDisplayRows-1
// so only rendered lines are returned.
func (m *PRDetailModel) firstDiffLineAtOrBelow(targetRow int) (fileIdx, hunkIdx, lineIdx int, found bool) {
	if len(m.navigableLines) == 0 {
		return 0, 0, 0, false
	}
	if targetRow < 0 {
		targetRow = 0
	}
	if targetRow >= maxDiffDisplayRows {
		targetRow = maxDiffDisplayRows - 1
	}
	for i, row := range m.navigableRows {
		if row >= targetRow {
			c := m.navigableLines[i]
			return c.FileIdx, c.HunkIdx, c.LineIdx, true
		}
	}
	// Fallback to last navigable line.
	c := m.navigableLines[len(m.navigableLines)-1]
	return c.FileIdx, c.HunkIdx, c.LineIdx, true
}

// enterVisualMode activates visual mode anchored at the current diff cursor
// position if valid, otherwise at the first diff line at or below ContentScroll.
func (m *PRDetailModel) enterVisualMode() {
	var fi, hi, li int
	var ok bool
	if m.validDiffCursor() {
		fi, hi, li = m.diffCursor.FileIdx, m.diffCursor.HunkIdx, m.diffCursor.LineIdx
		ok = true
	} else {
		fi, hi, li, ok = m.firstDiffLineAtOrBelow(m.ContentScroll)
	}
	if !ok {
		return
	}
	m.visual = visualModeState{
		Active:    true,
		FileIdx:   fi,
		HunkIdx:   hi,
		StartLine: li,
		EndLine:   li,
	}
}

// exitVisualMode deactivates visual mode and places the diff cursor at the
// selection start line.
func (m *PRDetailModel) exitVisualMode() {
	if m.validVisualState() {
		m.setDiffCursor(diffCursorLine{
			FileIdx: m.visual.FileIdx,
			HunkIdx: m.visual.HunkIdx,
			LineIdx: m.visual.StartLine,
		})
	}
	m.visual.Active = false
}

// buildDraftFromVisualSelection creates a DraftInlineComment from the current
// visual selection and the provided body text.
func (m *PRDetailModel) buildDraftFromVisualSelection(body string) domain.DraftInlineComment {
	if !m.validVisualState() {
		return domain.DraftInlineComment{}
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	firstLine := h.Lines[m.visual.StartLine]
	lastLine := h.Lines[m.visual.EndLine]

	path, lineNum, side, ok := anchorForLine(f, &lastLine)
	if !ok {
		return domain.DraftInlineComment{}
	}

	draft := domain.DraftInlineComment{
		ID:          generateDraftID(),
		Path:        path,
		Line:        lineNum,
		Side:        side,
		Body:        body,
		ContextLine: lastLine.Raw,
		HeadSHA:     m.headSHA(),
		CreatedAt:   time.Now(),
	}
	if m.visual.StartLine != m.visual.EndLine {
		if _, sl, ss, sok := anchorForLine(f, &firstLine); sok {
			draft.StartLine = sl
			draft.StartSide = ss
		}
	}
	return draft
}

// upsertDraft replaces an existing draft on the exact same range or appends a new one.
func (m *PRDetailModel) upsertDraft(draft domain.DraftInlineComment) {
	for i, d := range m.drafts {
		if d.Path == draft.Path && d.Line == draft.Line && d.Side == draft.Side &&
			d.StartLine == draft.StartLine && d.StartSide == draft.StartSide {
			m.drafts[i] = draft
			m.rebuildDraftCovered()
			m.commentEntriesDirty = true
			return
		}
	}
	m.drafts = append(m.drafts, draft)
	m.rebuildDraftCovered()
	m.commentEntriesDirty = true
}

// removeDraftAt removes any draft that overlaps the given file/hunk/line range.
func (m *PRDetailModel) removeDraftAt(fileIdx, hunkIdx, startLine, endLine int) bool {
	if m.Diff == nil {
		return false
	}
	if fileIdx < 0 || fileIdx >= len(m.Diff.Files) {
		return false
	}
	f := &m.Diff.Files[fileIdx]
	if hunkIdx < 0 || hunkIdx >= len(f.Hunks) {
		return false
	}
	h := &f.Hunks[hunkIdx]
	if startLine < 0 || startLine >= len(h.Lines) || endLine < 0 || endLine >= len(h.Lines) {
		return false
	}
	firstLine := h.Lines[startLine]
	lastLine := h.Lines[endLine]
	if len(lastLine.Anchors) == 0 {
		return false
	}
	path := lastLine.Anchors[0].Path
	line := *lastLine.Anchors[0].Line
	side := lastLine.Anchors[0].Side
	startLineNum := 0
	startSide := ""
	if startLine != endLine && len(firstLine.Anchors) > 0 {
		startLineNum = *firstLine.Anchors[0].Line
		startSide = firstLine.Anchors[0].Side
	}

	// Iterate backwards so slice deletion doesn't skip elements.
	for i := len(m.drafts) - 1; i >= 0; i-- {
		d := m.drafts[i]
		if d.Path == path && d.Side == side && d.Line == line {
			// Single-line draft match.
			if d.StartLine == 0 && startLineNum == 0 {
				m.drafts = append(m.drafts[:i], m.drafts[i+1:]...)
				m.rebuildDraftCovered()
				m.commentEntriesDirty = true
				return true
			}
			// Multi-line draft match.
			if d.StartLine == startLineNum && d.StartSide == startSide {
				m.drafts = append(m.drafts[:i], m.drafts[i+1:]...)
				m.rebuildDraftCovered()
				m.commentEntriesDirty = true
				return true
			}
		}
	}
	return false
}

// findDraftForSelection returns the draft matching the exact current visual
// selection, or nil if none exists.
func (m *PRDetailModel) findDraftForSelection() *domain.DraftInlineComment {
	if !m.validVisualState() {
		return nil
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	firstLine := h.Lines[m.visual.StartLine]
	lastLine := h.Lines[m.visual.EndLine]
	if len(lastLine.Anchors) == 0 {
		return nil
	}
	path := lastLine.Anchors[0].Path
	line := *lastLine.Anchors[0].Line
	side := lastLine.Anchors[0].Side
	startLineNum := 0
	startSide := ""
	if m.visual.StartLine != m.visual.EndLine && len(firstLine.Anchors) > 0 {
		startLineNum = *firstLine.Anchors[0].Line
		startSide = firstLine.Anchors[0].Side
	}

	for i := range m.drafts {
		d := &m.drafts[i]
		if d.Path == path && d.Side == side && d.Line == line {
			if d.StartLine == 0 && startLineNum == 0 {
				return d
			}
			if d.StartLine == startLineNum && d.StartSide == startSide {
				return d
			}
		}
	}
	return nil
}

// rebuildDraftCovered recomputes the draftCovered map from m.drafts.
// Call this whenever drafts change (add, remove, load, clear).
func (m *PRDetailModel) rebuildDraftCovered() {
	if m.Diff == nil {
		m.draftCovered = nil
		return
	}
	m.ensureDiffIndices()
	m.draftCovered = make(map[hunkLineKey]bool)
	for _, d := range m.drafts {
		fi, hi, endLI, ok := m.findDiffLineAnchor(d.Path, d.Line, d.Side)
		if !ok {
			continue
		}
		startLI := endLI
		if d.StartLine > 0 {
			if _, _, sli, ok := m.findDiffLineAnchor(d.Path, d.StartLine, d.StartSide); ok {
				startLI = sli
			}
		}
		for li := startLI; li <= endLI; li++ {
			m.draftCovered[hunkLineKey{fi, hi, li}] = true
		}
	}
}

func (m *PRDetailModel) persistDrafts() {
	if m.PRService == nil {
		return
	}
	headSHA := m.headSHA()
	if headSHA == "" {
		return // no SHA to key drafts against; skip persistence to avoid collision
	}
	// Errors are logged by the service layer; no additional UI feedback needed.
	_ = m.PRService.SaveDraftComments(context.Background(), m.Repo, m.Summary.Number, headSHA, m.drafts)
}

// loadDrafts loads drafts from the cache for the current PR.
func (m *PRDetailModel) loadDrafts() {
	if m.PRService == nil {
		return
	}
	headSHA := m.headSHA()
	if headSHA == "" {
		return
	}
	// Errors are logged by the service layer; missing cache entries are expected (not an error).
	drafts, _ := m.PRService.LoadDraftComments(context.Background(), m.Repo, m.Summary.Number, headSHA)
	m.drafts = drafts
	m.rebuildDraftCovered()
	m.commentEntriesDirty = true
}

func (m *PRDetailModel) headSHA() string {
	if m.Diff != nil && m.Diff.HeadSHA != "" {
		return m.Diff.HeadSHA
	}
	return m.Summary.HeadRefOID
}

// ensureDiffIndices lazily rebuilds the diff indices when they are stale.
func (m *PRDetailModel) ensureDiffIndices() {
	if m.Diff != nil && m.diffLineIndex == nil {
		m.rebuildDiffIndices()
	}
}

// rebuildDiffIndices rebuilds the O(1) lookup maps from m.Diff.
// Call whenever m.Diff changes.
func (m *PRDetailModel) rebuildDiffIndices() {
	if m.Diff == nil {
		m.diffLineIndex = nil
		m.diffAnchorIndex = nil
		return
	}
	m.diffLineIndex = make(map[string]map[int]string)
	m.diffAnchorIndex = make(map[string]map[int]map[string][3]int)
	for fi, f := range m.Diff.Files {
		for hi, h := range f.Hunks {
			for li, dl := range h.Lines {
				for _, a := range dl.Anchors {
					if a.Path == "" || a.Line == nil {
						continue
					}
					lineNum := *a.Line
					if m.diffLineIndex[a.Path] == nil {
						m.diffLineIndex[a.Path] = make(map[int]string)
						m.diffAnchorIndex[a.Path] = make(map[int]map[string][3]int)
					}
					m.diffLineIndex[a.Path][lineNum] = dl.Raw
					if m.diffAnchorIndex[a.Path][lineNum] == nil {
						m.diffAnchorIndex[a.Path][lineNum] = make(map[string][3]int)
					}
					m.diffAnchorIndex[a.Path][lineNum][a.Side] = [3]int{fi, hi, li}
				}
			}
		}
	}
}

// lookupDiffLine finds the raw diff line text for a given path:line.
func (m *PRDetailModel) lookupDiffLine(path string, line int) string {
	m.ensureDiffIndices()
	if m.diffLineIndex == nil {
		return ""
	}
	if lines, ok := m.diffLineIndex[path]; ok {
		return lines[line]
	}
	return ""
}

// findDiffLineAnchor returns the hunk coordinates for a given path:line:side anchor.
func (m *PRDetailModel) findDiffLineAnchor(path string, line int, side string) (fileIdx, hunkIdx, lineIdx int, ok bool) {
	m.ensureDiffIndices()
	if m.diffAnchorIndex == nil {
		return 0, 0, 0, false
	}
	if lines, ok := m.diffAnchorIndex[path]; ok {
		if sides, ok := lines[line]; ok {
			if coords, ok := sides[side]; ok {
				return coords[0], coords[1], coords[2], true
			}
		}
	}
	return 0, 0, 0, false
}

// findDiffLineAnchorAnySide returns the hunk coordinates for any anchor matching
// path:line, regardless of side. Tries RIGHT first, then LEFT, then any other.
func (m *PRDetailModel) findDiffLineAnchorAnySide(path string, line int) (fileIdx, hunkIdx, lineIdx int, ok bool) {
	m.ensureDiffIndices()
	if fi, hi, li, ok := m.findDiffLineAnchor(path, line, "RIGHT"); ok {
		return fi, hi, li, ok
	}
	if fi, hi, li, ok := m.findDiffLineAnchor(path, line, "LEFT"); ok {
		return fi, hi, li, ok
	}
	return 0, 0, 0, false
}

// SearchActive reports whether the diff search is currently active.
func (m *PRDetailModel) SearchActive() bool { return m.searchActive }

// isInDiffSection reports whether the user is viewing the diff content area.
func (m *PRDetailModel) isInDiffSection() bool {
	return m.activeTab == TabDiff && m.leftPanel.Focus == FocusContent
}

// IsDiffTabActive reports whether the Diff tab is currently active.
func (m *PRDetailModel) IsDiffTabActive() bool { return m.activeTab == TabDiff }

// ActiveTab returns the currently active content tab.
func (m *PRDetailModel) ActiveTab() ContentTab { return m.activeTab }

// DraftCount returns the number of pending draft inline comments.
func (m *PRDetailModel) DraftCount() int { return len(m.drafts) }
