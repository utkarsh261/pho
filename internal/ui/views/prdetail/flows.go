package prdetail

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

// StatusHint returns the status bar hint text for the current state.
func (m *PRDetailModel) StatusHint() string {
	if m.visual.Active {
		return "j/k: Select lines | c: Comment | d: Discard | Esc: Exit visual"
	}
	if m.confirmDiscardAll {
		return fmt.Sprintf("Discard all %d drafts? (y/n)", len(m.drafts))
	}
	switch m.mergeStep {
	case mergeStepSelectMethod:
		return fmt.Sprintf("Merge #%d: [s]quash [r]ebase [M]erge [esc]cancel", m.Summary.Number)
	case mergeStepConfirm:
		return fmt.Sprintf("%s and merge #%d? (y/n)", strings.ToLower(m.mergeMethod), m.Summary.Number)
	case mergeStepChecking:
		return "Checking mergeability..."
	case mergeStepExecuting:
		return "Merging..."
	}
	if m.mergeErr != "" {
		return m.mergeErr
	}
	switch m.updateStep {
	case updateStepConfirm:
		return fmt.Sprintf("Update branch #%d with base? (y/n)", m.Summary.Number)
	case updateStepExecuting:
		return "Updating branch..."
	}
	if m.updateErr != "" {
		return m.updateErr
	}
	if m.resolveErr != "" {
		return m.resolveErr
	}
	switch m.closeStep {
	case closeStepConfirm:
		action := "Close"
		if m.closeTarget == "REOPEN" {
			action = "Reopen"
		}
		return fmt.Sprintf("%s #%d? (y/n)", action, m.Summary.Number)
	case closeStepExecuting:
		if m.closeTarget == "CLOSE" {
			return "Closing..."
		}
		return "Reopening..."
	}
	if m.closeErr != "" {
		return m.closeErr
	}
	if m.checkoutInFlight {
		return "Checking out " + m.Summary.HeadRefName + "..."
	}
	if m.checkoutErr != "" {
		return m.checkoutErr
	}
	if m.checkoutStatus != "" {
		return m.checkoutStatus
	}
	if m.editPrompt != "" {
		return m.editPrompt
	}
	if m.editPosting {
		return "Updating PR..."
	}
	if m.editErr != "" {
		return m.editErr
	}
	if m.CommitMode {
		if m.searchActive {
			return fmt.Sprintf("Search: %s  (%d/%d)  | Enter: commit  | Esc: clear", m.searchQuery, m.searchCursor+1, len(m.searchMatches))
		}
		if m.leftPanel.Focus == FocusFiles {
			return "j/k: Move | Enter: Jump | l: Focus diff | ?: Keymap"
		}
		return "j/k: Scroll | h: Focus files | o: Open browser | y: Copy SHA | q: Back | ?: Keymap"
	}
	hint := "Tab: Switch | Space: Visual | 1/2/3/4: Tabs | R: Refresh | /: Search | ?"
	if len(m.drafts) > 0 {
		hint = "Tab: Switch | Space: Visual | 1/2/3/4: Tabs | R: Refresh | /: Search | D: Discard drafts | ?"
	}
	return hint
}

// handleMergeKey routes keys when the merge flow is active.
// Returns a non-nil tea.Cmd if a merge action was triggered.
func (m *PRDetailModel) handleMergeKey(msg tea.KeyMsg) tea.Cmd {
	switch m.mergeStep {
	case mergeStepSelectMethod:
		switch msg.String() {
		case "s":
			m.mergeMethod = "SQUASH"
			m.mergeStep = mergeStepConfirm
		case "r":
			m.mergeMethod = "REBASE"
			m.mergeStep = mergeStepConfirm
		case "M":
			m.mergeMethod = "MERGE"
			m.mergeStep = mergeStepConfirm
		case "esc", "n":
			m.resetMergeFlow()
		}
		return func() tea.Msg { return nil }
	case mergeStepConfirm:
		switch msg.String() {
		case "y":
			m.mergeStep = mergeStepChecking
			return cmds.CheckMergeableCmd(m.PRService, m.mergeRepo, m.Summary.Number)
		case "n", "esc":
			m.resetMergeFlow()
		}
		return func() tea.Msg { return nil }
	case mergeStepChecking:
		// The check is just a query; user can cancel and retry.
		if msg.String() == "esc" {
			m.resetMergeFlow()
		}
		return func() tea.Msg { return nil }
	case mergeStepExecuting:
		// Merge mutation is in flight and cannot be cancelled.
		// User already confirmed with 'y'; commit is in progress.
		return func() tea.Msg { return nil }
	case mergeStepNone:
		if msg.String() == "M" {
			if m.PRService == nil {
				return func() tea.Msg { return nil }
			}
			if m.Detail == nil {
				// Still loading; silently ignore.
				return func() tea.Msg { return nil }
			}
			if m.isAnyActionInProgress() {
				return func() tea.Msg { return nil }
			}
			if m.Detail.Mergeable != "MERGEABLE" {
				m.mergeErr = "PR is not mergeable (" + humanizeMergeState(m.Detail.MergeState) + ")"
				return func() tea.Msg { return nil }
			}
			m.mergeErr = ""
			m.mergeStep = mergeStepSelectMethod
			m.mergeRepo = m.Repo
			m.mergePRID = m.Summary.ID
			return func() tea.Msg { return nil }
		}
		// Any other key while mergeErr is showing clears the error.
		if m.mergeErr != "" {
			m.mergeErr = ""
		}
	}
	return nil
}

func (m *PRDetailModel) resetMergeFlow() {
	m.mergeStep = mergeStepNone
	m.mergeMethod = ""
	m.mergeErr = ""
}

// handleUpdateKey routes keys when the "Update branch" flow is active.
// Returns a non-nil tea.Cmd if an update action was triggered.
func (m *PRDetailModel) handleUpdateKey(msg tea.KeyMsg) tea.Cmd {
	switch m.updateStep {
	case updateStepConfirm:
		switch msg.String() {
		case "y":
			m.updateStep = updateStepExecuting
			// Pass empty expected_head_sha so the server uses the PR branch's
			// current HEAD. Optimistic concurrency is not worth the false-422
			// risk on a TUI with SWR-cached detail.
			return cmds.UpdateBranchCmd(m.PRService, m.updateRepo, m.Summary.Number, "")
		case "n", "esc":
			m.resetUpdateFlow()
		}
		return func() tea.Msg { return nil }
	case updateStepExecuting:
		// Mutation is in flight; cannot be cancelled.
		return func() tea.Msg { return nil }
	case updateStepNone:
		if msg.String() == "U" {
			if m.PRService == nil || m.Detail == nil {
				return func() tea.Msg { return nil }
			}
			if m.isAnyActionInProgress() {
				return func() tea.Msg { return nil }
			}
			if m.Detail.State != "OPEN" {
				m.updateErr = "Cannot update branch: PR is " + strings.ToLower(string(m.Detail.State))
				return func() tea.Msg { return nil }
			}
			if m.Detail.MergeState != "BEHIND" {
				m.updateErr = "PR is not behind base (" + humanizeMergeState(m.Detail.MergeState) + ")"
				return func() tea.Msg { return nil }
			}
			m.updateErr = ""
			m.updateStep = updateStepConfirm
			m.updateRepo = m.Repo
			return func() tea.Msg { return nil }
		}
		// Any other key while updateErr is showing clears the error.
		if m.updateErr != "" {
			m.updateErr = ""
		}
	}
	return nil
}

func (m *PRDetailModel) resetUpdateFlow() {
	m.updateStep = updateStepNone
	m.updateErr = ""
	m.updateRepo = domain.Repository{}
}

// isAnyActionInProgress reports whether a transient action is active.
func (m *PRDetailModel) isAnyActionInProgress() bool {
	return m.mergeStep != mergeStepNone ||
		m.updateStep != updateStepNone ||
		m.closeStep != closeStepNone ||
		m.checkoutInFlight ||
		m.editPosting ||
		m.editPrompt != "" ||
		m.confirmDiscardAll ||
		m.pendingToggle.active()
}

// handleCloseStart initiates the close/reopen flow when x is pressed.
func (m *PRDetailModel) handleCloseStart() tea.Cmd {
	if m.PRService == nil {
		return nil
	}
	if m.Detail == nil {
		return nil
	}
	if m.isAnyActionInProgress() {
		return nil
	}
	state := m.Detail.State
	if state == domain.PRStateMerged {
		m.closeErr = "Merged PRs cannot be closed"
		return nil
	}
	m.closeErr = ""
	if state == domain.PRStateOpen {
		m.closeTarget = "CLOSE"
	} else {
		m.closeTarget = "REOPEN"
	}
	m.closeStep = closeStepConfirm
	return nil
}

// handleCloseKey routes keys when the close/reopen flow is active.
func (m *PRDetailModel) handleCloseKey(msg tea.KeyMsg) tea.Cmd {
	switch m.closeStep {
	case closeStepConfirm:
		switch msg.String() {
		case "y":
			m.closeStep = closeStepExecuting
			if m.closeTarget == "CLOSE" {
				return cmds.ClosePRCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.ID)
			}
			return cmds.ReopenPRCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.ID)
		case "n", "esc":
			m.resetCloseFlow()
		}
		return func() tea.Msg { return nil }
	case closeStepExecuting:
		return func() tea.Msg { return nil }
	case closeStepNone:
		if msg.String() == "x" {
			m.handleCloseStart()
			return func() tea.Msg { return nil }
		}
		if m.closeErr != "" {
			m.closeErr = ""
		}
	}
	return nil
}

func (m *PRDetailModel) resetCloseFlow() {
	m.closeStep = closeStepNone
	m.closeTarget = ""
	m.closeErr = ""
}

func (m *PRDetailModel) emitBackToDashboard() tea.Cmd {
	return func() tea.Msg { return BackToDashboard{} }
}

func (m *PRDetailModel) emitBackToPRDetail() tea.Cmd {
	return func() tea.Msg { return BackToPRDetail{} }
}

func (m *PRDetailModel) emitOpenBrowserCommit() tea.Cmd {
	if m.CommitMode {
		return func() tea.Msg {
			return OpenBrowserCommit{Repo: m.Repo, SHA: m.Commit.SHA}
		}
	}
	// Commits tab in PR detail mode
	if m.commitCursor >= 0 && m.commitCursor < len(m.commits) {
		c := m.commits[m.commitCursor]
		return func() tea.Msg {
			return OpenBrowserCommit{Repo: m.Repo, SHA: c.SHA}
		}
	}
	return nil
}

func (m *PRDetailModel) emitCopyCommitPermalink() tea.Cmd {
	if !m.validDiffCursor() {
		return nil
	}
	fi, hi, li := m.diffCursor.FileIdx, m.diffCursor.HunkIdx, m.diffCursor.LineIdx
	if m.Diff == nil || fi >= len(m.Diff.Files) {
		return nil
	}
	f := &m.Diff.Files[fi]
	if hi >= len(f.Hunks) {
		return nil
	}
	h := &f.Hunks[hi]
	if li >= len(h.Lines) {
		return nil
	}
	dl := &h.Lines[li]
	if len(dl.Anchors) == 0 {
		return nil
	}
	anchor := dl.Anchors[0]
	url := fmt.Sprintf("https://%s/%s/blob/%s/%s#L%d",
		m.Repo.Host, m.Repo.FullName, m.Commit.SHA, anchor.Path, *anchor.Line)
	return func() tea.Msg {
		return CopyCommitPermalink{URL: url}
	}
}

func (m *PRDetailModel) emitOpenBrowser() tea.Cmd {
	return func() tea.Msg {
		return OpenBrowserPR{Repo: m.Summary.Repo, Number: m.Summary.Number}
	}
}

func (m *PRDetailModel) emitOpenBrowserCI() tea.Cmd {
	if m.leftPanel.CICursor < 0 || m.leftPanel.CICursor >= len(m.leftPanel.Checks) {
		return nil
	}
	url := m.leftPanel.Checks[m.leftPanel.CICursor].URL
	if url == "" {
		return nil
	}
	return func() tea.Msg {
		return OpenBrowserCI{URL: url}
	}
}

// handleCheckout initiates the async checkout of the PR branch.
func (m *PRDetailModel) handleCheckout() tea.Cmd {
	if m.checkoutInFlight {
		return nil
	}
	if m.isAnyActionInProgress() {
		return nil
	}
	if m.Repo.LocalPath == "" {
		m.checkoutErr = "Not a local repo"
		return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return checkoutClearMsg{}
		})
	}
	m.checkoutInFlight = true
	return cmds.CheckoutBranchCmd(m.Repo, m.Summary.Number, m.Summary.HeadRefName, m.Summary.IsCrossRepository)
}

// pendingToggleState tracks an in-flight resolve/unresolve mutation so the
// UI can optimistically flip the thread state and reconcile after reload.
type pendingToggleState struct {
	ThreadID       string
	PrevResolved   bool
	PrevResolver   string
	TargetResolved bool
	TargetResolver string
}

func (p pendingToggleState) active() bool {
	return p.ThreadID != ""
}

// findThreadByID returns a pointer to the thread with the given ID, or nil.
func (m *PRDetailModel) findThreadByID(threadID string) *domain.PreviewReviewThread {
	if m.Detail == nil {
		return nil
	}
	for i := range m.Detail.ReviewThreads {
		if m.Detail.ReviewThreads[i].ID == threadID {
			return &m.Detail.ReviewThreads[i]
		}
	}
	return nil
}

// unresolvedThreadCount returns the number of unresolved review threads.
func (m *PRDetailModel) unresolvedThreadCount() int {
	if m.Detail == nil {
		return 0
	}
	count := 0
	for _, t := range m.Detail.ReviewThreads {
		if !t.IsResolved {
			count++
		}
	}
	return count
}

// isThreadExpanded returns true if a resolved thread should render expanded.
func (m *PRDetailModel) isThreadExpanded(threadID string, isResolved bool) bool {
	if !isResolved {
		return true // Unresolved threads are always expanded.
	}
	return m.expandedResolved[threadID]
}
