package overlay

import (
	"fmt"
	"math"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/ui/theme"
)

const (
	defaultSearchLimit = 12
	minBoxWidth        = 30
	minBoxHeight       = 10
)

// SearchService provides the results shown by the command palette.
type SearchService interface {
	SearchPRs(query string, limit int) []domain.SearchResult
	SearchRepos(query string, limit int) []domain.SearchResult
}

// SelectRepo asks the root model to switch to a repo.
type SelectRepo struct {
	Repo string
}

// OpenPR asks the root model to open a pull request in the browser.
type OpenPR struct {
	Repo   string
	Number int
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

	totalRepos    int
	hydratedRepos int

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

// SetRepoHydrationStats updates the footer hint counts.
func (m *Model) SetRepoHydrationStats(totalRepos, hydratedRepos int) {
	if totalRepos < 0 {
		totalRepos = 0
	}
	if hydratedRepos < 0 {
		hydratedRepos = 0
	}
	if hydratedRepos > totalRepos {
		hydratedRepos = totalRepos
	}
	m.totalRepos = totalRepos
	m.hydratedRepos = hydratedRepos
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

	results := append([]domain.SearchResult(nil), m.search.SearchRepos(m.query, m.limit)...)
	results = append(results, m.search.SearchPRs(m.query, m.limit)...)
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
				},
			}
		}
	case domain.SearchResultPR:
		messages := []tea.Msg{}
		if selected.Repo != "" && selected.Repo != m.activeRepo {
			messages = append(messages, SelectRepo{Repo: selected.Repo})
		}
		messages = append(messages, OpenPR{Repo: selected.Repo, Number: selected.Number})
		return func() tea.Msg {
			return DispatchMsg{Messages: messages}
		}
	default:
		return nil
	}
}

// View renders the centered overlay box.
func (m Model) View() string {
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return ""
	}
	box := m.renderBox(boxW, boxH)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderBox(boxW, boxH int) string {
	innerW := maxInt(0, boxW-2)
	innerH := maxInt(0, boxH-2)

	content := m.bodyLines(innerW, innerH)

	if m.theme != nil {
		innerContent := strings.Join(content, "\n")
		return m.theme.BoxBorder.Width(boxW).Height(boxH).Render(innerContent)
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
	title := centerText("Command Palette", innerW)
	query := m.queryLine(innerW)
	if m.theme != nil {
		title = m.theme.BoxTitle.Render(title)
		query = m.theme.BoxQuery.Render(query)
	}
	lines := []string{
		title,
		query,
		"",
	}

	visible := m.visibleResultsForBox(innerH)
	for i, result := range visible {
		absoluteIndex := m.scrollOffset + i
		prefix := "  "
		if absoluteIndex == m.selectedIndex {
			prefix = "> "
		}
		line := prefix + formatResult(result, innerW-2)
		if m.theme != nil {
			if absoluteIndex == m.selectedIndex {
				line = m.theme.BoxSelected.Render(line)
			} else {
				line = m.theme.BoxNormal.Render(line)
			}
		}
		lines = append(lines, line)
	}

	footer := m.footerHint()
	if footer != "" {
		lines = append(lines, "")
		footerLine := truncate(footer, innerW)
		if m.theme != nil {
			footerLine = m.theme.BoxFooter.Render(footerLine)
		}
		lines = append(lines, footerLine)
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
	prefix := "Query: "
	cursorMarker := ""
	if m.cursor == len(m.query) {
		cursorMarker = "▏"
	}
	line := prefix + m.query[:minInt(m.cursor, len(m.query))] + cursorMarker
	if m.cursor < len(m.query) {
		line += m.query[m.cursor:]
	}
	return truncate(line, innerW)
}

func (m Model) footerHint() string {
	if m.totalRepos <= 0 || m.hydratedRepos >= m.totalRepos {
		return ""
	}
	unhydrated := m.totalRepos - m.hydratedRepos
	return fmt.Sprintf("%d of %d repos still hydrating; results may be incomplete", unhydrated, m.totalRepos)
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

func formatResult(result domain.SearchResult, width int) string {
	var text string
	switch result.Kind {
	case domain.SearchResultRepo:
		text = fmt.Sprintf("REPO %s", result.Repo)
		if result.Title != "" {
			text += " - " + result.Title
		}
	case domain.SearchResultPR:
		text = fmt.Sprintf("PR #%d %s", result.Number, result.Repo)
		if result.Title != "" {
			text += " - " + result.Title
		}
	default:
		text = result.Title
		if text == "" {
			text = result.Repo
		}
	}
	return truncate(text, width)
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
