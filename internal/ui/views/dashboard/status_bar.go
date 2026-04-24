package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

type StatusBarModel struct {
	Width        int
	Focus        domain.FocusTarget
	Loading      bool
	Freshness    domain.Freshness
	Errors       domain.ErrorState
	CurrentTab   domain.DashboardTab
	SelectedRepo string
	HintOverride string // when non-empty, replaces focus-based hint text
	theme        *theme.Theme
	spinner      spinner.Model

	searchActive bool
	searchQuery  string
	searchIndex  int
	searchCount  int
}

func NewStatusBarModel() *StatusBarModel {
	s := spinner.New(spinner.WithSpinner(spinner.Points))
	s.Spinner.FPS = time.Millisecond * 8
	return &StatusBarModel{
		spinner: s,
	}
}

func (m *StatusBarModel) Init() tea.Cmd { return m.spinner.Tick }

func (m *StatusBarModel) SetRect(width int) {
	m.Width = width
}

func (m *StatusBarModel) SetTheme(th *theme.Theme) {
	m.theme = th
	if m.theme != nil {
		m.spinner.Style = lipgloss.NewStyle().Foreground(m.theme.Warning)
	}
}

// SetSearchState controls the temporary search text shown in the status bar.
// SetSearchState("", 0, 0) clears search mode and restores normal help text.
func (m *StatusBarModel) SetSearchState(query string, matchIndex, matchCount int) {
	if query == "" && matchIndex == 0 && matchCount == 0 {
		m.searchActive = false
		m.searchQuery = ""
		m.searchIndex = 0
		m.searchCount = 0
		return
	}

	m.searchActive = true
	m.searchQuery = m.truncateSearchQuery(query)
	m.searchIndex = max(0, matchIndex)
	m.searchCount = max(0, matchCount)
}

func (m *StatusBarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		return m, spinCmd
	}
	return m, spinCmd
}

func (m *StatusBarModel) View() string {
	if m.Width <= 0 {
		return ""
	}

	// Top border
	border := strings.Repeat("─", m.Width)
	if m.theme != nil {
		if len(m.Errors.Errors) > 0 {
			border = lipgloss.NewStyle().Foreground(m.theme.Error).Render(border)
		} else {
			border = m.theme.StatusSep.Render(border)
		}
	}

	helpText, searchError := m.searchHelpText()
	if m.theme != nil {
		if searchError {
			helpText = m.theme.StatusError.Render(helpText)
		} else if m.searchActive {
			helpText = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FDE047")).
				Bold(true).
				Render(helpText)
		} else {
			helpText = m.theme.StatusHelp.Render(helpText)
		}
	}

	parts := []string{helpText}

	if m.Loading {
		parts = append(parts, m.spinner.View())
	}

	if freshness := strings.TrimSpace(string(m.Freshness)); freshness != "" && m.Freshness != domain.FreshnessFresh {
		fresh := freshness
		if m.theme != nil {
			fresh = m.theme.StatusStale.Render(fresh)
		}
		parts = append(parts, fresh)
	}

	if errText := m.errorText(); errText != "" {
		err := errText
		if m.theme != nil {
			err = m.theme.StatusError.Render(err)
		}
		parts = append(parts, err)
	}

	sep := " | "
	if m.theme != nil {
		sep = m.theme.StatusSep.Render(" │ ")
	}
	joined := fitLine(joinVisible(parts, sep), m.Width)
	return border + "\n" + joined
}

func (m *StatusBarModel) hintText() string {
	if m.HintOverride != "" {
		return m.HintOverride
	}
	switch m.Focus {
	case domain.FocusRepoPanel:
		return "j/k: Move | Enter: Select | R: Refresh | Tab: Next panel | Ctrl+P: Jump to PR/Repository"
	case domain.FocusPRListPanel:
		return "j/k: Navigate | h/l: Tab | Enter: Open detailed view | o: Open browser | R: Refresh | Tab: Next panel | Ctrl+P: Jump to PR/Repository"
	case domain.FocusPreviewPanel:
		return "j/k: Scroll | Enter: Open detailed view | o: Open browser | R: Refresh | Tab: Next panel"
	case domain.FocusCmdPalette:
		return "Esc: Close | Enter: Run"
	default:
		return "Tab: Next panel | Ctrl+P: Search"
	}
}

func (m *StatusBarModel) searchHelpText() (text string, isError bool) {
	if !m.searchActive {
		return m.hintText(), false
	}
	if m.searchQuery == "" {
		return "/ _", false
	}
	if m.searchCount == 0 {
		return fmt.Sprintf("/ %s  0 matches", m.searchQuery), true
	}
	idx := m.searchIndex
	if idx <= 0 {
		idx = 1
	}
	if idx > m.searchCount {
		idx = m.searchCount
	}
	return fmt.Sprintf("/ %s  %d/%d matches", m.searchQuery, idx, m.searchCount), false
}

func (m *StatusBarModel) truncateSearchQuery(query string) string {
	if query == "" {
		return ""
	}
	budget := 32
	if m.Width > 0 {
		budget = max(8, m.Width/3)
	}
	r := []rune(query)
	if len(r) <= budget {
		return query
	}
	if budget <= 1 {
		return "…"
	}
	return string(r[:budget-1]) + "…"
}

func (m *StatusBarModel) errorText() string {
	if len(m.Errors.Errors) == 0 {
		return ""
	}
	err := m.Errors.Errors[0]
	switch err.Kind {
	case domain.ErrorKindAuth:
		if strings.TrimSpace(err.Message) == "" {
			return "auth: gh auth login"
		}
		return "auth: " + err.Message
	case domain.ErrorKindRateLimit:
		if m.Errors.RateLimitReset != nil {
			return fmt.Sprintf("rate limit: retry at %s", formatTimestamp(*m.Errors.RateLimitReset))
		}
		return "rate limit"
	case domain.ErrorKindNetwork:
		return "network: " + err.Message
	case domain.ErrorKindCache:
		return "cache: " + err.Message
	case domain.ErrorKindDiscovery:
		return "discovery: " + err.Message
	case domain.ErrorKindParse:
		return "parse: " + err.Message
	default:
		if strings.TrimSpace(err.Message) != "" {
			return strings.ToLower(string(err.Kind)) + ": " + err.Message
		}
		return string(err.Kind)
	}
}

func (m *StatusBarModel) SetRateLimitReset(t time.Time) {
	m.Errors.RateLimitReset = &t
}
