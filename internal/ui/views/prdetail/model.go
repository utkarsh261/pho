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

// effectiveBodyH returns the body height available for the left/right panels,
// accounting for the compose pane when it is open (3 rows: top border + 2 content rows).
func (m *PRDetailModel) effectiveBodyH() int {
	bodyH := max(m.Height-3, 1)
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

	commentCursor int // -1 = none, 0..n-1 = index of focused comment entry
	postedComment bool

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
				cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, false),
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
		m.DetailLoading = false
		if msg.Err != nil {
			if m.Detail == nil {
				m.DetailLoading = true
			}
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.Detail = &msg.Detail
		m.DetailFromCache = msg.FromCache
		m.commentEntriesDirty = true
		m.resetCommentCursor()
		// Sync checks into left panel.
		m.leftPanel.Checks = msg.Detail.Checks

		// Auto-scroll to the newly posted comment after a successful post.
		if m.postedComment {
			m.postedComment = false
			cw := contentViewportWidth(m.rightPanelWidth())
			entries := m.commentEntries()
			startRows := m.commentEntryStartRows(cw)
			if len(startRows) > 0 {
				lastIdx := len(startRows) - 1
				entryTop := startRows[lastIdx]
				entryH := m.entryRowCount(entries[lastIdx], cw) + 2
				endRow := entryTop + entryH
				vh := m.contentViewportHeight()
				target := max(endRow-vh+1, 0)
				m.switchTab(TabComments)
				m.ContentScroll = target
				m.clampContentScroll()
				// Place cursor at the new comment so j/k starts from here.
				m.commentCursor = lastIdx
			}
		}

		var out []tea.Cmd
		out = append(out, spinCmd, composeCmd)
		// Stale cache hit → schedule background revalidation.
		if msg.FromCache {
			out = append(out, cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true))
		}
		return m, tea.Batch(out...)

	case cmds.DiffLoaded:
		m.DiffLoading = false
		if msg.Err != nil {
			if m.Diff == nil {
				m.DiffLoading = true
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
			if body == "" {
				return m, tea.Batch(spinCmd, composeCmd)
			}
			draft := m.buildDraftFromVisualSelection(body)
			m.upsertDraft(draft)
			m.persistDrafts()
			m.compose.Close()
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
		m.drafts = nil
		m.rebuildDraftCovered()
		m.commentEntriesDirty = true
		if m.PRService != nil {
			if headSHA := m.headSHA(); headSHA != "" {
				_ = m.PRService.DeleteDraftComments(context.Background(), m.Repo, m.Summary.Number, headSHA)
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
			refreshCmd = cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd)

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
			refreshCmd = cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true)
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
			refreshCmd = cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true)
		}
		return m, tea.Batch(spinCmd, composeCmd, refreshCmd, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return composeSuccessDismissMsg{}
		}))

	case composeSuccessDismissMsg:
		wasEdit := m.compose.mode == composeModeEditTitle || m.compose.mode == composeModeEditBody
		m.compose.Close()
		if wasEdit {
			// Edit modes don't trigger comment auto-scroll.
			return m, tea.Batch(spinCmd, composeCmd)
		}
		m.postedComment = true
		if m.PRService != nil {
			return m, tea.Batch(spinCmd, composeCmd, cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true))
		}
		return m, tea.Batch(spinCmd, composeCmd)

	case composeClosedMsg:
		// Compose closed itself (e.g. Esc). No action needed here; the same-cycle
		// guard in tea.KeyMsg below prevents the consumed key from reaching handleKey.
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
