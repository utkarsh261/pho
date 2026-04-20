// Package prdetail implements the PR detail view model.
// It manages the PR detail state and handles keyboard routing within
// the PR detail view.
package prdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/model"
	diffsearch "github.com/utkarsh261/pho/internal/diff/search"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

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

// contentViewportHeight returns the number of visible rows in the content text area.
// Derived from the terminal height by subtracting the header box, the tab headBox,
// and body-box borders.
func (m *PRDetailModel) contentViewportHeight() int {
	bodyH := max(m.Height-3, 1)
	innerH := max(bodyH-4, 1)
	return max(innerH-2, 1)
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
	m.leftPanel.Focus = FocusContent
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
		m.normalizeDiffRows()
		m.searchIndex = nil
		m.refreshSearchMatches()
		// Sync files into left panel.
		m.leftPanel.Files = m.Diff.Files
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

	bodyH := max(m.Height-3, 1)

	var body string
	if m.Width >= MinWidthForSidebar {
		rightWidth := max(m.Width-LeftPanelWidth-2, 10)
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

// handleKey routes keyboard input within the PR detail view.
func (m *PRDetailModel) handleKey(msg tea.KeyMsg) (*PRDetailModel, tea.Cmd) {
	if m.searchActive && m.handleSearchKey(msg) {
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
		} else {
			return m, m.emitBackToDashboard()
		}
	case "q":
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
	case "enter":
		if m.leftPanel.Focus == FocusFiles {
			m.jumpToFile(m.leftPanel.FileIndex)
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
	case "2":
		m.jumpToSection(2)
	case "3":
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
	contentWidth := contentViewportWidth(m.rightPanelWidth())
	sections := m.buildContentSections(contentWidth)
	diffSec, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		return
	}
	fileOffset := 0
	for i := 0; i < idx; i++ {
		fileOffset += diffFileDisplayRows(&m.Diff.Files[i])
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
			// move focus back to Files.
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

func (m *PRDetailModel) emitBackToDashboard() tea.Cmd {
	return func() tea.Msg { return BackToDashboard{} }
}

func (m *PRDetailModel) emitOpenBrowser() tea.Cmd {
	return func() tea.Msg {
		return OpenBrowserPR{Repo: m.Summary.Repo, Number: m.Summary.Number}
	}
}

// BackToDashboard is emitted when the user presses q (or Esc while search is inactive) in PR detail.
type BackToDashboard struct{}

// OpenBrowserPR is emitted when the user presses 'o' in PR detail.
type OpenBrowserPR struct {
	Repo   string
	Number int
}
