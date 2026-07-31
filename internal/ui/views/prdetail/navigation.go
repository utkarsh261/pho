package prdetail

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"
)

// isInCommentsSection reports whether the Comments tab is active.
func (m *PRDetailModel) isInCommentsSection() bool {
	return m.activeTab == TabComments && m.leftPanel.Focus == FocusContent
}

// resetCommentCursor clears the comment cursor. Call whenever navigation leaves
// the Comments section or data changes.
func (m *PRDetailModel) resetCommentCursor() {
	m.commentCursor = -1
}

// moveCursorNextComment advances the comment cursor by one entry, scrolling
// the viewport to keep it visible. First call activates the cursor at entry 0.
func (m *PRDetailModel) moveCursorNextComment() {
	entries := m.commentEntries()
	if len(entries) == 0 {
		return
	}
	if m.commentCursor < 0 {
		m.commentCursor = 0
	} else if m.commentCursor < len(entries)-1 {
		m.commentCursor++
	}
	m.scrollToCommentCursor()
}

// moveCursorPrevComment moves the comment cursor back one entry. At entry 0,
// deactivates the cursor.
func (m *PRDetailModel) moveCursorPrevComment() {
	if m.commentCursor <= 0 {
		m.commentCursor = -1
		return
	}
	m.commentCursor--
	m.scrollToCommentCursor()
}

// scrollToCommentCursor scrolls comment-wise so the focused comment box is
// fully visible in the viewport. If the comment is taller than the viewport,
// its top is aligned to the top of the viewport. No-op when the entry already
// fits.
func (m *PRDetailModel) scrollToCommentCursor() {
	if m.commentCursor < 0 {
		return
	}
	cw := contentViewportWidth(m.rightPanelWidth())
	startRows := m.commentEntryStartRows(cw)
	if m.commentCursor >= len(startRows) {
		return
	}
	entries := m.commentEntries()
	entryTop := startRows[m.commentCursor]
	entryBottom := entryTop + m.entryRowCount(entries[m.commentCursor], cw)
	isRoot := entries[m.commentCursor].threadID == "" || m.commentCursor == 0 || entries[m.commentCursor-1].threadID != entries[m.commentCursor].threadID
	isLast := entries[m.commentCursor].threadID == "" || m.commentCursor == len(entries)-1 || entries[m.commentCursor+1].threadID != entries[m.commentCursor].threadID
	if isRoot {
		entryBottom += 1 // top border
	}
	if isLast {
		entryBottom += 1 // bottom border
	}
	vh := m.contentViewportHeight()
	viewTop := m.ContentScroll
	viewBottom := viewTop + vh

	switch {
	case entryTop < viewTop:
		// Entry top is above viewport: scroll up to show it from the top.
		m.ContentScroll = entryTop
	case entryBottom > viewBottom:
		// Entry bottom is below viewport: scroll down to show the whole comment
		// box starting from its top (comment-wise scrolling).
		m.ContentScroll = entryTop
	}
	m.clampContentScroll()
}

// moveCommentCursorBy shifts the comment cursor by approximately delta display
// rows, clamping to the first/last entry. Positive delta moves down; negative
// moves up. The viewport is scrolled so the new entry is visible.
func (m *PRDetailModel) moveCommentCursorBy(delta int) {
	entries := m.commentEntries()
	if len(entries) == 0 {
		return
	}
	cw := contentViewportWidth(m.rightPanelWidth())
	startRows := m.commentEntryStartRows(cw)
	if startRows == nil {
		return
	}
	if m.commentCursor < 0 {
		if delta >= 0 {
			m.commentCursor = 0
		} else {
			m.commentCursor = len(entries) - 1
		}
		m.scrollToCommentCursor()
		return
	}
	currentTop := startRows[m.commentCursor]
	targetTop := currentTop + delta
	if targetTop <= startRows[0] {
		m.commentCursor = 0
		m.scrollToCommentCursor()
		return
	}
	last := len(startRows) - 1
	if targetTop >= startRows[last] {
		m.commentCursor = last
		m.scrollToCommentCursor()
		return
	}
	// Find the entry whose start row is closest to target without going past it.
	best := m.commentCursor
	bestDist := abs(targetTop - startRows[best])
	for i, sr := range startRows {
		d := abs(targetTop - sr)
		if d < bestDist {
			best = i
			bestDist = d
		}
	}
	m.commentCursor = best
	m.scrollToCommentCursor()
}

// jumpToFile switches to the Diff tab and scrolls so that file at index idx is at
// the top. No-op when diff is absent or idx is out of range.
func (m *PRDetailModel) jumpToFile(idx int) {
	if m.Diff == nil || idx < 0 || idx >= len(m.Diff.Files) {
		return
	}
	m.leftPanel.LastOpenedIndex = idx
	m.switchTab(TabDiff)
	m.leftPanel.Focus = FocusContent
	fileOffset := 0
	for i := range idx {
		fileOffset += diffFileDisplayRows(&m.Diff.Files[i])
	}
	contentHeight := m.contentViewportHeight()
	diffRows := m.diffSectionRowCount()
	// When fileOffset falls beyond the rendered diff (truncated large diffs), show
	// the truncation banner instead.
	if fileOffset >= diffRows {
		m.ContentScroll = clamp(max(0, diffRows-contentHeight), 0, m.maxContentScroll())
		return
	}
	m.ContentScroll = clamp(fileOffset, 0, m.maxContentScroll())
	// Position the diff cursor at the first diff line of the target file
	// (skipping binary files to find the next navigable line).
	if fi, hi, li, ok := m.firstDiffLineAtOrBelow(fileOffset); ok {
		m.setDiffCursor(diffCursorLine{FileIdx: fi, HunkIdx: hi, LineIdx: li})
	}
}

// cycleForward advances focus: Files → CI (if checks) → Content → Files.
func (m *PRDetailModel) cycleForward() {
	if m.Width < MinWidthForSidebar {
		return // sidebar hidden, only Content exists
	}
	switch m.leftPanel.Focus {
	case FocusFiles:
		if !m.leftPanel.HideCI && len(m.leftPanel.Checks) > 0 {
			m.leftPanel.CICursor = 0
			m.leftPanel.Focus = FocusCI
		} else {
			m.leftPanel.Focus = FocusContent
		}
	case FocusCI:
		m.leftPanel.Focus = FocusContent
	case FocusContent:
		m.leftPanel.Focus = FocusFiles
	}
}

// cycleBackward retreats focus: Files → Content → CI (if checks) → Files.
func (m *PRDetailModel) cycleBackward() {
	if m.Width < MinWidthForSidebar {
		return
	}
	switch m.leftPanel.Focus {
	case FocusFiles:
		m.leftPanel.Focus = FocusContent
	case FocusCI:
		m.leftPanel.Focus = FocusFiles
	case FocusContent:
		if !m.leftPanel.HideCI && len(m.leftPanel.Checks) > 0 {
			m.leftPanel.CICursor = 0
			m.leftPanel.Focus = FocusCI
		} else {
			m.leftPanel.Focus = FocusFiles
		}
	}
}

// Navigation within focused sub-area

func (m *PRDetailModel) scrollDown() {
	switch m.leftPanel.Focus {
	case FocusFiles:
		if len(m.leftPanel.Files) == 0 {
			return
		}
		m.leftPanel.Cursor++
		last := len(m.leftPanel.Files) - 1
		if m.leftPanel.Cursor > last {
			// If CI has checks, move focus there.
			m.leftPanel.Cursor = last
			if !m.leftPanel.HideCI && len(m.leftPanel.Checks) > 0 {
				m.leftPanel.CICursor = 0
				m.leftPanel.CIScroll = 0
				m.leftPanel.Focus = FocusCI
			}
			return
		}
		m.ensureFileVisible()
	case FocusCI:
		if len(m.leftPanel.Checks) == 0 {
			return
		}
		m.leftPanel.CICursor++
		last := len(m.leftPanel.Checks) - 1
		if m.leftPanel.CICursor > last {
			m.leftPanel.CICursor = last
		}
		m.ensureCIVisible()
	case FocusContent:
		m.ContentScroll++
		m.clampContentScroll()
	}
}

func (m *PRDetailModel) scrollUp() {
	switch m.leftPanel.Focus {
	case FocusFiles:
		if m.leftPanel.Cursor <= 0 {
			return
		}
		m.leftPanel.Cursor--
		m.ensureFileVisible()
	case FocusCI:
		if m.leftPanel.CICursor <= 0 {
			// move focus back to Files.
			m.leftPanel.Focus = FocusFiles
			m.leftPanel.Scroll = 0
			return
		}
		m.leftPanel.CICursor--
		m.ensureCIVisible()
	case FocusContent:
		m.ContentScroll--
		m.clampContentScroll()
	}
}

func (m *PRDetailModel) jumpFileViewer() {
	if m.leftPanel.Focus == FocusContent && m.Width >= MinWidthForSidebar {
		m.leftPanel.Focus = FocusFiles
	}
}

func (m *PRDetailModel) jumpDiffViewer() {
	if m.leftPanel.Focus == FocusFiles && m.Width >= MinWidthForSidebar {
		m.jumpToFile(m.leftPanel.Cursor)
	}
}

// jumpPrevFile moves to previous file
func (m *PRDetailModel) jumpPrevFile() {
	if m.leftPanel.Focus != FocusFiles {
		return
	}
	m.leftPanel.Cursor = clamp(m.leftPanel.Cursor-1, 0, max(0, len(m.leftPanel.Files)-1))
	m.ensureFileVisible()
}

// jumpNextFile moves the file cursor to the next file
func (m *PRDetailModel) jumpNextFile() {
	if m.leftPanel.Focus != FocusFiles {
		return
	}
	m.leftPanel.Cursor = clamp(m.leftPanel.Cursor+1, 0, max(0, len(m.leftPanel.Files)-1))
	m.ensureFileVisible()
}

func (m *PRDetailModel) scrollToTop() {
	switch m.leftPanel.Focus {
	case FocusFiles:
		m.leftPanel.Cursor = 0
		m.leftPanel.Scroll = 0
	case FocusCI:
		m.leftPanel.CIScroll = 0
	case FocusContent:
		m.ContentScroll = 0
	}
}

func (m *PRDetailModel) scrollToBottom() {
	switch m.leftPanel.Focus {
	case FocusFiles:
		if len(m.leftPanel.Files) > 0 {
			m.leftPanel.Cursor = len(m.leftPanel.Files) - 1
			m.ensureFileVisible()
		}
	case FocusCI:
		visibleCI := m.ciVisibleRows()
		m.leftPanel.CIScroll = max(0, len(m.leftPanel.Checks)-visibleCI)
	case FocusContent:
		m.ContentScroll = m.maxContentScroll()
	}
}

func (m *PRDetailModel) scrollHalfPageDown() {
	half := m.contentViewportHeight() / 2
	switch m.leftPanel.Focus {
	case FocusContent:
		m.ContentScroll += half
		m.clampContentScroll()
	case FocusFiles:
		m.leftPanel.Cursor = clamp(m.leftPanel.Cursor+half, 0, max(0, len(m.leftPanel.Files)-1))
		m.ensureFileVisible()
	case FocusCI:
		visibleCI := m.ciVisibleRows()
		m.leftPanel.CIScroll = clamp(m.leftPanel.CIScroll+half, 0, max(0, len(m.leftPanel.Checks)-visibleCI))
	}
}

func (m *PRDetailModel) scrollHalfPageUp() {
	half := m.contentViewportHeight() / 2
	switch m.leftPanel.Focus {
	case FocusContent:
		m.ContentScroll -= half
		m.clampContentScroll()
	case FocusFiles:
		m.leftPanel.Cursor = clamp(m.leftPanel.Cursor-half, 0, max(0, len(m.leftPanel.Files)-1))
		m.ensureFileVisible()
	case FocusCI:
		m.leftPanel.CIScroll = max(0, m.leftPanel.CIScroll-half)
	}
}

// bodyHeight returns the available rows for the two-panel body.
func (m *PRDetailModel) bodyHeight() int {
	return max(1, m.Height-2) // subtract header + section buttons rows
}

// ciVisibleRows returns the visible row count within the CI sub-area.
func (m *PRDetailModel) ciVisibleRows() int {
	ciH := computeCIHeight(m.bodyHeight(), len(m.leftPanel.Checks))
	inner := ciH - 2
	contentH := max(inner-2, 1)
	return contentH
}

// switchTab changes the active content tab, saving and restoring per-tab scroll.
func (m *PRDetailModel) switchTab(tab ContentTab) {
	if m.activeTab == tab {
		return
	}
	// Save current scroll.
	switch m.activeTab {
	case TabDescription:
		m.descScroll = m.ContentScroll
	case TabDiff:
		m.diffScroll = m.ContentScroll
	case TabComments:
		m.commentsScroll = m.ContentScroll
	case TabCommits:
		m.commitsScroll = m.ContentScroll
	}
	// Load new scroll.
	switch tab {
	case TabDescription:
		m.ContentScroll = m.descScroll
	case TabDiff:
		m.ContentScroll = m.diffScroll
	case TabComments:
		m.ContentScroll = m.commentsScroll
	case TabCommits:
		m.ContentScroll = m.commitsScroll
	}
	m.activeTab = tab
	m.leftPanel.Focus = FocusContent
	m.resetCommentCursor()
	m.confirmDiscardAll = false
	if m.visual.Active {
		m.exitVisualMode()
	}
	if tab == TabDiff {
		m.ensureDiffCursor()
		m.scrollToCursor(scrollPadding)
	}
	if tab == TabCommits && !m.commitsLoaded && !m.commitsLoading && m.PRService != nil {
		m.commitsLoading = true
	}
	m.clampContentScroll()
}

// maxContentScroll returns the maximum valid content scroll value for the active tab.
func (m *PRDetailModel) maxContentScroll() int {
	cw := contentViewportWidth(m.rightPanelWidth())
	vh := m.contentViewportHeight()
	switch m.activeTab {
	case TabDescription:
		return max(0, len(m.descriptionLines(cw))-vh)
	case TabDiff:
		return max(0, m.diffSectionRowCount()-vh)
	case TabComments:
		cLines := m.commentLines(cw, m.commentCursor)
		return max(0, len(cLines)-vh)
	case TabCommits:
		return max(0, m.commitsSectionRowCount()-vh)
	}
	return 0
}

func (m *PRDetailModel) clampContentScroll() {
	m.ContentScroll = clamp(m.ContentScroll, 0, m.maxContentScroll())
}

// ensureFileVisible scrolls FilesScroll so FileIndex is visible.
// Accounts for top border constraints and Tab spacing.
func (m *PRDetailModel) ensureFileVisible() {
	filesH := m.bodyHeight() - computeCIHeight(m.bodyHeight(), len(m.leftPanel.Checks))
	innerH := max(1, filesH-2)
	contentH := max(1, innerH-2)

	if m.leftPanel.Cursor < m.leftPanel.Scroll {
		m.leftPanel.Scroll = m.leftPanel.Cursor
	} else if m.leftPanel.Cursor >= m.leftPanel.Scroll+contentH {
		m.leftPanel.Scroll = m.leftPanel.Cursor - contentH + 1
	}
}

// ensureCIVisible scrolls CIScroll so CICursor is visible.
func (m *PRDetailModel) ensureCIVisible() {
	visible := m.ciVisibleRows()
	if m.leftPanel.CICursor < m.leftPanel.CIScroll {
		m.leftPanel.CIScroll = m.leftPanel.CICursor
	} else if m.leftPanel.CICursor >= m.leftPanel.CIScroll+visible {
		m.leftPanel.CIScroll = m.leftPanel.CICursor - visible + 1
	}
}

// handleRefresh clears cached data and refires both load commands with force=true
// in parallel. Clearing m.Detail and m.Diff causes the right viewport to show
// loading placeholders immediately, giving visual confirmation that a refresh is
// underway (analogous to the left-panel spinner).
func (m *PRDetailModel) handleRefresh() (*PRDetailModel, tea.Cmd) {
	if m.PRService == nil {
		return m, nil
	}
	// A full user refresh already reloads detail and all visible dependent
	// state, so it supersedes the SHA-less post-update fallback.
	m.reloadDependentsIfHeadUnknown = false
	m.Detail = nil
	m.Diff = nil
	m.DetailLoading = true
	m.DiffLoading = true
	m.leftPanel.Loading = true
	m.commits = nil
	m.commitsLoaded = false
	m.commitsLoading = false
	m.searchIndex = nil
	m.refreshSearchMatches()
	headSHA := m.Summary.HeadRefOID
	cmds_ := []tea.Cmd{
		m.loadPRDetailCmd(true),
		cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, true),
	}
	if m.activeTab == TabCommits || m.CommitMode {
		m.commitsLoading = true
		cmds_ = append(cmds_, cmds.LoadPRCommitsCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.HeadRefOID, true))
	}
	return m, tea.Batch(cmds_...)
}

func (m *PRDetailModel) handleCommitRefresh() (*PRDetailModel, tea.Cmd) {
	if m.PRService == nil {
		return m, nil
	}
	m.Diff = nil
	m.DiffLoading = true
	m.leftPanel.Loading = true
	m.searchIndex = nil
	m.refreshSearchMatches()
	return m, cmds.LoadCommitDiffCmd(m.PRService, m.Repo, m.Commit.SHA, true)
}

// handleResolveToggle toggles the resolved state of the thread at the current
// comment cursor. No-op if not in the Comments tab, cursor is inactive, or
// the active entry is not part of a review thread.
func (m *PRDetailModel) handleResolveToggle() tea.Cmd {
	if !m.isInCommentsSection() || m.commentCursor < 0 {
		return nil
	}
	if m.isAnyActionInProgress() {
		return nil
	}
	if m.PRService == nil || m.Detail == nil {
		return nil
	}
	entries := m.commentEntries()
	if m.commentCursor >= len(entries) {
		return nil
	}
	entry := entries[m.commentCursor]
	if entry.threadID == "" {
		return nil
	}

	thread := m.findThreadByID(entry.threadID)
	if thread == nil {
		return nil
	}

	// Stash previous state for revert-on-failure and reload reconciliation.
	m.pendingToggle = pendingToggleState{
		ThreadID:       thread.ID,
		PrevResolved:   thread.IsResolved,
		PrevResolver:   thread.ResolvedBy,
		TargetResolved: !thread.IsResolved,
	}

	if m.pendingToggle.TargetResolved {
		m.pendingToggle.TargetResolver = m.ViewerLogin
		// Optimistic flip: resolve → collapse (remove from expandedResolved).
		thread.IsResolved = true
		thread.ResolvedBy = m.pendingToggle.TargetResolver
		delete(m.expandedResolved, thread.ID)
	} else {
		// Optimistic flip: unresolve → expand (unresolved threads ignore the map).
		thread.IsResolved = false
		thread.ResolvedBy = ""
	}

	m.commentEntriesDirty = true
	m.resolveErr = ""

	// Adjust cursor after optimistic flip so it lands on the right entry.
	// On resolve: thread collapses to a summary → cursor on the summary.
	// On unresolve: thread expands → cursor on the first comment.
	entries = m.commentEntries()
	for i, e := range entries {
		if e.threadID == thread.ID {
			if m.pendingToggle.TargetResolved && e.isResolvedSummary {
				m.commentCursor = i
				break
			}
			if !m.pendingToggle.TargetResolved && e.isThreadStart {
				m.commentCursor = i
				break
			}
		}
	}

	if m.pendingToggle.TargetResolved {
		return cmds.ResolveThreadCmd(m.PRService, m.Repo, m.Summary.Number, thread.ID)
	}
	return cmds.UnresolveThreadCmd(m.PRService, m.Repo, m.Summary.Number, thread.ID)
}

// expandResolvedThread expands a collapsed resolved thread so its comments
// are visible. The cursor is moved to the first comment of the thread.
func (m *PRDetailModel) expandResolvedThread(threadID string) {
	if m.expandedResolved == nil {
		m.expandedResolved = make(map[string]bool)
	}
	m.expandedResolved[threadID] = true
	m.commentEntriesDirty = true

	// Move cursor to the first comment of the now-expanded thread.
	entries := m.commentEntries()
	for i, e := range entries {
		if e.threadID == threadID && !e.isResolvedSummary {
			m.commentCursor = i
			m.scrollToCommentCursor()
			return
		}
	}
}

// jumpUnresolvedThread moves the comment cursor to the first comment of the
// next (or previous) unresolved thread. No-op if there are no unresolved
// threads. Stops silently at boundaries.
func (m *PRDetailModel) jumpUnresolvedThread(forward bool) {
	entries := m.commentEntries()
	if len(entries) == 0 {
		return
	}

	// Build a list of unresolved thread-start entry indices.
	var unresolvedStarts []int
	seen := make(map[string]bool)
	for i, e := range entries {
		if e.threadID == "" || e.isResolvedSummary || e.isThreadReply {
			continue
		}
		if seen[e.threadID] {
			continue
		}
		seen[e.threadID] = true
		// Check if the thread is unresolved.
		thread := m.findThreadByID(e.threadID)
		if thread != nil && !thread.IsResolved {
			unresolvedStarts = append(unresolvedStarts, i)
		}
	}

	if len(unresolvedStarts) == 0 {
		return
	}

	cur := m.commentCursor
	if forward {
		for _, idx := range unresolvedStarts {
			if idx > cur {
				m.commentCursor = idx
				m.scrollToCommentCursor()
				return
			}
		}
		// Stop silently at the last unresolved thread.
	} else {
		for i := len(unresolvedStarts) - 1; i >= 0; i-- {
			idx := unresolvedStarts[i]
			if idx < cur {
				m.commentCursor = idx
				m.scrollToCommentCursor()
				return
			}
		}
		// Stop silently at the first unresolved thread.
	}
}
