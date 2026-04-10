package dashboard

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
)

type RepoPanelModel struct {
	Repos       []domain.Repository
	ActiveIndex int
	Cursor      int
	Scroll      int
	Width       int
	Height      int
	OrderFrozen bool
}

func NewRepoPanelModel(repos []domain.Repository) *RepoPanelModel {
	m := &RepoPanelModel{}
	m.SetRepos(repos)
	return m
}

func (m *RepoPanelModel) Init() tea.Cmd { return nil }

func (m *RepoPanelModel) SetRect(width, height int) {
	m.Width = width
	m.Height = height
	m.ensureVisible()
}

func (m *RepoPanelModel) SetActiveIndex(index int) {
	if index < 0 || index >= len(m.Repos) {
		m.ActiveIndex = -1
		return
	}
	m.ActiveIndex = index
}

func (m *RepoPanelModel) SetRepos(repos []domain.Repository) {
	if !m.OrderFrozen {
		m.Repos = append([]domain.Repository(nil), repos...)
		sortRepos(m.Repos)
		m.OrderFrozen = true
		m.clampCursor()
		return
	}

	existing := make(map[string]domain.Repository, len(repos))
	for _, repo := range repos {
		existing[repo.FullName] = repo
	}

	nextRepos := make([]domain.Repository, 0, len(repos))
	seen := make(map[string]struct{}, len(repos))
	for _, repo := range m.Repos {
		if updated, ok := existing[repo.FullName]; ok {
			nextRepos = append(nextRepos, updated)
			seen[repo.FullName] = struct{}{}
		}
	}

	var newcomers []domain.Repository
	for _, repo := range repos {
		if _, ok := seen[repo.FullName]; !ok {
			newcomers = append(newcomers, repo)
		}
	}
	sortRepos(newcomers)
	nextRepos = append(nextRepos, newcomers...)
	m.Repos = nextRepos
	m.clampCursor()
}

func (m *RepoPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetRect(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.moveCursor(1)
			return m, nil
		case "k", "up":
			m.moveCursor(-1)
			return m, nil
		case "enter":
			if m.Cursor >= 0 && m.Cursor < len(m.Repos) {
				return m, selectRepoCmd(m.Cursor, m.Repos[m.Cursor])
			}
		}
	}
	return m, nil
}

func (m *RepoPanelModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}
	lines := []string{fitLine("Repos", m.Width)}
	visible := m.visibleRepos()
	for _, repo := range visible {
		lines = append(lines, fitLine(m.renderRepoRow(repo), m.Width))
	}
	lines = append(lines, fitLine("", m.Width))
	return renderBlock(lines, m.Width, m.Height)
}

func (m *RepoPanelModel) moveCursor(delta int) {
	if len(m.Repos) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return
	}
	m.Cursor += delta
	m.clampCursor()
	m.ensureVisible()
}

func (m *RepoPanelModel) clampCursor() {
	if len(m.Repos) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Repos) {
		m.Cursor = len(m.Repos) - 1
	}
	m.ensureVisible()
}

func (m *RepoPanelModel) ensureVisible() {
	if len(m.Repos) == 0 {
		m.Scroll = 0
		return
	}
	visible := m.visibleCount()
	if visible <= 0 {
		m.Scroll = 0
		return
	}
	if m.Cursor < m.Scroll {
		m.Scroll = m.Cursor
	}
	if m.Cursor >= m.Scroll+visible {
		m.Scroll = m.Cursor - visible + 1
	}
	maxScroll := len(m.Repos) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.Scroll > maxScroll {
		m.Scroll = maxScroll
	}
	if m.Scroll < 0 {
		m.Scroll = 0
	}
}

func (m *RepoPanelModel) visibleCount() int {
	if m.Height <= 2 {
		return 0
	}
	return m.Height - 2
}

func (m *RepoPanelModel) visibleRepos() []domain.Repository {
	if len(m.Repos) == 0 {
		return nil
	}
	visible := m.visibleCount()
	if visible <= 0 {
		return nil
	}
	start := m.Scroll
	if start < 0 {
		start = 0
	}
	if start > len(m.Repos) {
		start = len(m.Repos)
	}
	end := start + visible
	if end > len(m.Repos) {
		end = len(m.Repos)
	}
	return m.Repos[start:end]
}

func (m *RepoPanelModel) renderRepoRow(repo domain.Repository) string {
	idx := -1
	for i := range m.Repos {
		if m.Repos[i].FullName == repo.FullName {
			idx = i
			break
		}
	}
	bar := " "
	if idx == m.Cursor {
		bar = "▌"
	}
	active := " "
	if idx == m.ActiveIndex {
		active = "▶"
	}
	label := repo.FullName
	if strings.TrimSpace(label) == "" {
		label = repo.Owner + "/" + repo.Name
	}
	return strings.TrimRight(bar+" "+active+" "+label, " ")
}
