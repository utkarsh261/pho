package overlay

import (
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const (
	defaultSearchLimit = 12
	minBoxWidth        = 30
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

	// pickMode turns the palette into a plain repo chooser (used by the
	// `pho pr <n>` deep link): results are pickPool filtered in-memory,
	// independent of the search index.
	pickMode bool
	pickPool []domain.SearchResult
	pickHint string
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

// OpenRepoPick switches the palette into repo-pick mode listing the given
// repos, with hint as the box title.
func (m *Model) OpenRepoPick(repos []domain.Repository, hint string) {
	m.pickMode = true
	m.pickHint = hint
	m.pickPool = make([]domain.SearchResult, 0, len(repos))
	for _, r := range repos {
		m.pickPool = append(m.pickPool, domain.SearchResult{
			Kind:  domain.SearchResultRepo,
			Repo:  r.FullName,
			Title: r.FullName,
		})
	}
	m.refreshResults()
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
	case tea.KeyBackspace:
		if m.deleteBeforeCursor() {
			m.refreshResults()
		}
		return m, nil
	case tea.KeyDelete:
		if m.deleteAfterCursor() {
			m.refreshResults()
		}
		return m, nil
	case tea.KeyLeft:
		_, size := utf8.DecodeLastRuneInString(m.query[:m.cursor])
		m.cursor -= size
		return m, nil
	case tea.KeyRight:
		_, size := utf8.DecodeRuneInString(m.query[m.cursor:])
		m.cursor += size
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

// The cursor is a byte offset into query, always on a rune boundary.

func (m *Model) deleteBeforeCursor() bool {
	_, size := utf8.DecodeLastRuneInString(m.query[:m.cursor])
	if size == 0 {
		return false
	}
	m.query = m.query[:m.cursor-size] + m.query[m.cursor:]
	m.cursor -= size
	return true
}

func (m *Model) deleteAfterCursor() bool {
	_, size := utf8.DecodeRuneInString(m.query[m.cursor:])
	if size == 0 {
		return false
	}
	m.query = m.query[:m.cursor] + m.query[m.cursor+size:]
	return true
}

func (m *Model) refreshResults() {
	if m.pickMode {
		m.results = filterRepoPool(m.pickPool, m.query)
		m.selectedIndex = clampIndex(0, len(m.results))
		m.scrollOffset = 0
		m.ensureSelectionVisible()
		return
	}

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

func filterRepoPool(pool []domain.SearchResult, query string) []domain.SearchResult {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return append([]domain.SearchResult(nil), pool...)
	}
	out := make([]domain.SearchResult, 0, len(pool))
	for _, r := range pool {
		if strings.Contains(strings.ToLower(r.Repo), q) {
			out = append(out, r)
		}
	}
	return out
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
			ID:                selected.ID,
			Repo:              selected.Repo,
			Number:            selected.Number,
			Title:             selected.Title,
			Author:            selected.Author,
			State:             selected.State,
			IsDraft:           selected.IsDraft,
			HeadRefName:       selected.Branch,
			IsCrossRepository: selected.IsCrossRepository,
		}
		messages = append(messages, OpenPR{Repo: selected.Repo, Number: selected.Number, Summary: summary})
		return func() tea.Msg {
			return DispatchMsg{Messages: messages}
		}
	default:
		return nil
	}
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
