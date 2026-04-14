// Package prdetail implements the PR detail view model for Phase 2.
// It manages the PR detail state and handles keyboard routing within
// the PR detail view.
package prdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/utk/git-term/internal/application/cmds"
	"github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/ui/theme"
)

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

	leftPanel LeftPanelModel
	spinner   spinner.Model

	theme *theme.Theme
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
	}
	m.leftPanel.Loading = loading
	m.leftPanel.Focus = FocusFiles
	return m
}

// SetTheme applies a theme to the PR detail model.
func (m *PRDetailModel) SetTheme(th *theme.Theme) {
	m.theme = th
	m.leftPanel.SetTheme(th)
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, spinCmd

	case cmds.PRDetailLoaded:
		m.DetailLoading = false
		if msg.Err != nil {
			if m.Detail == nil {
				m.DetailLoading = true
			}
			return m, spinCmd
		}
		m.Detail = &msg.Detail
		m.DetailFromCache = msg.FromCache
		// Sync checks into left panel.
		m.leftPanel.Checks = msg.Detail.Checks

		var out []tea.Cmd
		out = append(out, spinCmd)
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
			return m, spinCmd
		}
		// Validate SHA if HeadRefOID is available.
		if m.Summary.HeadRefOID != "" && msg.Diff.HeadSHA != "" && msg.Diff.HeadSHA != m.Summary.HeadRefOID {
			// SHA mismatch — discard and refetch.
			m.DiffLoading = true
			return m, tea.Batch(spinCmd,
				cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.HeadRefOID, true))
		}
		m.Diff = &msg.Diff
		// Sync files into left panel.
		m.leftPanel.Files = msg.Diff.Files
		m.leftPanel.Loading = false
		return m, spinCmd

	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		return next, tea.Batch(spinCmd, cmd)

	default:
		return m, spinCmd
	}
}

func (m *PRDetailModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}

	headerRow := m.renderHeader()

	bodyH := m.Height - 3
	if bodyH < 1 {
		bodyH = 1
	}

	var body string
	if m.Width >= MinWidthForSidebar {
		rightWidth := m.Width - LeftPanelWidth - 2
		if rightWidth < 10 {
			rightWidth = 10
		}
		leftView := m.leftPanel.View(bodyH, m.spinner.View())
		rightView := m.renderRightViewport(rightWidth, bodyH)
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftView, "  ", rightView)
	} else {
		body = m.renderNarrowBody(m.Width, bodyH)
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

	innerW := m.Width - 2
	if innerW < 1 {
		innerW = 1
	}

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
		padWidth := innerW - leftWidth - hintsLen
		if padWidth < 1 { padWidth = 1 }
		finalHeader = leftPart + strings.Repeat(" ", padWidth) + hints
	} else {
		finalHeader = leftPart + strings.Repeat(" ", max(0, innerW - lipgloss.Width(leftPart)))
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
	innerH := height - 4
	if innerH < 1 {
		innerH = 1
	}
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	contentW := innerW - 2
	if contentW < 1 {
		contentW = 1
	}
	contentH := innerH - 2
	if contentH < 1 {
		contentH = 1
	}

	var contentLines []string

	contentLines = append(contentLines, "Description")

	if m.Detail != nil && strings.TrimSpace(m.Detail.BodyExcerpt) != "" {
		bodyLines := wrapParagraph(m.Detail.BodyExcerpt, contentW)
		for _, l := range bodyLines {
			contentLines = append(contentLines, l)
		}
	} else if !m.DetailLoading {
		contentLines = append(contentLines, "No description provided.")
	} else {
		contentLines = append(contentLines, "Loading…")
	}

	contentLines = append(contentLines, "")
	contentLines = append(contentLines, strings.Repeat("─", contentW))

	if m.DiffLoading {
		contentLines = append(contentLines, "⠋ Loading diff…")
	} else if m.Diff == nil || len(m.Diff.Files) == 0 {
		contentLines = append(contentLines, "No changes")
	} else {
		stats := m.Diff.Stats
		contentLines = append(contentLines, fmt.Sprintf("%d file(s), +%d -%d", stats.TotalFiles, stats.TotalAdditions, stats.TotalDeletions))
	}

	start := m.ContentScroll
	if start < 0 {
		start = 0
	}
	end := start + contentH
	if end > len(contentLines) {
		end = len(contentLines)
	}

	var visible []string
	if start < len(contentLines) {
		visible = append([]string(nil), contentLines[start:end]...)
	}

	var tabDesc, tabDiff, tabComments string
	if m.theme != nil {
		tabDesc = m.theme.TabActive.Render("● Desc")
		tabDiff = m.theme.TabInactive.Render("Diff")
		tabComments = m.theme.TabInactive.Render("Comments")
	} else {
		tabDesc = "[ ● Desc ]"
		tabDiff = "Diff"
		tabComments = "Comments"
	}
	tabsStr := tabDesc + " " + tabDiff + " " + tabComments
	
	// Add inner padding
	for i, r := range visible {
		visible[i] = " " + r
	}
	tabsStr = " " + tabsStr

	contentStr := renderBlock(visible, innerW, contentH)

	var borderColor lipgloss.Color
	if m.theme != nil {
		borderColor = m.theme.Border
	} else {
		borderColor = theme.Default().Border
	}

	// Active focus color
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



// handleKey routes keyboard input within the PR detail view.
func (m *PRDetailModel) handleKey(msg tea.KeyMsg) (*PRDetailModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, m.emitBackToDashboard()
	case "r":
		return m.handleRefresh()
	case "o":
		return m, m.emitOpenBrowser()
	case "tab":
		m.cycleForward()
	case "shift+tab":
		m.cycleBackward()
	case "j", "down":
		m.scrollDown()
	case "k", "up":
		m.scrollUp()
	case "h", "left":
		m.jumpPrevFile()
	case "l", "right":
		m.jumpNextFile()
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

// cycleForward advances focus: Files → CI (if checks) → Content → Files.
func (m *PRDetailModel) cycleForward() {
	if m.Width < MinWidthForSidebar {
		return // sidebar hidden, only Content exists
	}
	switch m.leftPanel.Focus {
	case FocusFiles:
		if len(m.leftPanel.Checks) > 0 {
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
			m.leftPanel.Focus = FocusCI
		} else {
			m.leftPanel.Focus = FocusFiles
		}
	}
}

// ─── Navigation within focused sub-area ─────────────────────────────────────

func (m *PRDetailModel) scrollDown() {
	switch m.leftPanel.Focus {
	case FocusFiles:
		if len(m.leftPanel.Files) == 0 {
			return
		}
		m.leftPanel.FileIndex++
		last := len(m.leftPanel.Files) - 1
		if m.leftPanel.FileIndex > last {
			// Boundary: if CI has checks, move focus there.
			m.leftPanel.FileIndex = last
			if len(m.leftPanel.Checks) > 0 {
				m.leftPanel.Focus = FocusCI
				m.leftPanel.CIScroll = 0
			}
			return
		}
		m.ensureFileVisible()
	case FocusCI:
		if len(m.leftPanel.Checks) == 0 {
			return
		}
		visibleCI := m.ciVisibleRows()
		m.leftPanel.CIScroll = clamp(m.leftPanel.CIScroll+1, 0, max(0, len(m.leftPanel.Checks)-visibleCI))
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
		if m.leftPanel.CIScroll <= 0 {
			// Boundary: move focus back to Files.
			m.leftPanel.Focus = FocusFiles
			m.leftPanel.FilesScroll = 0
			return
		}
		m.leftPanel.CIScroll--
	case FocusContent:
		m.ContentScroll--
		m.clampContentScroll()
	}
}

// jumpPrevFile moves the file cursor to the previous file (only when Files focused).
func (m *PRDetailModel) jumpPrevFile() {
	if m.leftPanel.Focus != FocusFiles {
		return
	}
	m.leftPanel.FileIndex = clamp(m.leftPanel.FileIndex-1, 0, max(0, len(m.leftPanel.Files)-1))
	m.ensureFileVisible()
}

// jumpNextFile moves the file cursor to the next file (only when Files focused).
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
	half := m.contentVisibleHeight() / 2
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
	half := m.contentVisibleHeight() / 2
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

// contentVisibleHeight returns the visible row count for the content viewport.
func (m *PRDetailModel) contentVisibleHeight() int {
	return max(1, m.bodyHeight())
}

// ciVisibleRows returns the visible row count within the CI sub-area.
func (m *PRDetailModel) ciVisibleRows() int {
	ciH := computeCIHeight(m.bodyHeight(), len(m.leftPanel.Checks))
	inner := ciH - 2
	contentH := inner - 2
	if contentH < 1 {
		contentH = 1
	}
	return contentH
}

// maxContentScroll returns the maximum content scroll value.
func (m *PRDetailModel) maxContentScroll() int {
	return max(0, m.totalContentRows()-m.contentVisibleHeight())
}

// totalContentRows estimates total rows in the right content viewport.
func (m *PRDetailModel) totalContentRows() int {
	rows := 3 // section headers
	if m.Detail != nil {
		rows += len(strings.Split(m.Detail.BodyExcerpt, "\n"))
	}
	if m.Diff != nil {
		for _, f := range m.Diff.Files {
			rows += f.DisplayRows
		}
	}
	return rows
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


// handleRefresh refires both load commands with force=true.
func (m *PRDetailModel) handleRefresh() (*PRDetailModel, tea.Cmd) {
	if m.PRService == nil {
		return m, nil
	}
	m.DetailLoading = true
	m.DiffLoading = true
	m.leftPanel.Loading = true
	headSHA := m.Summary.HeadRefOID
	return m, tea.Batch(
		cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true),
		cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, true),
	)
}

func (m *PRDetailModel) emitBackToDashboard() tea.Cmd {
	return func() tea.Msg { return BackToDashboard{} }
}

func (m *PRDetailModel) emitOpenBrowser() tea.Cmd {
	return func() tea.Msg {
		return OpenBrowserPR{Repo: m.Summary.Repo, Number: m.Summary.Number}
	}
}


// BackToDashboard is emitted when the user presses Esc or q in PR detail.
type BackToDashboard struct{}

// OpenBrowserPR is emitted when the user presses 'o' in PR detail.
type OpenBrowserPR struct {
	Repo   string
	Number int
}
