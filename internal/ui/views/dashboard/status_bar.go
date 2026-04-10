package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utk/git-term/internal/domain"
)

type StatusBarModel struct {
	Width        int
	Focus        domain.FocusTarget
	Loading      bool
	Freshness    domain.Freshness
	Errors       domain.ErrorState
	CurrentTab   domain.DashboardTab
	SelectedRepo string
}

func NewStatusBarModel() *StatusBarModel {
	return &StatusBarModel{}
}

func (m *StatusBarModel) Init() tea.Cmd { return nil }

func (m *StatusBarModel) SetRect(width int) {
	m.Width = width
}

func (m *StatusBarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		return m, nil
	}
	return m, nil
}

func (m *StatusBarModel) View() string {
	if m.Width <= 0 {
		return ""
	}
	parts := []string{m.hintText()}
	if m.Loading {
		parts = append(parts, "loading")
	}
	if freshness := strings.TrimSpace(string(m.Freshness)); freshness != "" && m.Freshness != domain.FreshnessFresh {
		parts = append(parts, freshness)
	}
	if errText := m.errorText(); errText != "" {
		parts = append(parts, errText)
	}
	return fitLine(joinVisible(parts, " | "), m.Width)
}

func (m *StatusBarModel) hintText() string {
	switch m.Focus {
	case domain.FocusRepoPanel:
		return "j/k: Move | Enter: Select | r: Refresh | Tab: Next panel"
	case domain.FocusPRListPanel:
		return "j/k: Navigate | h/l: Tab | o: Open browser | r: Refresh | Tab: Next panel"
	case domain.FocusPreviewPanel:
		return "j/k: Scroll | o/Enter: Open browser | r: Refresh | Tab: Next panel"
	case domain.FocusCmdPalette:
		return "Esc: Close | Enter: Run"
	default:
		return "Tab: Next panel | Ctrl+P: Search"
	}
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
