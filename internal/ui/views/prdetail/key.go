package prdetail

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"
)

// handleKey routes keyboard input within the PR detail view.
func (m *PRDetailModel) handleKey(msg tea.KeyMsg) (*PRDetailModel, tea.Cmd) {
	if m.searchActive && m.handleSearchKey(msg) {
		m.LastKey = ""
		return m, nil
	}

	// Visual mode consumes only its own keys.
	if m.visual.Active {
		switch msg.String() {
		case "j", "down":
			m.expandVisualSelectionDown()
		case "k", "up":
			m.shrinkVisualSelectionUp()
		case "c":
			if m.PRService != nil {
				draft := m.findDraftForSelection()
				m.compose.Open(composeModeDraftInline, commentEntry{}, len(m.drafts))
				if draft != nil {
					m.compose.SetText(draft.Body)
				}
			}
		case "d":
			if m.removeDraftAt(m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine, m.visual.EndLine) {
				m.persistDrafts()
			}
		case "esc":
			m.exitVisualMode()
		}
		m.LastKey = ""
		return m, nil
	}

	// Confirm discard state.
	if m.confirmDiscardAll {
		switch msg.String() {
		case "y":
			m.drafts = nil
			m.rebuildDraftCovered()
			m.commentEntriesDirty = true
			m.persistDrafts()
			m.confirmDiscardAll = false
		case "n", "esc":
			m.confirmDiscardAll = false
		}
		m.LastKey = ""
		return m, nil
	}

	// Close/reopen flow state machine.
	if cmd := m.handleCloseKey(msg); cmd != nil {
		return m, cmd
	}

	// Merge flow state machine.
	if cmd := m.handleMergeKey(msg); cmd != nil {
		return m, cmd
	}

	// Edit prompt state — waiting for t/b/esc.
	if m.editPrompt != "" {
		switch msg.String() {
		case "t":
			m.editPrompt = ""
			m.compose.Open(composeModeEditTitle, commentEntry{}, len(m.drafts))
			m.compose.SetText(m.Summary.Title)
		case "b":
			m.editPrompt = ""
			m.compose.Open(composeModeEditBody, commentEntry{}, len(m.drafts))
			m.compose.SetText(m.Detail.BodyExcerpt)
		case "esc":
			m.editPrompt = ""
		}
		m.LastKey = ""
		return m, nil
	}

	switch msg.String() {
	case "?":
		return m, func() tea.Msg { return ToggleKeymapOverlayMsg{} }
	case "/":
		m.activateSearch()
		return m, nil
	case "n", "N":
		// Search navigation is only meaningful while searchActive=true.
		return m, nil
	case "esc":
		if m.CommitMode {
			return m, m.emitBackToPRDetail()
		}
		// Esc cycles: Content → Files → Dashboard
		if m.leftPanel.Focus == FocusContent && m.Width >= MinWidthForSidebar {
			m.leftPanel.Focus = FocusFiles
			m.resetCommentCursor()
		} else if m.leftPanel.Focus == FocusCI && m.Width >= MinWidthForSidebar {
			m.leftPanel.Focus = FocusFiles
		} else {
			return m, m.emitBackToDashboard()
		}
	case "q":
		if m.CommitMode {
			return m, m.emitBackToPRDetail()
		}
		return m, m.emitBackToDashboard()
	case "x":
		if m.CommitMode {
			return m, nil
		}
		return m, m.handleCloseStart()
	case "e":
		if m.CommitMode {
			return m, nil
		}
		if m.PRService != nil && m.Detail != nil && !m.isAnyActionInProgress() {
			m.editPrompt = "Edit: [t]itle or [b]ody?"
		}
		return m, nil
	case "R":
		if m.CommitMode {
			return m.handleCommitRefresh()
		}
		return m.handleRefresh()
	case "C":
		if m.CommitMode {
			return m, nil
		}
		if m.PRService != nil {
			m.compose.Open(composeModeNew, commentEntry{}, len(m.drafts))
		}
		return m, nil
	case "a":
		if m.CommitMode {
			return m, nil
		}
		if m.PRService != nil {
			m.compose.Open(composeModeApprove, commentEntry{}, len(m.drafts))
		}
		return m, nil
	case "b":
		if m.CommitMode {
			return m, nil
		}
		return m, m.handleCheckout()
	case "v":
		if m.CommitMode {
			return m, nil
		}
		if m.PRService != nil {
			m.compose.Open(composeModeReviewComment, commentEntry{}, len(m.drafts))
		}
		return m, nil
	case "m":
		return m, m.handleResolveToggle()
	case "r":
		if m.CommitMode {
			return m, nil
		}
		if m.PRService != nil && m.commentCursor >= 0 {
			entries := m.commentEntries()
			if m.commentCursor < len(entries) {
				entry := entries[m.commentCursor]
				if entry.isDraft {
					// Re-open draft inline for editing.
					m.compose.Open(composeModeDraftInline, commentEntry{}, len(m.drafts))
					m.compose.SetText(entry.body)
				} else {
					m.compose.Open(composeModeReply, entry, len(m.drafts))
				}
			}
		}
		return m, nil
	case " ":
		if m.CommitMode {
			return m, nil
		}
		if m.isInDiffSection() {
			m.enterVisualMode()
		}
		return m, nil
	case "D":
		if m.CommitMode {
			return m, nil
		}
		if len(m.drafts) > 0 {
			m.confirmDiscardAll = true
		}
		return m, nil
	case "o":
		if m.CommitMode {
			return m, m.emitOpenBrowserCommit()
		}
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			return m, m.emitOpenBrowserCommit()
		}
		return m, m.emitOpenBrowser()
	case "y":
		if m.CommitMode && m.isInDiffSection() {
			return m, m.emitCopyCommitPermalink()
		}
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			return m, m.emitCopyCommitSHA()
		}
	case "tab":
		m.cycleForward()
		m.resetCommentCursor()
	case "shift+tab":
		m.cycleBackward()
		m.resetCommentCursor()
	case "j", "down":
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments {
			m.moveCursorNextComment()
			return m, nil
		}
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			m.moveCommitCursor(1)
			return m, nil
		}
		if m.isInDiffSection() {
			m.ensureDiffCursor()
			m.moveCursorDown()
			m.scrollToCursor(scrollPadding)
			return m, nil
		}
		m.scrollDown()
	case "k", "up":
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments && m.commentCursor >= 0 {
			m.moveCursorPrevComment()
			return m, nil
		}
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			m.moveCommitCursor(-1)
			return m, nil
		}
		if m.isInDiffSection() {
			m.ensureDiffCursor()
			m.moveCursorUp()
			m.scrollToCursor(scrollPadding)
			return m, nil
		}
		m.scrollUp()
		if m.activeTab != TabComments {
			m.resetCommentCursor()
		}
	case "J":
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			m.moveCommitCursor(5)
			return m, nil
		}
		if m.isInDiffSection() {
			m.ensureDiffCursor()
			m.moveCursorBy(5)
			m.scrollToCursor(scrollPadding)
			return m, nil
		}
	case "K":
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			m.moveCommitCursor(-5)
			return m, nil
		}
		if m.isInDiffSection() {
			m.ensureDiffCursor()
			m.moveCursorBy(-5)
			m.scrollToCursor(scrollPadding)
			return m, nil
		}
	case "enter":
		if m.leftPanel.Focus == FocusFiles {
			m.jumpToFile(m.leftPanel.Cursor)
		} else if m.leftPanel.Focus == FocusCI {
			return m, m.emitOpenBrowserCI()
		} else if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments && m.commentCursor >= 0 {
			// Enter on a collapsed resolved summary expands it; otherwise jump to code.
			entries := m.commentEntries()
			if m.commentCursor < len(entries) && entries[m.commentCursor].isResolvedSummary {
				m.expandResolvedThread(entries[m.commentCursor].threadID)
				return m, nil
			}
			m.jumpToCommentCode()
		} else if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			return m, m.emitOpenCommitDetail()
		}
	case "h", "left":
		m.jumpFileViewer()
	case "l", "right":
		m.jumpDiffViewer()
	case "shift+h":
		m.jumpPrevFile()
	case "shift+l":
		m.jumpNextFile()
	case "[", "]":
		if m.isInCommentsSection() {
			m.jumpUnresolvedThread(msg.String() == "]")
		}
	case "1":
		if !m.CommitMode {
			m.switchTab(TabDescription)
		}
	case "2":
		m.switchTab(TabDiff)
	case "3":
		if !m.CommitMode {
			m.switchTab(TabComments)
		}
	case "4":
		if !m.CommitMode {
			needLoad := !m.commitsLoaded && !m.commitsLoading && m.PRService != nil
			m.switchTab(TabCommits)
			if needLoad {
				return m, cmds.LoadPRCommitsCmd(m.PRService, m.Repo, m.Summary.Number, false)
			}
		}
	case "g":
		if m.LastKey == "g" {
			if m.isInDiffSection() {
				if len(m.navigableLines) > 0 {
					m.setDiffCursor(m.navigableLines[0])
					m.ContentScroll = 0
					m.syncFilePanelToCursor()
				}
			} else if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments {
				entries := m.commentEntries()
				if len(entries) > 0 {
					m.commentCursor = 0
					m.scrollToCommentCursor()
				}
			} else if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
				m.commitCursor = 0
				m.ContentScroll = 0
			} else {
				m.scrollToTop()
			}
			m.LastKey = ""
			return m, nil
		}
		m.LastKey = "g"
		return m, nil
	case "G":
		if m.isInDiffSection() {
			if len(m.navigableLines) > 0 {
				m.setDiffCursor(m.navigableLines[len(m.navigableLines)-1])
				m.scrollToCursor(scrollPadding)
				m.syncFilePanelToCursor()
			}
		} else if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments {
			entries := m.commentEntries()
			if len(entries) > 0 {
				m.commentCursor = len(entries) - 1
				m.scrollToCommentCursor()
			}
		} else if m.leftPanel.Focus == FocusContent && m.activeTab == TabCommits {
			m.commitCursor = max(0, len(m.commits)-1)
			cursorRow := m.commitCursor * 3
			vh := m.contentViewportHeight()
			if cursorRow >= m.ContentScroll+vh {
				m.ContentScroll = cursorRow - vh + 1
			}
			m.clampContentScroll()
		} else {
			m.scrollToBottom()
		}
	case "ctrl+d":
		if m.isInDiffSection() {
			m.ensureDiffCursor()
			m.moveCursorBy(m.contentViewportHeight() / 2)
			m.scrollToCursor(scrollPadding)
			return m, nil
		}
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments {
			m.moveCommentCursorBy(m.contentViewportHeight() / 2)
			return m, nil
		}
		m.scrollHalfPageDown()
	case "ctrl+u":
		if m.isInDiffSection() {
			m.ensureDiffCursor()
			m.moveCursorBy(-(m.contentViewportHeight() / 2))
			m.scrollToCursor(scrollPadding)
			return m, nil
		}
		if m.leftPanel.Focus == FocusContent && m.activeTab == TabComments {
			m.moveCommentCursorBy(-(m.contentViewportHeight() / 2))
			return m, nil
		}
		m.scrollHalfPageUp()
	}
	if msg.String() != "g" {
		m.LastKey = ""
	}
	return m, nil
}
