package overlay

import (
	"fmt"
	"math"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const (
	defaultSearchLimit = 12
	minBoxWidth        = 30
	minBoxHeight       = 10
)

// SearchService provides the results shown by the command palette.
type SearchService interface {
	SearchPRsForRepo(query, repo string, limit int) []domain.SearchResult
	SearchRepos(query string, limit int) []domain.SearchResult
}

// SelectRepo asks the root model to switch to a repo.
type SelectRepo struct {
	Repo string
}

// OpenPR asks the root model to open a pull request in the TUI detail view.
type OpenPR struct {
	Repo    string
	Number  int
	Summary domain.PullRequestSummary
}

// CloseCmdPalette dismisses the overlay.
type CloseCmdPalette struct{}

// DispatchMsg bundles ordered follow-up actions from the palette.
type DispatchMsg struct {
	Messages []tea.Msg
}

// Model is the command palette overlay state.
type Model struct {
	search SearchService
	theme  *theme.Theme

	activeRepo string
	query      string
	cursor     int

	results       []domain.SearchResult
	selectedIndex int
	scrollOffset  int

	width  int
	height int

	hydrating bool

	limit int
}

// NewModel constructs a command palette overlay model.
func NewModel(search SearchService) Model {
	return Model{
		search: search,
		limit:  defaultSearchLimit,
	}
}

func (m *Model) SetActiveRepo(repo string) {
	m.activeRepo = repo
}

func (m *Model) SetTheme(th *theme.Theme) {
	m.theme = th
}

// SetHydrating sets the loading indicator state.
func (m *Model) SetHydrating(hydrating bool) {
	m.hydrating = hydrating
}

// RefreshResults re-queries the search service and updates displayed results.
func (m *Model) RefreshResults() {
	m.refreshResults()
}

// SetResults is a test/helper hook for supplying results directly.
func (m *Model) SetResults(results []domain.SearchResult) {
	m.results = append([]domain.SearchResult(nil), results...)
	m.selectedIndex = clampIndex(m.selectedIndex, len(m.results))
	m.ensureSelectionVisible()
}

// Update handles Bubble Tea messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureSelectionVisible()
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m Model) updateKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return CloseCmdPalette{} }
	case tea.KeyEnter:
		return m, m.dispatchSelection()
	case tea.KeyUp:
		m.moveSelection(-1)
		return m, nil
	case tea.KeyDown:
		m.moveSelection(1)
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if m.deleteBeforeCursor() {
			m.refreshResults()
		}
		return m, nil
	case tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case tea.KeyRight:
		if m.cursor < len(m.query) {
			m.cursor++
		}
		return m, nil
	case tea.KeyHome:
		m.cursor = 0
		return m, nil
	case tea.KeyEnd:
		m.cursor = len(m.query)
		return m, nil
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			m.insertRunes(msg.Runes)
			m.refreshResults()
		}
		return m, nil
	default:
		switch msg.String() {
		case "j":
			m.moveSelection(1)
			return m, nil
		case "k":
			m.moveSelection(-1)
			return m, nil
		}
		return m, nil
	}
}

func (m *Model) insertRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	head := m.query[:m.cursor]
	tail := m.query[m.cursor:]
	inserted := string(runes)
	m.query = head + inserted + tail
	m.cursor += len(inserted)
}

func (m *Model) deleteBeforeCursor() bool {
	if m.cursor == 0 || len(m.query) == 0 {
		return false
	}
	head := m.query[:m.cursor-1]
	tail := m.query[m.cursor:]
	m.query = head + tail
	m.cursor--
	return true
}

func (m *Model) refreshResults() {
	if m.search == nil {
		m.results = nil
		m.selectedIndex = 0
		m.scrollOffset = 0
		return
	}

	var results []domain.SearchResult
	if m.query == "" {
		results = append([]domain.SearchResult(nil), m.search.SearchPRsForRepo("", m.activeRepo, m.limit)...)
	} else {
		prResults := m.search.SearchPRsForRepo(m.query, m.activeRepo, m.limit)
		repoResults := m.search.SearchRepos(m.query, m.limit)
		results = append(append([]domain.SearchResult(nil), prResults...), repoResults...)
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].Score != results[j].Score {
				return results[i].Score > results[j].Score
			}
			if results[i].Kind != results[j].Kind {
				return results[i].Kind < results[j].Kind
			}
			if results[i].Repo != results[j].Repo {
				return results[i].Repo < results[j].Repo
			}
			return results[i].Number < results[j].Number
		})
		if len(results) > m.limit {
			results = results[:m.limit]
		}
	}

	m.results = results
	m.selectedIndex = clampIndex(0, len(m.results))
	m.scrollOffset = 0
	m.ensureSelectionVisible()
}

func (m *Model) moveSelection(delta int) {
	if len(m.results) == 0 {
		return
	}
	m.selectedIndex = (m.selectedIndex + delta) % len(m.results)
	if m.selectedIndex < 0 {
		m.selectedIndex += len(m.results)
	}
	m.ensureSelectionVisible()
}

func (m *Model) ensureSelectionVisible() {
	visible := m.visibleResultCount()
	if visible <= 0 || len(m.results) == 0 {
		m.scrollOffset = 0
		return
	}
	if m.selectedIndex < m.scrollOffset {
		m.scrollOffset = m.selectedIndex
	}
	if m.selectedIndex >= m.scrollOffset+visible {
		m.scrollOffset = m.selectedIndex - visible + 1
	}
	maxScroll := maxInt(0, len(m.results)-visible)
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m Model) visibleResultCount() int {
	_, boxH := m.boxSize()
	if boxH <= 0 {
		return 0
	}
	return maxInt(0, boxH-6)
}

func (m Model) dispatchSelection() tea.Cmd {
	if len(m.results) == 0 || m.selectedIndex < 0 || m.selectedIndex >= len(m.results) {
		return nil
	}

	selected := m.results[m.selectedIndex]
	switch selected.Kind {
	case domain.SearchResultRepo:
		return func() tea.Msg {
			return DispatchMsg{
				Messages: []tea.Msg{
					SelectRepo{Repo: selected.Repo},
					CloseCmdPalette{},
				},
			}
		}
	case domain.SearchResultPR:
		messages := []tea.Msg{}
		if selected.Repo != "" && selected.Repo != m.activeRepo {
			messages = append(messages, SelectRepo{Repo: selected.Repo})
		}
		summary := domain.PullRequestSummary{
			Repo:        selected.Repo,
			Number:      selected.Number,
			Title:       selected.Title,
			Author:      selected.Author,
			State:       selected.State,
			IsDraft:     selected.IsDraft,
			HeadRefName: selected.Branch,
		}
		messages = append(messages, OpenPR{Repo: selected.Repo, Number: selected.Number, Summary: summary})
		return func() tea.Msg {
			return DispatchMsg{Messages: messages}
		}
	default:
		return nil
	}
}

// View renders the centered overlay box on a blank background.
func (m Model) View() string {
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return ""
	}
	box := m.renderBox(boxW, boxH)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ViewOver composites the overlay box onto bg.
// Non-box rows and the columns left/right of the box on box rows all show
// the background content unchanged — only the box footprint is replaced.
func (m Model) ViewOver(bg string) string {
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return bg
	}
	box := m.renderBox(boxW, boxH)

	bgLines := strings.Split(bg, "\n")
	boxLines := strings.Split(box, "\n")

	startRow := (m.height - boxH) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (m.width - boxW) / 2
	if startCol < 0 {
		startCol = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, boxLine := range boxLines {
		rowIdx := startRow + i
		if rowIdx < 0 || rowIdx >= len(result) {
			continue
		}
		bgLine := result[rowIdx]
		// Keep bg columns left and right of the box; replace only the box columns.
		left := ansi.Cut(bgLine, 0, startCol)
		right := ansi.Cut(bgLine, startCol+boxW, m.width)
		result[rowIdx] = left + boxLine + right
	}
	return strings.Join(result, "\n")
}

func (m Model) renderBox(boxW, boxH int) string {
	innerW := maxInt(0, boxW-2)
	innerH := maxInt(0, boxH-2)

	content := m.bodyLines(innerW, innerH)

	if m.theme != nil {
		innerContent := strings.Join(content, "\n")
		// Width/Height are CONTENT dimensions in lipgloss — use innerW/innerH so the
		// rendered box is exactly boxW × boxH (inner + 2 border chars each side).
		return m.theme.BoxBorder.Width(innerW).Height(innerH).Render(innerContent)
	}

	lines := make([]string, 0, boxH)
	lines = append(lines, "┌"+strings.Repeat("─", innerW)+"┐")
	for _, line := range content {
		lines = append(lines, "│"+padRight(truncate(line, innerW), innerW)+"│")
	}
	lines = append(lines, "└"+strings.Repeat("─", innerW)+"┘")
	return strings.Join(lines, "\n")
}

func (m Model) bodyLines(innerW, innerH int) []string {
	if m.theme == nil {
		return m.bodyLinesPlain(innerW, innerH)
	}

	badge := m.theme.BoxTitle.Render("Go to")
	title := lipgloss.PlaceHorizontal(innerW, lipgloss.Center, badge)
	query := m.queryLine(innerW)
	divider := m.theme.BoxDiv.Render(strings.Repeat("─", innerW))
	lines := []string{title, query, divider}

	visible := m.visibleResultsForBox(innerH)
	for i, result := range visible {
		absoluteIndex := m.scrollOffset + i
		isSelected := absoluteIndex == m.selectedIndex
		var line string
		if isSelected {
			line = m.theme.BoxSelected.Width(innerW).Render("  " + formatResult(result, innerW-2))
		} else {
			line = "  " + m.formatResultStyled(result, innerW-2)
		}
		lines = append(lines, line)
	}

	footer := m.footerHint()
	if footer != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.BoxFooter.Render(truncate(footer, innerW)))
	}

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) bodyLinesPlain(innerW, innerH int) []string {
	title := centerText("Go to", innerW)
	query := m.queryLine(innerW)
	divider := strings.Repeat("─", innerW)
	lines := []string{title, query, divider}

	visible := m.visibleResultsForBox(innerH)
	for i, result := range visible {
		absoluteIndex := m.scrollOffset + i
		line := "  " + formatResult(result, innerW-2)
		_ = absoluteIndex
		lines = append(lines, line)
	}

	footer := m.footerHint()
	if footer != "" {
		lines = append(lines, "")
		lines = append(lines, truncate(footer, innerW))
	}

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) visibleResultsForBox(innerH int) []domain.SearchResult {
	if innerH <= 0 || len(m.results) == 0 {
		return nil
	}
	start := max(m.scrollOffset, 0)
	available := innerH - 6
	if footer := m.footerHint(); footer != "" {
		available -= 2
	}
	if available < 0 {
		available = 0
	}
	end := min(start+available, len(m.results))
	if start > end {
		start = end
	}
	return append([]domain.SearchResult(nil), m.results[start:end]...)
}

func (m Model) queryLine(innerW int) string {
	beforeCursor := m.query[:minInt(m.cursor, len(m.query))]
	afterCursor := ""
	if m.cursor < len(m.query) {
		afterCursor = m.query[m.cursor:]
	}
	var cursorMark string
	if m.theme != nil {
		cursorMark = m.theme.BoxCursor.Render("▏")
	} else {
		cursorMark = "▏"
	}
	line := "  " + beforeCursor + cursorMark + afterCursor
	return truncate(line, innerW)
}

func (m Model) footerHint() string {
	if m.hydrating {
		return "  Loading…"
	}
	return ""
}

func (m Model) boxSize() (int, int) {
	if m.width <= 0 || m.height <= 0 {
		return 0, 0
	}
	boxW := int(math.Round(float64(m.width) * 0.6))
	boxH := int(math.Round(float64(m.height) * 0.4))
	if boxW < minBoxWidth {
		boxW = minBoxWidth
	}
	if boxH < minBoxHeight {
		boxH = minBoxHeight
	}
	if boxW > m.width {
		boxW = m.width
	}
	if boxH > m.height {
		boxH = m.height
	}
	return boxW, boxH
}

func prStateGlyph(state domain.PRState, isDraft bool) string {
	if isDraft {
		return "○"
	}
	switch state {
	case domain.PRStateOpen:
		return "◆"
	case domain.PRStateMerged:
		return "✓"
	case domain.PRStateClosed:
		return "✕"
	default:
		return "◆"
	}
}

func (m Model) glyphStyle(state domain.PRState, isDraft bool) lipgloss.Style {
	if m.theme == nil {
		return lipgloss.NewStyle()
	}
	if isDraft {
		return m.theme.BoxGlyphDraft
	}
	switch state {
	case domain.PRStateOpen:
		return m.theme.BoxGlyphOpen
	case domain.PRStateMerged:
		return m.theme.BoxGlyphMerged
	case domain.PRStateClosed:
		return m.theme.BoxGlyphClosed
	default:
		return m.theme.BoxGlyphOpen
	}
}

// formatResultStyled renders a result row with per-column theme colours (normal/unselected rows only).
func (m Model) formatResultStyled(result domain.SearchResult, width int) string {
	if m.theme == nil {
		return formatResult(result, width)
	}
	switch result.Kind {
	case domain.SearchResultRepo:
		return m.theme.BoxGlyphOpen.Render(truncate(result.Repo, width))
	case domain.SearchResultPR:
		return m.formatPRResultStyled(result, width)
	default:
		text := result.Title
		if text == "" {
			text = result.Repo
		}
		return m.theme.BoxNormal.Render(truncate(text, width))
	}
}

func (m Model) formatPRResultStyled(result domain.SearchResult, width int) string {
	if width <= 0 {
		return ""
	}
	glyph := prStateGlyph(result.State, result.IsDraft)
	numPadded := fmt.Sprintf("%-5d", result.Number)

	// Layout: glyph(1) + " #"(2) + numPadded(5) + "  "(2) = 10 cells prefix
	prefixCells := 10
	remaining := width - prefixCells
	if remaining < 0 {
		remaining = 0
	}

	authorMax := remaining / 4
	if authorMax > 12 {
		authorMax = 12
	}
	if authorMax < 0 {
		authorMax = 0
	}

	titleMax := remaining
	authorStr := ""
	sepStr := ""
	if authorMax > 0 && result.Author != "" {
		authorStr = truncate(result.Author, authorMax)
		sepStr = "  @"
		titleMax = remaining - 3 - authorMax
		if titleMax < 0 {
			titleMax = 0
		}
	}
	titleStr := padRight(truncate(result.Title, titleMax), titleMax)

	coloredGlyph := m.glyphStyle(result.State, result.IsDraft).Render(glyph)
	coloredNum := m.theme.BoxPRNum.Render("#" + numPadded)
	coloredTitle := m.theme.BoxNormal.Render(titleStr)

	line := coloredGlyph + " " + coloredNum + "  " + coloredTitle
	if authorStr != "" {
		line += m.theme.BoxPRAuthor.Render(sepStr+authorStr)
	}
	return line
}

func formatResult(result domain.SearchResult, width int) string {
	switch result.Kind {
	case domain.SearchResultRepo:
		return truncate(result.Repo, width)
	case domain.SearchResultPR:
		return formatPRResult(result, width)
	default:
		text := result.Title
		if text == "" {
			text = result.Repo
		}
		return truncate(text, width)
	}
}

func formatPRResult(result domain.SearchResult, width int) string {
	if width <= 0 {
		return ""
	}
	glyph := prStateGlyph(result.State, result.IsDraft)
	// glyph is 1 terminal cell but len(glyph) may be 3 bytes for multi-byte UTF-8 chars
	byteOverhead := len(glyph) - 1

	numStr := fmt.Sprintf("%-5d", result.Number)
	// prefix cells: glyph(1) + " #"(2) + numStr(5) + "  "(2) = 10
	prefixCells := 10
	remaining := width - prefixCells
	if remaining < 0 {
		remaining = 0
	}

	authorMax := remaining / 4
	if authorMax > 12 {
		authorMax = 12
	}
	if authorMax < 0 {
		authorMax = 0
	}

	titleMax := remaining
	sepPart := ""
	authorPart := ""
	if authorMax > 0 && result.Author != "" {
		sepPart = "  @"
		authorPart = truncate(result.Author, authorMax)
		titleMax = remaining - 3 - authorMax
		if titleMax < 0 {
			titleMax = 0
		}
	}

	titlePart := padRight(truncate(result.Title, titleMax), titleMax)
	line := glyph + " #" + numStr + "  " + titlePart + sepPart + authorPart
	// compensate for the extra bytes of the multi-byte glyph when truncating
	return truncate(line, width+byteOverhead)
}

func centerText(text string, width int) string {
	text = truncate(text, width)
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(text)
}

func padRight(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = truncate(text, width)
	return lipgloss.NewStyle().Width(width).Render(text)
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(text) <= width {
		return text
	}
	if width <= 1 {
		return text[:width]
	}
	return text[:width-1] + "…"
}

func clampIndex(index, size int) int {
	if size <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= size {
		return size - 1
	}
	return index
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
