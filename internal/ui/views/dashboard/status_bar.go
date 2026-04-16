package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
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
	theme        *theme.Theme
	spinner      spinner.Model
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

	helpText := m.hintText()
	if m.theme != nil {
		helpText = m.theme.StatusHelp.Render(helpText)
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
