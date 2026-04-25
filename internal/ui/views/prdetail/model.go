// Package prdetail implements the PR detail view model.
// It manages the PR detail state and handles keyboard routing within
// the PR detail view.
package prdetail

import (
	"context"
	"fmt"
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
	"github.com/utkarsh261/pho/internal/ui/markdown"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// composeSuccessDismissMsg is fired 1.5s after a comment is posted to close the compose pane.
type composeSuccessDismissMsg struct{}

// editorDoneMsg is fired when the external $EDITOR process exits.
type editorDoneMsg struct {
	path string
	err  error
}

// rightPanelWidth returns the outer width of the right panel given the current terminal width.
func (m *PRDetailModel) rightPanelWidth() int {
	if m.Width >= MinWidthForSidebar {
		return max(m.Width-LeftPanelWidth-2, 10)
	}
	return m.Width
}

// contentViewportWidth returns the usable text-column width inside the content area
// given the outer right-panel width.
func contentViewportWidth(rightWidth int) int {
	innerW := max(rightWidth-2, 1)
	return max(innerW-2, 1)
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

// visualModeState tracks the active visual-mode selection in the diff.
type visualModeState struct {
	Active    bool
	FileIdx   int
	HunkIdx   int
	StartLine int // index into hunk.Lines
	EndLine   int // index into hunk.Lines (inclusive)
}

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

	// Inline review drafts
	visual            visualModeState
	drafts            []domain.DraftInlineComment
	confirmDiscardAll bool
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
		compose:       newComposeModel(nil),
	}
	m.leftPanel.Loading = loading
	m.leftPanel.Focus = FocusContent
	m.leftPanel.LastOpenedIndex = 0
	m.leftPanel.CICursor = 0
	m.mdRenderer = markdown.New()
	return m
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
	cmdsOut = append(cmdsOut, m.spinner.Tick)
	if m.PRService != nil {
		headSHA := m.Summary.HeadRefOID
		cmdsOut = append(cmdsOut,
			cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, false),
			cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, false),
		)
	}
	return tea.Batch(cmdsOut...)
}

// Update handles messages and key events within the PR detail view.
func (m *PRDetailModel) Update(msg tea.Msg) (*PRDetailModel, tea.Cmd) {
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)

	// Forward all messages to compose so textinput receives tick events for cursor blink.
	var composeCmd tea.Cmd
	composeWasActive := m.compose.active
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
		m.resetCommentCursor()
		// Sync checks into left panel.
		m.leftPanel.Checks = msg.Detail.Checks

		// Auto-scroll to the newly posted comment after a successful post.
		if m.postedComment {
			m.postedComment = false
			cw := contentViewportWidth(m.rightPanelWidth())
			sections := m.buildContentSections(cw)
			if commentSec, ok := findSection(sections, domain.SectionComments); ok {
				entries := m.commentEntries()
				startRows := m.commentEntryStartRows(cw)
				if len(startRows) > 0 {
					lastIdx := len(startRows) - 1
					lastAbsRow := commentSec.StartRow + startRows[lastIdx]
					entryH := m.entryRowCount(entries[lastIdx], cw) + 2 // +2 for border
					endAbsRow := lastAbsRow + entryH
					vh := m.contentViewportHeight()
					target := max(endAbsRow-vh+1, 0)
					m.ContentScroll = target
					m.clampContentScroll()
					// Place cursor at the new comment so j/k starts from here.
					m.commentCursor = lastIdx
				}
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
		m.normalizeDiffRows()
		m.searchIndex = nil
		m.refreshSearchMatches()
		// Sync files into left panel.
		m.leftPanel.Files = m.Diff.Files
		m.leftPanel.Loading = false
		// Load persisted drafts for this PR/SHA.
		m.loadDrafts()
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
		if m.compose.mode == composeModeReply && m.commentCursor >= 0 {
			entries := m.commentEntries()
			if m.commentCursor < len(entries) {
				body = buildReplyBody(entries[m.commentCursor], msg.body)
			}
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
			postCmd := cmds.SubmitReviewWithDraftsCmd(m.PRService, m.Summary.ID, body, event, m.drafts)
			return m, tea.Batch(spinCmd, composeCmd, postCmd)
		}
		// No drafts: review comment with empty body is a no-op.
		if m.compose.mode == composeModeReviewComment && body == "" {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		var postCmd tea.Cmd
		if m.compose.mode == composeModeReviewComment {
			postCmd = cmds.PostReviewCommentCmd(m.PRService, m.Summary.ID, body)
		} else {
			postCmd = cmds.PostCommentCmd(m.PRService, m.Summary.ID, body)
		}
		return m, tea.Batch(spinCmd, composeCmd, postCmd)

	case submitApproveMsg:
		if m.PRService == nil {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// When drafts exist, batch-submit them as an approved review.
		if len(m.drafts) > 0 {
			postCmd := cmds.SubmitReviewWithDraftsCmd(m.PRService, m.Summary.ID, msg.body, "APPROVE", m.drafts)
			return m, tea.Batch(spinCmd, composeCmd, postCmd)
		}
		return m, tea.Batch(spinCmd, composeCmd, cmds.ApprovePRCmd(m.PRService, m.Summary.ID, msg.body))

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
				m.compose.SetText(strings.TrimSpace(string(content)))
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
		if m.PRService != nil {
			headSHA := ""
			if m.Diff != nil {
				headSHA = m.Diff.HeadSHA
			}
			if headSHA == "" {
				headSHA = m.Summary.HeadRefOID
			}
			_ = m.PRService.DeleteDraftComments(context.Background(), m.Repo, m.Summary.Number, headSHA)
		}
		return m, tea.Batch(spinCmd, composeCmd, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return composeSuccessDismissMsg{}
		}))

	case cmds.ReviewFailed:
		m.compose.status = composeStatusError
		m.compose.errMsg = msg.Err.Error()
		return m, tea.Batch(spinCmd, composeCmd)

	case composeSuccessDismissMsg:
		m.postedComment = true
		m.compose.Close()
		if m.PRService != nil {
			return m, tea.Batch(spinCmd, composeCmd, cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true))
		}
		return m, tea.Batch(spinCmd, composeCmd)

	case tea.KeyMsg:
		if m.compose.active {
			// Key already routed to compose.Update above; skip handleKey.
			return m, tea.Batch(spinCmd, composeCmd)
		}
		// If compose was just closed from draft-inline mode (via Esc), keep visual
		// mode active instead of letting handleKey process the Esc key.
		if composeWasActive && !m.compose.active && m.compose.mode == composeModeDraftInline && msg.String() == "esc" {
			return m, tea.Batch(spinCmd, composeCmd)
		}
		next, cmd := m.handleKey(msg)
		return next, tea.Batch(spinCmd, composeCmd, cmd)

	default:
		return m, tea.Batch(spinCmd, composeCmd)
	}
}

func (m *PRDetailModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}

	headerRow := m.renderHeader()

	bodyH := m.effectiveBodyH()

	var body string
	if m.compose.active && m.cachedBody != "" &&
		m.cachedBodyWidth == m.Width && m.cachedBodyHeight == bodyH {
		// Compose is open and nothing in the body has changed — reuse last render
		// so that text input navigation (arrow keys, backspace, etc.) is instant.
		body = m.cachedBody
	} else {
		if m.Width >= MinWidthForSidebar {
			rightWidth := max(m.Width-LeftPanelWidth-2, 10)
			leftView := m.leftPanel.View(bodyH, m.spinner.View())
			rightView := m.renderRightViewport(rightWidth, bodyH)
			body = lipgloss.JoinHorizontal(lipgloss.Top, leftView, "  ", rightView)
		} else {
			body = m.renderNarrowBody(m.Width, bodyH)
		}
		m.cachedBody = body
		m.cachedBodyWidth = m.Width
		m.cachedBodyHeight = bodyH
	}

	if m.compose.active {
		return headerRow + "\n" + body + "\n" + m.compose.View(m.Width)
	}
	return headerRow + "\n" + body
}

func (m *PRDetailModel) renderHeader() string {
	author := m.Summary.Author
	if author == "" {
		author = "unknown"
	}

	state := "OPEN"
	if m.Detail != nil {
		state = string(m.Detail.State)
	}

	var authorStr string
	var stateStr string
	if m.theme != nil {
		authorStr = m.theme.PrimaryTxt.Render(author)
		switch state {
		case "OPEN":
			stateStr = lipgloss.NewStyle().Foreground(m.theme.Secondary).Render("OPEN")
		case "MERGED":
			stateStr = m.theme.PrimaryTxt.Render("MERGED")
		case "CLOSED":
			stateStr = m.theme.ReviewChanges.Render("CLOSED")
		default:
			stateStr = m.theme.ReviewRequired.Render(state)
		}
	} else {
		authorStr = author
		stateStr = state
	}

	metaStr := authorStr + " " + stateStr
	metaLen := lipgloss.Width(metaStr)

	hints := "[o: Browser | Esc: Back]"
	if m.Width < 80 {
		hints = ""
	}
	hintsLen := lipgloss.Width(hints)

	innerW := max(m.Width-2, 1)

	// Build the title ensuring we don't overflow the width
	// Padding needed: spaces around components
	// Format: "Title <author> <state>                  [o: Browser | Esc: Back]"

	reservedSpace := metaLen
	if hintsLen > 0 {
		// we want spacing between meta and hints, or we right-align hints
		reservedSpace += 1 + hintsLen
	}

	// Prepend PR number
	baseTitle := fmt.Sprintf("#%d %s", m.Summary.Number, m.Summary.Title)
	if m.Summary.Title == "" {
		baseTitle = fmt.Sprintf("Pull Request #%d", m.Summary.Number)
	}

	// 1 space between title and meta
	titleBudget := innerW - reservedSpace - 2 // -2 just for padding
	if titleBudget < 5 {
		titleBudget = 5
	}

	truncTitle := baseTitle
	if lipgloss.Width(baseTitle) > titleBudget {
		truncTitle = truncateText(baseTitle, titleBudget)
	}

	leftPart := truncTitle + " " + metaStr

	var finalHeader string
	if hintsLen > 0 {
		leftWidth := lipgloss.Width(leftPart)
		padWidth := max(innerW-leftWidth-hintsLen, 1)
		finalHeader = leftPart + strings.Repeat(" ", padWidth) + hints
	} else {
		finalHeader = leftPart + strings.Repeat(" ", max(0, innerW-lipgloss.Width(leftPart)))
	}

	var content string
	var borderColor lipgloss.Color
	if m.theme != nil {
		// Apply the blueish background to the entire string
		content = m.theme.Header.Width(innerW).Render(finalHeader)
		borderColor = m.theme.Border
	} else {
		content = lipgloss.NewStyle().Width(innerW).Render(finalHeader)
		borderColor = theme.Default().Border
	}

	// Restore the island (the bordered box)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Render(content)
}

func (m *PRDetailModel) renderRightViewport(width, height int) string {
	innerH := max(height-4, 1)
	innerW := max(width-2, 1)
	contentW := max(innerW-2, 1)
	contentH := max(innerH-2, 1)

	// Build sections and clamp scroll within the real content bounds.
	sections := m.buildContentSections(contentW)
	total := totalRowsInSections(sections)
	scroll := clamp(m.ContentScroll, 0, max(0, total-contentH))

	// Scroll-spy: use the unclamped ContentScroll so that a section jump
	// (e.g. pressing '2') highlights the Diff tab even when total content
	// fits within the viewport and the display scroll is clamped to 0.
	active := activeSectionAt(sections, m.ContentScroll)

	// Render content lines using the overscan algorithm.
	lines := m.renderContentLines(sections, scroll, contentH, contentW)

	// Apply left-padding (1 space) to each content line.
	for i, l := range lines {
		lines[i] = " " + l
	}
	contentStr := renderBlock(lines, innerW, contentH)

	// Build tab indicators (scroll-spy only — not focusable).
	tabsStr := m.renderSectionTabs(sections, active)
	tabsStr = " " + tabsStr

	var borderColor lipgloss.Color
	if m.theme != nil {
		borderColor = m.theme.Border
	} else {
		borderColor = theme.Default().Border
	}
	if m.leftPanel.Focus == FocusContent {
		if m.theme != nil {
			borderColor = m.theme.Primary
		} else {
			borderColor = theme.Default().Primary
		}
	}

	headBox := lipgloss.NewStyle().
		Border(panelHeadBorder).
		BorderForeground(borderColor).
		Width(innerW).
		Render(tabsStr)

	bodyBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH).
		Render(contentStr)

	return lipgloss.JoinVertical(lipgloss.Left, headBox, bodyBox)
}

// renderSectionTabs builds the "Desc | Diff | Comments" indicator string.
// Active section is highlighted; sections with RowCount=0 are muted.
func (m *PRDetailModel) renderSectionTabs(sections []ContentSection, active domain.PRDetailSection) string {
	type tabDef struct {
		section domain.PRDetailSection
		label   string
	}
	tabs := []tabDef{
		{domain.SectionDescription, "Desc"},
		{domain.SectionDiff, "Diff"},
		{domain.SectionComments, "Comments"},
	}

	th := m.theme
	if th == nil {
		th = theme.Default()
	}

	parts := make([]string, len(tabs))
	for i, td := range tabs {
		sec, hasRows := findSection(sections, td.section)
		_ = sec

		var rendered string
		switch {
		case hasRows && active == td.section:
			rendered = th.TabActive.Render("● " + td.label)
		case hasRows:
			rendered = th.TabInactive.Render(td.label)
		default:
			rendered = th.MutedTxt.Render(td.label)
		}
		parts[i] = rendered
	}
	return strings.Join(parts, " ")
}

// renderNarrowBody renders the body for terminals < 80 cols (no sidebar).
// Shows "N files changed" as the first line then the content viewport.
func (m *PRDetailModel) renderNarrowBody(width, height int) string {
	fileCount := 0
	if m.Diff != nil {
		fileCount = len(m.Diff.Files)
	} else if m.Detail != nil {
		fileCount = m.Detail.FileCount
	}
	header := fmt.Sprintf("  %d files changed", fileCount)
	if height <= 1 {
		return lipgloss.NewStyle().Width(width).Render(header)
	}
	top := lipgloss.NewStyle().Width(width).Render(header)
	body := m.renderRightViewport(width, height-1)
	return top + "\n" + body
}

// isInCommentsSection reports whether the content viewport is currently showing
// the Comments section. Returns true when the viewport bottom overlaps the
// section, so natural scrolling to the bottom activates comment navigation even
// when the comments section is shorter than the viewport height.
func (m *PRDetailModel) isInCommentsSection() bool {
	if m.leftPanel.Focus != FocusContent {
		return false
	}
	cw := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(cw)
	commentSec, ok := findSection(sections, domain.SectionComments)
	if !ok {
		return false
	}
	vh := m.contentViewportHeight()
	viewEnd := m.ContentScroll + vh
	return m.ContentScroll >= commentSec.StartRow || viewEnd > commentSec.StartRow
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
// deactivates the cursor and scrolls up one line (exits comment nav mode).
func (m *PRDetailModel) moveCursorPrevComment() {
	if m.commentCursor <= 0 {
		m.commentCursor = -1
		// Fall through to normal line scroll.
		m.ContentScroll--
		m.clampContentScroll()
		return
	}
	m.commentCursor--
	m.scrollToCommentCursor()
}

// scrollToCommentCursor scrolls the minimum amount needed to make the focused
// comment entry fully visible. No-op when the entry already fits in the viewport.
func (m *PRDetailModel) scrollToCommentCursor() {
	if m.commentCursor < 0 {
		return
	}
	cw := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(cw)
	commentSec, ok := findSection(sections, domain.SectionComments)
	if !ok {
		return
	}
	startRows := m.commentEntryStartRows(cw)
	if m.commentCursor >= len(startRows) {
		return
	}
	entries := m.commentEntries()
	entryTop := commentSec.StartRow + startRows[m.commentCursor]
	entryBottom := entryTop + m.entryRowCount(entries[m.commentCursor], cw) + 2 // +2 for border
	vh := m.contentViewportHeight()
	viewTop := m.ContentScroll
	viewBottom := viewTop + vh

	switch {
	case entryTop < viewTop:
		// Entry top is above viewport: scroll up to show it.
		m.ContentScroll = entryTop
	case entryBottom > viewBottom:
		// Entry bottom is below viewport: scroll down the minimum to show it.
		m.ContentScroll = entryBottom - vh
	}
	m.clampContentScroll()
}

// expandVisualSelectionDown grows the selection by one line downward within the hunk.
func (m *PRDetailModel) expandVisualSelectionDown() {
	if !m.visual.Active || m.Diff == nil {
		return
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	if m.visual.EndLine+1 < len(h.Lines) {
		m.visual.EndLine++
		// Auto-scroll to keep selection visible.
		endRow := m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.EndLine)
		vh := m.contentViewportHeight()
		if endRow >= m.ContentScroll+vh {
			m.ContentScroll = endRow - vh + 1
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
	if m.visual.EndLine > m.visual.StartLine {
		m.visual.EndLine--
		// Auto-scroll to keep selection visible.
		startRow := m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine)
		if startRow < m.ContentScroll {
			m.ContentScroll = startRow
			m.clampContentScroll()
		}
	} else {
		m.exitVisualMode()
	}
}

// isInDiffSection reports whether the content viewport is currently showing
// the Diff section.
func (m *PRDetailModel) isInDiffSection() bool {
	cw := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(cw)
	diffSec, ok := findSection(sections, domain.SectionDiff)
	if !ok || diffSec.RowCount == 0 {
		return false
	}
	vh := m.contentViewportHeight()
	viewEnd := m.ContentScroll + vh
	return m.ContentScroll < diffSec.StartRow+diffSec.RowCount && viewEnd > diffSec.StartRow
}

// jumpToCommentCode scrolls the diff viewport to the code line referenced by
// the focused comment entry.
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
	if m.Diff == nil {
		return
	}
	for fi, f := range m.Diff.Files {
		for hi, h := range f.Hunks {
			for li, dl := range h.Lines {
				for _, a := range dl.Anchors {
					if a.Path == entry.path && a.Line != nil && *a.Line == entry.line {
						m.ContentScroll = m.diffLineToDisplayRow(fi, hi, li)
						m.leftPanel.Focus = FocusContent
						m.clampContentScroll()
						return
					}
				}
			}
		}
	}
}

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
			m.persistDrafts()
			m.confirmDiscardAll = false
		case "n", "esc":
			m.confirmDiscardAll = false
		}
		m.LastKey = ""
		return m, nil
	}

	switch msg.String() {
	case "/":
		m.activateSearch()
		return m, nil
	case "n", "N":
		// Search navigation is only meaningful while searchActive=true.
		return m, nil
	case "esc":
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
		return m, m.emitBackToDashboard()
	case "R":
		return m.handleRefresh()
	case "C":
		if m.PRService != nil {
			m.compose.Open(composeModeNew, commentEntry{}, len(m.drafts))
		}
		return m, nil
	case "a":
		if m.PRService != nil {
			m.compose.Open(composeModeApprove, commentEntry{}, len(m.drafts))
		}
		return m, nil
	case "v":
		if m.PRService != nil {
			m.compose.Open(composeModeReviewComment, commentEntry{}, len(m.drafts))
		}
		return m, nil
	case "r":
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
		if m.leftPanel.Focus == FocusContent && m.isInDiffSection() {
			m.enterVisualMode()
		}
		return m, nil
	case "D":
		if len(m.drafts) > 0 {
			m.confirmDiscardAll = true
		}
		return m, nil
	case "o":
		return m, m.emitOpenBrowser()
	case "tab":
		m.cycleForward()
		m.resetCommentCursor()
	case "shift+tab":
		m.cycleBackward()
		m.resetCommentCursor()
	case "j", "down":
		if m.isInCommentsSection() {
			m.moveCursorNextComment()
			return m, nil
		}
		m.scrollDown()
	case "k", "up":
		if m.isInCommentsSection() && m.commentCursor >= 0 {
			// moveCursorPrevComment handles deactivation+scrollUp at entry 0.
			m.moveCursorPrevComment()
			return m, nil
		}
		m.scrollUp()
		if !m.isInCommentsSection() {
			m.resetCommentCursor()
		}
	case "enter":
		if m.leftPanel.Focus == FocusFiles {
			m.jumpToFile(m.leftPanel.FileIndex)
		} else if m.leftPanel.Focus == FocusCI {
			return m, m.emitOpenBrowserCI()
		} else if m.leftPanel.Focus == FocusContent && m.isInCommentsSection() && m.commentCursor >= 0 {
			m.jumpToCommentCode()
		}
	case "h", "left":
		m.jumpFileViewer()
	case "l", "right":
		m.jumpDiffViewer()
	case "shift+h":
		m.jumpPrevFile()
	case "shift+l":
		m.jumpNextFile()
	case "1":
		m.jumpToSection(1)
		m.resetCommentCursor()
	case "2":
		m.jumpToSection(2)
		m.resetCommentCursor()
	case "3":
		m.resetCommentCursor()
		m.jumpToSection(3)
	case "g":
		if m.LastKey == "g" {
			m.scrollToTop()
			m.LastKey = ""
			return m, nil
		}
		m.LastKey = "g"
		return m, nil
	case "G":
		m.scrollToBottom()
	case "ctrl+d":
		m.scrollHalfPageDown()
	case "ctrl+u":
		m.scrollHalfPageUp()
	}
	if msg.String() != "g" {
		m.LastKey = ""
	}
	return m, nil
}

// jumpToSection scrolls the content viewport to the start of the given section (1=Desc, 2=Diff, 3=Comments).
// If the section is empty (RowCount = 0) or does not exist, this is a no-op.
// On success, focus moves to the Content viewport.
func (m *PRDetailModel) jumpToSection(num int) {
	target := domain.PRDetailSection(num - 1)
	contentWidth := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(contentWidth)
	sec, ok := findSection(sections, target)
	if !ok {
		return
	}
	m.ContentScroll = sec.StartRow
	m.leftPanel.Focus = FocusContent
}

// jumpToFile scrolls the content viewport so that file at index idx is at the top
// and moves focus to the Content viewport. No-op when diff is absent or idx is out of range.
func (m *PRDetailModel) jumpToFile(idx int) {
	if m.Diff == nil || idx < 0 || idx >= len(m.Diff.Files) {
		return
	}
	m.leftPanel.LastOpenedIndex = idx
	contentWidth := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(contentWidth)
	diffSec, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		return
	}
	fileOffset := 0
	for i := range idx {
		fileOffset += diffFileDisplayRows(&m.Diff.Files[i])
	}
	contentHeight := m.contentViewportHeight()
	// When fileOffset falls beyond the rendered diff (truncated large diffs), the
	// scroll would overflow into comments. Show the truncation banner instead.
	if fileOffset >= diffSec.RowCount {
		target := diffSec.StartRow + max(0, diffSec.RowCount-contentHeight)
		m.ContentScroll = clamp(target, 0, m.maxContentScroll())
		m.leftPanel.Focus = FocusContent
		return
	}
	m.ContentScroll = clamp(diffSec.StartRow+fileOffset, 0, m.maxContentScroll())
	m.leftPanel.Focus = FocusContent
}

// cycleForward advances focus: Files → CI (if checks) → Content → Files.
func (m *PRDetailModel) cycleForward() {
	if m.Width < MinWidthForSidebar {
		return // sidebar hidden, only Content exists
	}
	switch m.leftPanel.Focus {
	case FocusFiles:
		if len(m.leftPanel.Checks) > 0 {
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
		if len(m.leftPanel.Checks) > 0 {
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
		m.leftPanel.FileIndex++
		last := len(m.leftPanel.Files) - 1
		if m.leftPanel.FileIndex > last {
			// If CI has checks, move focus there.
			m.leftPanel.FileIndex = last
			if len(m.leftPanel.Checks) > 0 {
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
		if m.leftPanel.FileIndex <= 0 {
			return
		}
		m.leftPanel.FileIndex--
		m.ensureFileVisible()
	case FocusCI:
		if m.leftPanel.CICursor <= 0 {
			// move focus back to Files.
			m.leftPanel.Focus = FocusFiles
			m.leftPanel.FilesScroll = 0
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
		m.jumpToFile(m.leftPanel.FileIndex)
	}
}

// jumpPrevFile moves to previous file
func (m *PRDetailModel) jumpPrevFile() {
	if m.leftPanel.Focus != FocusFiles {
		return
	}
	m.leftPanel.FileIndex = clamp(m.leftPanel.FileIndex-1, 0, max(0, len(m.leftPanel.Files)-1))
	m.ensureFileVisible()
}

// jumpNextFile moves the file cursor to the next file
func (m *PRDetailModel) jumpNextFile() {
	if m.leftPanel.Focus != FocusFiles {
		return
	}
	m.leftPanel.FileIndex = clamp(m.leftPanel.FileIndex+1, 0, max(0, len(m.leftPanel.Files)-1))
	m.ensureFileVisible()
}

func (m *PRDetailModel) scrollToTop() {
	switch m.leftPanel.Focus {
	case FocusFiles:
		m.leftPanel.FileIndex = 0
		m.leftPanel.FilesScroll = 0
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
			m.leftPanel.FileIndex = len(m.leftPanel.Files) - 1
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
		m.leftPanel.FileIndex = clamp(m.leftPanel.FileIndex+half, 0, max(0, len(m.leftPanel.Files)-1))
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
		m.leftPanel.FileIndex = clamp(m.leftPanel.FileIndex-half, 0, max(0, len(m.leftPanel.Files)-1))
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

// maxContentScroll returns the maximum valid content scroll value.
func (m *PRDetailModel) maxContentScroll() int {
	contentWidth := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(contentWidth)
	total := totalRowsInSections(sections)
	return max(0, total-m.contentViewportHeight())
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

	if m.leftPanel.FileIndex < m.leftPanel.FilesScroll {
		m.leftPanel.FilesScroll = m.leftPanel.FileIndex
	} else if m.leftPanel.FileIndex >= m.leftPanel.FilesScroll+contentH {
		m.leftPanel.FilesScroll = m.leftPanel.FileIndex - contentH + 1
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
	m.Detail = nil
	m.Diff = nil
	m.DetailLoading = true
	m.DiffLoading = true
	m.leftPanel.Loading = true
	m.searchIndex = nil
	m.refreshSearchMatches()
	headSHA := m.Summary.HeadRefOID
	return m, tea.Batch(
		cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true),
		cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, true),
	)
}

// ── Visual mode & draft helpers ───────────────────────────────────────────────

// diffLineToDisplayRow returns the absolute display row for a diff line.
func (m *PRDetailModel) diffLineToDisplayRow(fileIdx, hunkIdx, lineIdx int) int {
	if m.Diff == nil {
		return 0
	}
	sections := m.buildContentSections(contentViewportWidth(m.rightPanelWidth()))
	diffSec, _ := findSection(sections, domain.SectionDiff)
	row := diffSec.StartRow
	for i := 0; i < fileIdx; i++ {
		row += diffFileDisplayRows(&m.Diff.Files[i])
	}
	row += 3 // blank + separator + header
	f := &m.Diff.Files[fileIdx]
	for i := 0; i < hunkIdx; i++ {
		row += 1 + len(f.Hunks[i].Lines)
	}
	row += 1 + lineIdx // hunk header + line offset
	return row
}

// firstDiffLineAtOrBelow finds the first actual DiffLine at or after targetRow.
func (m *PRDetailModel) firstDiffLineAtOrBelow(targetRow int) (fileIdx, hunkIdx, lineIdx int, found bool) {
	if m.Diff == nil || len(m.Diff.Files) == 0 {
		return 0, 0, 0, false
	}
	sections := m.buildContentSections(contentViewportWidth(m.rightPanelWidth()))
	diffSec, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		return 0, 0, 0, false
	}
	if targetRow < diffSec.StartRow || targetRow >= diffSec.StartRow+diffSec.RowCount {
		return 0, 0, 0, false
	}
	localTarget := targetRow - diffSec.StartRow
	for fi := range m.Diff.Files {
		f := &m.Diff.Files[fi]
		dr := diffFileDisplayRows(f)
		if localTarget < dr {
			if f.IsBinary {
				// Skip binary files — no diff lines to select.
				return 0, 0, 0, false
			}
			localTarget -= 3 // skip blank, separator, header
			if localTarget <= 0 {
				return fi, 0, 0, true
			}
			for hi, hunk := range f.Hunks {
				if localTarget == 0 {
					return fi, hi, 0, true
				}
				localTarget--
				if localTarget < len(hunk.Lines) {
					return fi, hi, localTarget, true
				}
				localTarget -= len(hunk.Lines)
			}
			lastHunk := len(f.Hunks) - 1
			if lastHunk >= 0 {
				lastLines := len(f.Hunks[lastHunk].Lines)
				if lastLines > 0 {
					return fi, lastHunk, lastLines-1, true
				}
			}
			return fi, 0, 0, true
		}
		localTarget -= dr
	}
	return 0, 0, 0, false
}

// enterVisualMode activates visual mode anchored at the first diff line at or
// below the current ContentScroll position.
func (m *PRDetailModel) enterVisualMode() {
	fi, hi, li, ok := m.firstDiffLineAtOrBelow(m.ContentScroll)
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

// exitVisualMode deactivates visual mode.
func (m *PRDetailModel) exitVisualMode() {
	m.visual.Active = false
}

// buildDraftFromVisualSelection creates a DraftInlineComment from the current
// visual selection and the provided body text.
func (m *PRDetailModel) buildDraftFromVisualSelection(body string) domain.DraftInlineComment {
	if !m.visual.Active || m.Diff == nil {
		return domain.DraftInlineComment{}
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	firstLine := h.Lines[m.visual.StartLine]
	lastLine := h.Lines[m.visual.EndLine]

	draft := domain.DraftInlineComment{
		ID:          generateDraftID(),
		Path:        lastLine.Anchors[0].Path,
		Line:        *lastLine.Anchors[0].Line,
		Side:        lastLine.Anchors[0].Side,
		Body:        body,
		ContextLine: lastLine.Raw,
		HeadSHA:     lastLine.Anchors[0].CommitSHA,
		CreatedAt:   time.Now(),
	}
	if m.visual.StartLine != m.visual.EndLine && firstLine.Anchors != nil && len(firstLine.Anchors) > 0 {
		draft.StartLine = *firstLine.Anchors[0].Line
		draft.StartSide = firstLine.Anchors[0].Side
	}
	return draft
}

// upsertDraft replaces an existing draft on the exact same range or appends a new one.
func (m *PRDetailModel) upsertDraft(draft domain.DraftInlineComment) {
	for i, d := range m.drafts {
		if d.Path == draft.Path && d.Line == draft.Line && d.Side == draft.Side &&
			d.StartLine == draft.StartLine && d.StartSide == draft.StartSide {
			m.drafts[i] = draft
			return
		}
	}
	m.drafts = append(m.drafts, draft)
}

// removeDraftAt removes any draft that overlaps the given file/hunk/line range.
func (m *PRDetailModel) removeDraftAt(fileIdx, hunkIdx, startLine, endLine int) bool {
	if m.Diff == nil {
		return false
	}
	f := &m.Diff.Files[fileIdx]
	h := &f.Hunks[hunkIdx]
	if startLine < 0 || startLine >= len(h.Lines) || endLine < 0 || endLine >= len(h.Lines) {
		return false
	}
	firstLine := h.Lines[startLine]
	lastLine := h.Lines[endLine]
	path := lastLine.Anchors[0].Path
	line := *lastLine.Anchors[0].Line
	side := lastLine.Anchors[0].Side
	startLineNum := 0
	startSide := ""
	if startLine != endLine && firstLine.Anchors != nil && len(firstLine.Anchors) > 0 {
		startLineNum = *firstLine.Anchors[0].Line
		startSide = firstLine.Anchors[0].Side
	}

	for i, d := range m.drafts {
		if d.Path == path && d.Side == side && d.Line == line {
			// Single-line draft match.
			if d.StartLine == 0 && startLineNum == 0 {
				m.drafts = append(m.drafts[:i], m.drafts[i+1:]...)
				return true
			}
			// Multi-line draft match.
			if d.StartLine == startLineNum && d.StartSide == startSide {
				m.drafts = append(m.drafts[:i], m.drafts[i+1:]...)
				return true
			}
		}
	}
	return false
}

// draftOverlapsSelection reports whether any draft overlaps the visual selection.
func (m *PRDetailModel) draftOverlapsSelection() bool {
	if !m.visual.Active || m.Diff == nil {
		return false
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	firstLine := h.Lines[m.visual.StartLine]
	lastLine := h.Lines[m.visual.EndLine]
	path := lastLine.Anchors[0].Path
	line := *lastLine.Anchors[0].Line
	side := lastLine.Anchors[0].Side
	startLineNum := 0
	startSide := ""
	if m.visual.StartLine != m.visual.EndLine && firstLine.Anchors != nil && len(firstLine.Anchors) > 0 {
		startLineNum = *firstLine.Anchors[0].Line
		startSide = firstLine.Anchors[0].Side
	}

	for _, d := range m.drafts {
		if d.Path == path && d.Side == side && d.Line == line {
			if d.StartLine == 0 && startLineNum == 0 {
				return true
			}
			if d.StartLine == startLineNum && d.StartSide == startSide {
				return true
			}
		}
	}
	return false
}

// findDraftForSelection returns the draft matching the exact current visual
// selection, or nil if none exists.
func (m *PRDetailModel) findDraftForSelection() *domain.DraftInlineComment {
	if !m.visual.Active || m.Diff == nil {
		return nil
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	firstLine := h.Lines[m.visual.StartLine]
	lastLine := h.Lines[m.visual.EndLine]
	path := lastLine.Anchors[0].Path
	line := *lastLine.Anchors[0].Line
	side := lastLine.Anchors[0].Side
	startLineNum := 0
	startSide := ""
	if m.visual.StartLine != m.visual.EndLine && firstLine.Anchors != nil && len(firstLine.Anchors) > 0 {
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

// persistDrafts saves the current drafts to the cache.
func (m *PRDetailModel) persistDrafts() {
	if m.PRService == nil {
		return
	}
	headSHA := ""
	if m.Diff != nil {
		headSHA = m.Diff.HeadSHA
	}
	if headSHA == "" {
		headSHA = m.Summary.HeadRefOID
	}
	_ = m.PRService.SaveDraftComments(context.Background(), m.Repo, m.Summary.Number, headSHA, m.drafts)
}

// loadDrafts loads drafts from the cache for the current PR.
func (m *PRDetailModel) loadDrafts() {
	if m.PRService == nil {
		return
	}
	headSHA := ""
	if m.Diff != nil {
		headSHA = m.Diff.HeadSHA
	}
	if headSHA == "" {
		headSHA = m.Summary.HeadRefOID
	}
	drafts, _ := m.PRService.LoadDraftComments(context.Background(), m.Repo, m.Summary.Number, headSHA)
	m.drafts = drafts
}

// lookupDiffLine finds the raw diff line text for a given path:line.
func (m *PRDetailModel) lookupDiffLine(path string, line int) string {
	if m.Diff == nil {
		return ""
	}
	for _, f := range m.Diff.Files {
		for _, h := range f.Hunks {
			for _, dl := range h.Lines {
				for _, a := range dl.Anchors {
					if a.Path == path && a.Line != nil && *a.Line == line {
						return dl.Raw
					}
				}
			}
		}
	}
	return ""
}

// generateDraftID creates a simple unique ID for a draft comment.
func generateDraftID() string {
	return fmt.Sprintf("draft-%d", time.Now().UnixNano())
}

// StatusHint returns the status bar hint text for the current state.
func (m *PRDetailModel) StatusHint() string {
	if m.visual.Active {
		return "j/k: Select lines | c: Comment | d: Discard | Esc: Exit visual"
	}
	if m.confirmDiscardAll {
		return fmt.Sprintf("Discard all %d drafts? (y/n)", len(m.drafts))
	}
	hint := "Tab: Switch Panel | Space: Visual | 1/2/3: Jump to section | R: Refresh | v: Review | C: Comment | a: Approve | /: Search in Diff"
	if len(m.drafts) > 0 {
		hint += " | D: Discard all drafts"
	}
	return hint
}

func (m *PRDetailModel) emitBackToDashboard() tea.Cmd {
	return func() tea.Msg { return BackToDashboard{} }
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

// BackToDashboard is emitted when the user presses q (or Esc while search is inactive) in PR detail.
type BackToDashboard struct{}

// OpenBrowserPR is emitted when the user presses 'o' in PR detail.
type OpenBrowserPR struct {
	Repo   string
	Number int
}

// OpenBrowserCI is emitted when the user presses Enter on a CI check row.
type OpenBrowserCI struct {
	URL string
}
