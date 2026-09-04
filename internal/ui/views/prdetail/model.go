// Package prdetail implements the PR detail view model.
// It manages the PR detail state and handles keyboard routing within
// the PR detail view.
package prdetail

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/model"
	diffsearch "github.com/utkarsh261/pho/internal/diff/search"
	"github.com/utkarsh261/pho/internal/domain"
	pholog "github.com/utkarsh261/pho/internal/log"
	"github.com/utkarsh261/pho/internal/ui/markdown"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

type composeSuccessDismissMsg struct{}

type editorDoneMsg struct {
	path string
	err  error
}

type checkoutResultMsg struct {
	branch  string
	stashed bool
	err     error
}

type checkoutClearMsg struct{}

func (m *PRDetailModel) rightPanelWidth() int {
	if m.Width >= MinWidthForSidebar {
		return max(m.Width-LeftPanelWidth-2, 10)
	}
	return m.Width
}

// headerContentRows returns the number of text rows inside the header box
// (excluding borders): one, plus a second when the reviewer strip renders.
func (m *PRDetailModel) headerContentRows() int {
	if !m.CommitMode && m.renderReviewerStrip(max(m.Width-2, 1)) != "" {
		return 2
	}
	return 1
}

// effectiveBodyH returns the body height available for the left/right panels,
// accounting for the actual header height and the compose pane when it is
// open (3 rows: top border + 2 content rows).
func (m *PRDetailModel) effectiveBodyH() int {
	bodyH := max(m.Height-m.headerContentRows()-2, 1)
	if m.compose.active {
		return max(bodyH-3, 1)
	}
	return bodyH
}

// contentViewportHeight returns the number of visible rows in the content text area.
// Derived from the terminal height by subtracting the header box, the tab headBox,
// and body-box borders.
func (m *PRDetailModel) contentViewportHeight() int {
	innerH := max(m.effectiveBodyH()-4, 1)
	return max(innerH-2, 1)
}

// ContentTab identifies the active tab in the right content panel.
type ContentTab int

const (
	TabDescription ContentTab = iota
	TabDiff
	TabComments
	TabCommits
)

// visualModeState tracks the active visual-mode selection in the diff.
type visualModeState struct {
	Active    bool
	FileIdx   int
	HunkIdx   int
	StartLine int // index into hunk.Lines
	EndLine   int // index into hunk.Lines (inclusive)
}

// diffCursorLine identifies a single diff line for cursor-based navigation.
// FileIdx==-1 means the cursor is invalid/unset.
type diffCursorLine struct {
	FileIdx int
	HunkIdx int
	LineIdx int
}

// scrollPadding is the number of lines of context kept between the cursor
// and the viewport edge during auto-scroll.
const scrollPadding = 4

type PRDetailModel struct {
	Summary domain.PullRequestSummary

	Detail *domain.PRPreviewSnapshot

	Diff *model.DiffModel

	DetailLoading bool
	DiffLoading   bool

	// LoadErr holds the initial load's failure; the view shows an error panel
	// until a retry or reload clears it.
	LoadErr error

	DetailFromCache bool

	Width  int
	Height int

	PRService cmds.PRService
	Repo      domain.Repository
	Log       *pholog.Logger

	ContentScroll int

	LastKey string

	searchActive  bool
	searchQuery   string
	searchIndex   *diffsearch.DiffSearchIndex
	searchMatches []diffsearch.Match
	searchCursor  int
	searchCommit  bool

	commentCursor       int // -1 = none, 0..n-1 = index of focused comment entry
	postedComment       bool
	postedCommentTarget commentEntry // remember what was replied to, for scroll-to

	compose ComposeModel

	leftPanel LeftPanelModel
	spinner   spinner.Model

	theme      *theme.Theme
	mdRenderer *markdown.Renderer

	// cachedBody holds the rendered body (left+right panels) from the last
	// frame where compose was not active. While compose is open, the body
	// doesn't change (user is typing, not scrolling), so reusing it makes
	// every keystroke render O(1) instead of re-rendering all markdown.
	cachedBody       string
	cachedBodyWidth  int
	cachedBodyHeight int

	// Content tabs
	activeTab      ContentTab
	descScroll     int
	diffScroll     int
	commentsScroll int
	commitsScroll  int

	// Commits tab state
	commits        []domain.Commit
	commitsLoading bool
	commitCursor   int
	commitsLoaded  bool

	// Diff cursor (line-by-line navigation in Diff tab)
	diffCursor diffCursorLine

	// Flat index of all navigable diff lines (excludes binary files).
	// Rebuilt whenever Diff changes; invalidated alongside diffCursor.
	navigableLines []diffCursorLine // ordered by display position
	navigableRows  []int            // parallel display rows for each navigable line
	navIdxMap      map[diffCursorLine]int
	navIdx         int // current position in navigableLines; -1 = invalid

	// Inline review drafts
	visual            visualModeState
	drafts            []domain.DraftInlineComment
	draftHeadSHA      string
	draftsStale       bool
	inlineDraftStale  bool
	confirmDiscardAll bool
	draftCovered      map[hunkLineKey]bool // precomputed for diff rendering

	// Diff indices (rebuilt when Diff changes)
	diffLineIndex   map[string]map[int]string            // path → line → raw text
	diffAnchorIndex map[string]map[int]map[string][3]int // path → line → side → {fileIdx, hunkIdx, lineIdx}

	// Comment entries cache (invalidated when Detail or drafts change)
	cachedCommentEntries []commentEntry
	commentEntriesDirty  bool

	// Merge flow state
	mergeStep   mergeStep
	mergeMethod string
	mergeErr    string
	mergeRepo   domain.Repository
	mergePRID   string

	// Update-branch flow state
	updateStep updateStep
	updateErr  string
	updateRepo domain.Repository
	// Set after an accepted update-branch mutation. The next successful detail
	// response uses this to reconcile dependent state on hosts that cannot
	// provide a head SHA; hosts with a SHA continue to use normal change
	// detection.
	reloadDependentsIfHeadUnknown bool

	detailRequestID uint64

	// Close/reopen flow state
	closeStep   closeStep
	closeTarget string // "CLOSE" or "REOPEN"
	closeErr    string

	// Checkout state
	checkoutInFlight bool
	checkoutStatus   string
	checkoutErr      string

	// Edit title/body state
	editPrompt  string // "Edit: [t]itle or [b]ody?" when waiting for choice
	editPosting bool
	editErr     string

	// Resolve/unresolve thread state
	expandedResolved map[string]bool // threadID → expanded (resolved threads only)
	pendingToggle    pendingToggleState
	resolveErr       string

	// ViewerLogin is the authenticated user's login, used for optimistic
	// resolver display. May be empty if viewer resolution hasn't completed.
	ViewerLogin string

	// Commit mode: when true, this model shows a single commit diff instead of
	// a full PR. Only the Diff tab is shown, no Desc/Comments/Commits tabs,
	// no CI section, and the header shows commit info.
	CommitMode bool
	Commit     domain.Commit
}

// mergeStep tracks the PR merge workflow state.
type mergeStep int

const (
	mergeStepNone mergeStep = iota
	mergeStepSelectMethod
	mergeStepConfirm
	mergeStepChecking
	mergeStepExecuting
)

// updateStep tracks the "Update branch" workflow state.
type updateStep int

const (
	updateStepNone updateStep = iota
	updateStepConfirm
	updateStepExecuting
)

// closeStep tracks the close/reopen workflow state.
type closeStep int

const (
	closeStepNone closeStep = iota
	closeStepConfirm
	closeStepExecuting
)

// hunkLineKey identifies a specific line within a hunk for draft highlighting.
type hunkLineKey struct{ fileIdx, hunkIdx, lineIdx int }

func (m *PRDetailModel) isLoading() bool {
	return m.DetailLoading || m.DiffLoading || m.commitsLoading || m.leftPanel.Loading
}

func (m *PRDetailModel) matchesPR(host, repo string, number int) bool {
	if number != 0 && number != m.Summary.Number {
		return false
	}
	if host != "" && m.Repo.Host != "" && !strings.EqualFold(host, m.Repo.Host) {
		return false
	}
	if repo != "" && !strings.EqualFold(repo, m.Repo.FullName) && !strings.EqualFold(repo, m.Summary.Repo) {
		return false
	}
	return true
}

func (m *PRDetailModel) loadPRDetailCmd(force bool) tea.Cmd {
	if m.PRService == nil {
		return nil
	}
	m.detailRequestID++
	return cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, force, m.detailRequestID)
}

func (m *PRDetailModel) reloadHeadDependentState(headSHA string) tea.Cmd {
	reloadCommits := m.commitsLoaded || m.commitsLoading || m.activeTab == TabCommits
	// Every diff-derived structure is tied to the previous head SHA. Drop it
	// before refetching so stale lines, anchors, and commits cannot be relabeled
	// as belonging to a newly observed head.
	m.Diff = nil
	m.DiffLoading = true
	m.leftPanel.Files = nil
	m.leftPanel.Loading = true
	m.navigableLines = nil
	m.navigableRows = nil
	m.navIdxMap = nil
	m.invalidateDiffCursor()
	m.visual.Active = false
	m.diffLineIndex = nil
	m.diffAnchorIndex = nil
	m.searchIndex = nil
	m.refreshSearchMatches()
	m.commits = nil
	m.commitsLoaded = false
	m.commitsLoading = reloadCommits

	if m.PRService == nil {
		m.DiffLoading = false
		m.leftPanel.Loading = false
		m.commitsLoading = false
		return nil
	}
	cmdsOut := []tea.Cmd{
		cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, true),
	}
	if reloadCommits {
		cmdsOut = append(cmdsOut,
			cmds.LoadPRCommitsCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, true))
	}
	return tea.Batch(cmdsOut...)
}

// NewModel creates a new PRDetailModel for the given PR.
func NewModel(summary domain.PullRequestSummary, repo domain.Repository, prService cmds.PRService) *PRDetailModel {
	loading := prService != nil
	s := spinner.New(spinner.WithSpinner(spinner.Points))
	s.Spinner.FPS = time.Millisecond * 100

	m := &PRDetailModel{
		Summary:       summary,
		PRService:     prService,
		Repo:          repo,
		DetailLoading: loading,
		DiffLoading:   loading,
		spinner:       s,
		commentCursor: -1,
		diffCursor:    diffCursorLine{FileIdx: -1},
		compose:       newComposeModel(nil),
		activeTab:     TabDescription,
	}
	m.leftPanel.Loading = loading
	m.leftPanel.Focus = FocusContent
	m.leftPanel.LastOpenedIndex = 0
	m.leftPanel.CICursor = 0
	m.mdRenderer = markdown.New()
	return m
}

// NewCommitModel creates a PRDetailModel configured for viewing a single commit's diff.
// It starts on the Diff tab with no CI panel and skips loading PR detail/comments.
func NewCommitModel(repo domain.Repository, commit domain.Commit, prService cmds.PRService) *PRDetailModel {
	s := spinner.New(spinner.WithSpinner(spinner.Points))
	s.Spinner.FPS = time.Millisecond * 100

	summary := domain.PullRequestSummary{
		Repo:   repo.FullName,
		Number: 0,
		Title:  commit.MessageHeadline,
		Author: commit.AuthorLogin,
	}

	m := &PRDetailModel{
		Summary:     summary,
		PRService:   prService,
		Repo:        repo,
		DiffLoading: prService != nil,
		spinner:     s,
		diffCursor:  diffCursorLine{FileIdx: -1},
		compose:     newComposeModel(nil),
		activeTab:   TabDiff,
		CommitMode:  true,
		Commit:      commit,
	}
	m.leftPanel.Loading = prService != nil
	m.leftPanel.HideCI = true
	m.leftPanel.Focus = FocusContent
	m.leftPanel.LastOpenedIndex = 0
	return m
}

// SetDiffFiles sets the file list in the left panel sidebar (for commit mode setup).
func (m *PRDetailModel) SetDiffFiles(files []model.DiffFile) {
	m.leftPanel.Files = files
}

// SetTheme applies a theme to the PR detail model.
func (m *PRDetailModel) SetTheme(th *theme.Theme) {
	m.theme = th
	m.leftPanel.SetTheme(th)
	m.compose.theme = th
	if th != nil {
		m.spinner.Style = lipgloss.NewStyle().Foreground(th.Warning)
	}
}

// Init fires the parallel load commands for PR detail and diff.
func (m *PRDetailModel) Init() tea.Cmd {
	var cmdsOut []tea.Cmd
	if m.isLoading() {
		cmdsOut = append(cmdsOut, m.spinner.Tick)
	}
	if m.PRService != nil {
		if m.CommitMode {
			cmdsOut = append(cmdsOut,
				cmds.LoadCommitDiffCmd(m.PRService, m.Repo, m.Commit.SHA, false),
			)
		} else {
			headSHA := m.Summary.HeadRefOID
			cmdsOut = append(cmdsOut,
				m.loadPRDetailCmd(false),
				cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, false),
			)
		}
	}
	return tea.Batch(cmdsOut...)
}

// Update handles messages and key events within the PR detail view.
func (m *PRDetailModel) Update(msg tea.Msg) (*PRDetailModel, tea.Cmd) {
	var spinCmd tea.Cmd
	if m.isLoading() {
		m.spinner, spinCmd = m.spinner.Update(msg)
	}

	// Forward all messages to compose so textinput receives tick events for cursor blink.
	var composeCmd tea.Cmd
	composeConsumedKey := m.compose.active
	composeWasIdle := m.compose.active && m.compose.status == composeStatusIdle
	m.compose, composeCmd = m.compose.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.PRDetailLoaded:
		if !m.matchesPR(msg.Host, msg.Repo, msg.Number) {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if msg.RequestID != 0 && msg.RequestID < m.detailRequestID {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if msg.RequestID > m.detailRequestID {
			m.detailRequestID = msg.RequestID
		}
		m.DetailLoading = false
		if msg.Err != nil {
			if m.Detail == nil {
				m.LoadErr = msg.Err
				// Nothing else will clear the sidebar spinners on a failed
				// first load.
				m.DiffLoading = false
				m.leftPanel.Loading = false
			}
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.LoadErr = nil
		previousHead := m.headSHA()
		m.Detail = &msg.Detail
		m.DetailFromCache = msg.FromCache
		if msg.Detail.HeadRefOID != "" {
			m.Summary.HeadRefOID = msg.Detail.HeadRefOID
		}
		// Deep-linked opens start from a bare {repo, number} summary; fill the
		// rest from the loaded snapshot without clobbering real values. The ID
		// is required by every mutation (comment, approve, merge, close).
		if m.Summary.ID == "" {
			m.Summary.ID = msg.Detail.ID
		}
		if m.Summary.Title == "" {
			m.Summary.Title = msg.Detail.Title
		}
		if m.Summary.Author == "" {
			m.Summary.Author = msg.Detail.Author
		}
		if m.Summary.State == "" {
			m.Summary.State = msg.Detail.State
		}
		if m.Summary.HeadRefName == "" {
			m.Summary.HeadRefName = msg.Detail.HeadRefName
		}
		m.commentEntriesDirty = true

		// Replication-lag mitigation: when a toggle is pending, force the
		// optimistic state for the toggled thread over the reload payload.
		if m.pendingToggle.active() {
			thread := m.findThreadByID(m.pendingToggle.ThreadID)
			if thread != nil {
				thread.IsResolved = m.pendingToggle.TargetResolved
				thread.ResolvedBy = m.pendingToggle.TargetResolver
			}
		}

		m.resetCommentCursor()
		// Sync checks into left panel.
		m.leftPanel.Checks = msg.Detail.Checks

		// Re-anchor cursor to the toggled thread after a resolve/unresolve reload.
		if m.pendingToggle.active() {
			toggleThreadID := m.pendingToggle.ThreadID
			m.pendingToggle = pendingToggleState{}
			entries := m.commentEntries()
			if m.activeTab == TabComments && len(entries) > 0 {
				idx := -1
				for i, e := range entries {
					if e.threadID == toggleThreadID {
						idx = i
						break
					}
				}
				if idx >= 0 {
					m.commentCursor = idx
					m.scrollToCommentCursor()
				}
			}
		}

		// Auto-scroll to the newly posted comment after a successful post.
		if m.postedComment {
			m.postedComment = false
			cw := contentViewportWidth(m.rightPanelWidth())
			entries := m.commentEntries()
			startRows := m.commentEntryStartRows(cw)
			if len(startRows) > 0 {
				target := m.postedCommentTarget
				idx := -1
				if target.threadID != "" {
					for i := len(entries) - 1; i >= 0; i-- {
						if entries[i].threadID == target.threadID {
							idx = i
							break
						}
					}
				}
				if idx < 0 {
					idx = len(startRows) - 1
				}
				entryTop := startRows[idx]
				entryH := m.entryRenderHeight(entries[idx], cw, entries, idx)
				endRow := entryTop + entryH
				vh := m.contentViewportHeight()
				targetScroll := max(endRow-vh+1, 0)
				m.switchTab(TabComments)
				m.ContentScroll = targetScroll
				m.clampContentScroll()
				m.commentCursor = idx
			}
			m.postedCommentTarget = commentEntry{}
		}

		headChanged := msg.Detail.HeadRefOID != "" && msg.Detail.HeadRefOID != previousHead
		if headChanged {
			if m.compose.active && m.compose.mode == composeModeDraftInline {
				m.inlineDraftStale = true
			}
			m.markDraftsStale(previousHead)
		}
		reloadUnknownHead := m.reloadDependentsIfHeadUnknown && msg.Detail.HeadRefOID == ""
		m.reloadDependentsIfHeadUnknown = false
		if reloadUnknownHead {
			// Do not label freshly fetched diff/commit data with a pre-update SHA
			// when the host cannot report the current one.
			m.Summary.HeadRefOID = ""
		}

		var out []tea.Cmd
		out = append(out, spinCmd, composeCmd)
		if headChanged || reloadUnknownHead {
			out = append(out, m.reloadHeadDependentState(m.Summary.HeadRefOID))
		}
		// Stale cache hit → schedule background revalidation.
		if msg.FromCache {
			out = append(out, m.loadPRDetailCmd(true))
		}
		return m, tea.Batch(out...)

	case cmds.DiffLoaded:
		if !m.matchesPR(msg.Host, msg.Repo, msg.Number) {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.DiffLoading = false
		if msg.Err != nil {
			if m.Diff == nil {
				if m.LoadErr != nil {
					// The detail load already failed; don't keep spinning.
					m.leftPanel.Loading = false
				} else {
					m.DiffLoading = true
				}
			}
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// Validate SHA if HeadRefOID is available.
		if m.Summary.HeadRefOID != "" && msg.Diff.HeadSHA != "" && msg.Diff.HeadSHA != m.Summary.HeadRefOID {
			// SHA mismatch — discard and refetch.
			m.DiffLoading = true
			return m, tea.Batch(spinCmd, composeCmd,
				cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.HeadRefOID, true))
		}
		m.Diff = &msg.Diff
		m.buildNavigableIndex()
		m.invalidateDiffCursor()
		m.rebuildDiffIndices()
		m.normalizeDiffRows()
		m.searchIndex = nil
		m.refreshSearchMatches()
		// Sync files into left panel.
		m.leftPanel.Files = m.Diff.Files
		m.leftPanel.Loading = false
		// Load persisted drafts for this PR/SHA.
		m.loadDrafts()
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.CommitsLoaded:
		if !m.matchesPR(msg.Host, msg.Repo, msg.Number) {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if msg.HeadSHA != "" && m.Summary.HeadRefOID != "" && msg.HeadSHA != m.Summary.HeadRefOID {
			if m.PRService == nil {
				m.commitsLoading = false
				return m, tea.Batch(spinCmd, composeCmd)
			}
			m.commitsLoading = true
			return m, tea.Batch(spinCmd, composeCmd,
				cmds.LoadPRCommitsCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.HeadRefOID, true))
		}
		m.commitsLoading = false
		if msg.Err != nil {
			m.commits = nil
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.commitsLoaded = true
		m.commits = msg.Commits
		m.commitCursor = 0
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.CommitDiffLoaded:
		m.DiffLoading = false
		if msg.Err != nil {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.Diff = &msg.Diff
		m.buildNavigableIndex()
		m.invalidateDiffCursor()
		m.rebuildDiffIndices()
		m.normalizeDiffRows()
		m.searchIndex = nil
		m.refreshSearchMatches()
		m.leftPanel.Files = m.Diff.Files
		m.leftPanel.Loading = false
		return m, tea.Batch(spinCmd, composeCmd)

	case submitComposeMsg:
		body := msg.body
		if m.compose.mode == composeModeDraftInline {
			if m.draftsStale || m.inlineDraftStale {
				m.compose.status = composeStatusError
				m.compose.errMsg = "The diff changed; reopen the inline draft on the current head"
				return m, tea.Batch(spinCmd, composeCmd)
			}
			if body == "" {
				return m, tea.Batch(spinCmd, composeCmd)
			}
			draft := m.buildDraftFromVisualSelection(body)
			m.upsertDraft(draft)
			m.persistDrafts()
			m.compose.Close()
			m.inlineDraftStale = false
			m.exitVisualMode()
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if m.compose.mode == composeModeEditTitle {
			title := strings.TrimSpace(msg.body)
			if title == "" {
				m.compose.status = composeStatusError
				m.compose.errMsg = "Title cannot be empty"
				return m, tea.Batch(spinCmd, composeCmd)
			}
			m.editPosting = true
			return m, tea.Batch(spinCmd, composeCmd, cmds.UpdatePRCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.ID, title, m.Detail.BodyExcerpt))
		}
		if m.compose.mode == composeModeEditBody {
			m.editPosting = true
			return m, tea.Batch(spinCmd, composeCmd, cmds.UpdatePRCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.ID, m.Summary.Title, msg.body))
		}
		if m.PRService == nil {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if draftErr := m.draftSubmissionError(); draftErr != "" && len(m.drafts) > 0 && (m.compose.mode == composeModeReviewComment || m.compose.mode == composeModeApprove) {
			m.compose.status = composeStatusError
			m.compose.errMsg = draftErr
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// When drafts exist, batch-submit them with the review event.
		if len(m.drafts) > 0 && (m.compose.mode == composeModeReviewComment || m.compose.mode == composeModeApprove) {
			event := "COMMENT"
			if m.compose.mode == composeModeApprove {
				event = "APPROVE"
			}
			postCmd := cmds.SubmitReviewWithDraftsCmd(m.PRService, m.Repo, m.Summary.ID, body, event, m.drafts)
			return m, tea.Batch(spinCmd, composeCmd, postCmd)
		}
		// No drafts: review comment with empty body is a no-op.
		if m.compose.mode == composeModeReviewComment && body == "" {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		var postCmd tea.Cmd
		if m.compose.mode == composeModeReviewComment {
			postCmd = cmds.PostReviewCommentCmd(m.PRService, m.Repo, m.Summary.ID, body)
		} else if m.compose.mode == composeModeReply {
			target := m.compose.target
			if target.threadID != "" {
				postCmd = cmds.PostThreadReplyCmd(m.PRService, m.Repo, target.threadID, body)
			} else if target.commentID != "" {
				body = buildReplyBody(target, body)
				postCmd = cmds.PostCommentReplyCmd(m.PRService, m.Repo, m.Summary.ID, target.commentID, body)
			} else if target.login != "" {
				body = buildReplyBody(target, body)
				postCmd = cmds.PostCommentCmd(m.PRService, m.Repo, m.Summary.ID, body)
			} else {
				postCmd = cmds.PostCommentCmd(m.PRService, m.Repo, m.Summary.ID, body)
			}
		} else {
			postCmd = cmds.PostCommentCmd(m.PRService, m.Repo, m.Summary.ID, body)
		}
		return m, tea.Batch(spinCmd, composeCmd, postCmd)

	case submitApproveMsg:
		if m.PRService == nil {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if draftErr := m.draftSubmissionError(); draftErr != "" && len(m.drafts) > 0 {
			m.compose.status = composeStatusError
			m.compose.errMsg = draftErr
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// When drafts exist, batch-submit them as an approved review.
		if len(m.drafts) > 0 {
			postCmd := cmds.SubmitReviewWithDraftsCmd(m.PRService, m.Repo, m.Summary.ID, msg.body, "APPROVE", m.drafts)
			return m, tea.Batch(spinCmd, composeCmd, postCmd)
		}
		return m, tea.Batch(spinCmd, composeCmd, cmds.ApprovePRCmd(m.PRService, m.Repo, m.Summary.ID, msg.body))

	case openEditorComposeMsg:
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}
		tmpFile, err := os.CreateTemp("", "pho-comment-*.md")
		if err != nil {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		tmpPath := tmpFile.Name()
		if _, werr := tmpFile.WriteString(msg.draft); werr != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return m, tea.Batch(spinCmd, composeCmd)
		}
		tmpFile.Close()
		return m, tea.Batch(spinCmd, composeCmd, tea.ExecProcess(
			exec.Command(editor, tmpPath),
			func(err error) tea.Msg { return editorDoneMsg{path: tmpPath, err: err} },
		))

	case editorDoneMsg:
		if msg.err == nil {
			if content, err := os.ReadFile(msg.path); err == nil {
				if m.compose.mode == composeModeEditBody {
					m.compose.SetText(string(content))
				} else {
					m.compose.SetText(strings.TrimSpace(string(content)))
				}
			}
		}
		os.Remove(msg.path)
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.CommentPosted:
		m.compose.status = composeStatusSuccess
		// Auto-expand resolved thread when replying to it (Q20).
		if m.compose.target.threadID != "" {
			thread := m.findThreadByID(m.compose.target.threadID)
			if thread != nil && thread.IsResolved {
				if m.expandedResolved == nil {
					m.expandedResolved = make(map[string]bool)
				}
				m.expandedResolved[thread.ID] = true
				m.commentEntriesDirty = true
			}
		}
		return m, tea.Batch(spinCmd, composeCmd, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return composeSuccessDismissMsg{}
		}))

	case cmds.CommentFailed:
		m.compose.status = composeStatusError
		m.compose.errMsg = msg.Err.Error()
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.ApprovalPosted:
		m.compose.status = composeStatusSuccess
		return m, tea.Batch(spinCmd, composeCmd, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return composeSuccessDismissMsg{}
		}))

	case cmds.ApprovalFailed:
		m.compose.status = composeStatusError
		m.compose.errMsg = msg.Err.Error()
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.ReviewPosted:
		m.compose.status = composeStatusSuccess
		draftHeadSHA := m.draftCacheHeadSHA()
		m.drafts = nil
		m.draftHeadSHA = ""
		m.draftsStale = false
		m.rebuildDraftCovered()
		m.commentEntriesDirty = true
		if m.PRService != nil {
			if draftHeadSHA != "" {
				_ = m.PRService.DeleteDraftComments(context.Background(), m.Repo, m.Summary.Number, draftHeadSHA)
			}
		}
		return m, tea.Batch(spinCmd, composeCmd, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return composeSuccessDismissMsg{}
		}))

	case cmds.ReviewFailed:
		m.compose.status = composeStatusError
		m.compose.errMsg = msg.Err.Error()
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.MergeableChecked:
		if m.mergeStep != mergeStepChecking {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if msg.Err != nil {
			m.mergeStep = mergeStepNone
			m.mergeErr = "Check failed: " + msg.Err.Error()
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if msg.State.Mergeable != "MERGEABLE" {
			m.mergeStep = mergeStepNone
			m.mergeErr = "PR is no longer mergeable (" + humanizeMergeState(msg.State.MergeStateStatus) + ")"
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.mergeStep = mergeStepExecuting
		return m, tea.Batch(spinCmd, composeCmd, cmds.MergePRCmd(m.PRService, m.mergeRepo, m.Summary.Number, m.mergePRID, msg.State.HeadRefOid, m.mergeMethod))

	case cmds.MergePRMsg:
		if m.mergeStep != mergeStepExecuting {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.mergeStep = mergeStepNone
		if msg.Err != nil {
			m.mergeErr = "Merge failed: " + msg.Err.Error()
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.mergeErr = ""
		if m.Detail != nil {
			m.Detail.State = "MERGED"
		}
		// Refresh detail to show merged state.
		var refreshCmd tea.Cmd
		if m.PRService != nil {
			refreshCmd = m.loadPRDetailCmd(true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd)

	case cmds.UpdateBranchMsg:
		if !m.matchesPR(msg.Host, msg.Repo, msg.Number) {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if m.updateStep != updateStepExecuting {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		if msg.Err != nil {
			m.updateStep = updateStepNone
			m.updateRepo = domain.Repository{}
			m.reloadDependentsIfHeadUnknown = false
			m.updateErr = "Update branch failed: " + msg.Err.Error()
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.updateErr = ""
		m.updateStep = updateStepNone
		m.updateRepo = domain.Repository{}
		m.reloadDependentsIfHeadUnknown = true
		return m, tea.Batch(spinCmd, composeCmd, m.loadPRDetailCmd(true))

	case cmds.PRStateChangedMsg:
		if m.closeStep != closeStepExecuting {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.closeStep = closeStepNone
		if msg.Err != nil {
			m.closeErr = "Failed: " + msg.Err.Error()
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.closeErr = ""
		if m.Detail != nil {
			m.Detail.State = msg.NewState
		}
		var refreshCmd tea.Cmd
		if m.PRService != nil {
			refreshCmd = m.loadPRDetailCmd(true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd)

	case cmds.PRUpdated:
		m.editPosting = false
		if msg.Err != nil {
			m.editErr = "Failed to update PR: " + msg.Err.Error()
			if m.compose.active && (m.compose.mode == composeModeEditTitle || m.compose.mode == composeModeEditBody) {
				m.compose.status = composeStatusError
				m.compose.errMsg = msg.Err.Error()
			}
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.editErr = ""
		m.Summary.Title = msg.Title
		if m.Detail != nil {
			m.Detail.Title = msg.Title
			m.Detail.BodyExcerpt = msg.Body
		}
		if m.compose.active && (m.compose.mode == composeModeEditTitle || m.compose.mode == composeModeEditBody) {
			m.compose.status = composeStatusSuccess
		}
		var refreshCmd tea.Cmd
		if m.PRService != nil {
			refreshCmd = m.loadPRDetailCmd(true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return composeSuccessDismissMsg{}
		}))

	case cmds.ThreadResolvedMsg:
		if !m.pendingToggle.active() || m.pendingToggle.ThreadID != msg.ThreadID {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// Don't clear pendingToggle here — PRDetailLoaded uses it for
		// replication-lag mitigation and cursor re-anchor, then clears it.
		var refreshCmd tea.Cmd
		if m.PRService != nil {
			refreshCmd = m.loadPRDetailCmd(true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd)

	case cmds.ThreadUnresolvedMsg:
		if !m.pendingToggle.active() || m.pendingToggle.ThreadID != msg.ThreadID {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// Don't clear pendingToggle here — PRDetailLoaded uses it for
		// replication-lag mitigation and cursor re-anchor, then clears it.
		var refreshCmd tea.Cmd
		if m.PRService != nil {
			refreshCmd = m.loadPRDetailCmd(true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd)

	case cmds.ThreadResolveFailed:
		if !m.pendingToggle.active() || m.pendingToggle.ThreadID != msg.ThreadID {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// Revert optimistic state.
		thread := m.findThreadByID(msg.ThreadID)
		if thread != nil {
			thread.IsResolved = m.pendingToggle.PrevResolved
			thread.ResolvedBy = m.pendingToggle.PrevResolver
		}
		m.resolveErr = "Failed: " + msg.Err.Error()
		m.pendingToggle = pendingToggleState{}
		m.commentEntriesDirty = true
		// Reload to reconcile against fresh GitHub state.
		var refreshCmd tea.Cmd
		if m.PRService != nil {
			refreshCmd = m.loadPRDetailCmd(true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd)

	case composeSuccessDismissMsg:
		wasEdit := m.compose.mode == composeModeEditTitle || m.compose.mode == composeModeEditBody
		target := m.compose.target
		m.compose.Close()
		if wasEdit {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.postedComment = true
		m.postedCommentTarget = target
		if m.PRService != nil {
			return m, tea.Batch(spinCmd, composeCmd, m.loadPRDetailCmd(true))
		}
		return m, tea.Batch(spinCmd, composeCmd)

	case composeClosedMsg:
		// Compose closed itself (e.g. Esc). No action needed here; the same-cycle
		// guard in tea.KeyMsg below prevents the consumed key from reaching handleKey.
		m.inlineDraftStale = false
		return m, tea.Batch(spinCmd, composeCmd)

	case cmds.CheckoutResult:
		return m, tea.Batch(spinCmd, composeCmd, func() tea.Msg {
			return checkoutResultMsg{branch: msg.Branch, stashed: msg.Stashed, err: msg.Err}
		})

	case checkoutResultMsg:
		m.checkoutInFlight = false
		if msg.err != nil {
			m.checkoutErr = msg.err.Error()
			return m, tea.Batch(spinCmd, composeCmd, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return checkoutClearMsg{}
			}))
		}
		m.checkoutStatus = "Checked out " + msg.branch
		if msg.stashed {
			m.checkoutStatus += " (changes stashed — git stash pop to restore)"
		}
		return m, tea.Batch(spinCmd, composeCmd, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return checkoutClearMsg{}
		}))

	case checkoutClearMsg:
		m.checkoutStatus = ""
		m.checkoutErr = ""
		return m, tea.Batch(spinCmd, composeCmd)

	case tea.KeyMsg:
		// Clear resolve error on any key (mirrors mergeErr/closeErr behavior).
		if m.resolveErr != "" && !m.pendingToggle.active() {
			m.resolveErr = ""
		}
		if m.compose.active && m.compose.status == composeStatusIdle {
			// Key already routed to compose.Update above; skip handleKey.
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// If compose was idle at the start of this cycle, the key was consumed
		// by compose (typing, Enter that triggers posting, Esc that closes, etc.)
		// and must not fall through to handleKey.
		if composeConsumedKey && composeWasIdle {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// If compose closed itself (e.g. Esc dismissed error), swallow the key.
		if composeConsumedKey && !m.compose.active {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		next, cmd := m.handleKey(msg)
		return next, tea.Batch(spinCmd, composeCmd, cmd)

	default:
		return m, tea.Batch(spinCmd, composeCmd)
	}
}

// BackToDashboard is emitted when the user presses q (or Esc while search is inactive) in PR detail.
type BackToDashboard struct{}

// BackToPRDetail is emitted when the user presses q/Esc in commit mode.
type BackToPRDetail struct{}

// ToggleKeymapOverlayMsg is emitted when the user presses ? (while not composing) in PR detail.
type ToggleKeymapOverlayMsg struct{}

// OpenBrowserCommit is emitted when the user presses 'o' in commit mode.
type OpenBrowserCommit struct {
	Repo domain.Repository
	SHA  string
}

// CopyCommitPermalink is emitted when the user presses 'y' with a diff cursor in commit mode.
type CopyCommitPermalink struct {
	URL string
}

func (m *PRDetailModel) log() *pholog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return pholog.NewNop()
}

// CopyCommitSHA is emitted when the user presses 'y' on a commit in the Commits tab.
type CopyCommitSHA struct {
	SHA string
}

// OpenBrowserPR is emitted when the user presses 'o' in PR detail.
type OpenBrowserPR struct {
	Repo   string
	Number int
}

// OpenBrowserCI is emitted when the user presses Enter on a CI check row.
type OpenBrowserCI struct {
	URL string
}

// OpenCommitDetail is emitted when the user presses Enter on a commit in the Commits tab.
type OpenCommitDetail struct {
	Repo   domain.Repository
	Commit domain.Commit
}
