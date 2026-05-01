// Package keymapoverlay implements a lazygit-style contextual keybinding help overlay.
package keymapoverlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

// ToggleKeymapOverlay is emitted when the user presses ?.
type ToggleKeymapOverlay struct{}

// CloseKeymapOverlay dismisses the overlay.
type CloseKeymapOverlay struct{}

// Binding maps a key string to a human-readable description.
type Binding struct {
	Key         string
	Description string
}

// Group is a named collection of bindings.
type Group struct {
	Name     string
	Bindings []Binding
}

// Context describes the UI state used to select bindings.
type Context struct {
	View       domain.PrimaryView
	Focus      domain.FocusTarget
	Tab        prdetail.ContentTab // only meaningful when View == PRDetail
	PRState    domain.PRState      // only meaningful when View == PRDetail
	DraftCount int                 // only meaningful when View == PRDetail
}

// Model is the keymap overlay state.
type Model struct {
	Visible bool
	Groups  []Group
	width   int
	height  int
	theme   *theme.Theme
}

// NewModel creates an empty keymap overlay model.
func NewModel() Model {
	return Model{}
}

// SetTheme applies a theme.
func (m *Model) SetTheme(th *theme.Theme) {
	m.theme = th
}

// SetSize updates terminal dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "?", "esc", "q":
			m.Visible = false
			return m, nil
		}
	}
	return m, nil
}

// View renders the overlay centered on a blank background.
func (m Model) View() string {
	if !m.Visible || len(m.Groups) == 0 {
		return ""
	}
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return ""
	}
	box := m.renderBox(boxW, boxH)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ViewOver composites the overlay onto bg without dimming.
func (m Model) ViewOver(bg string) string {
	if !m.Visible || len(m.Groups) == 0 {
		return bg
	}
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
		left := ansi.Cut(bgLine, 0, startCol)
		right := ansi.Cut(bgLine, startCol+boxW, m.width)
		result[rowIdx] = left + boxLine + right
	}
	return strings.Join(result, "\n")
}

func (m Model) boxSize() (int, int) {
	if m.width <= 0 || m.height <= 0 {
		return 0, 0
	}
	// Width: fixed max of 80, with padding.
	boxW := 80
	if boxW > m.width-4 {
		boxW = m.width - 4
	}
	if boxW < 30 {
		boxW = 30
	}

	// Height: content-driven.
	innerW := maxInt(0, boxW-2)
	contentLines := m.contentLines(innerW)
	totalLines := len(contentLines)
	boxH := totalLines + 2 // +2 for borders
	if boxH > m.height-4 {
		boxH = m.height - 4
	}
	if boxH < 10 {
		boxH = 10
	}
	return boxW, boxH
}

func (m Model) renderBox(boxW, boxH int) string {
	innerW := maxInt(0, boxW-2)
	innerH := maxInt(0, boxH-2)

	content := m.contentLines(innerW)
	if len(content) > innerH {
		content = content[:innerH]
	}
	for len(content) < innerH {
		content = append(content, "")
	}

	if m.theme != nil {
		innerContent := strings.Join(content, "\n")
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

func (m Model) contentLines(innerW int) []string {
	if m.theme != nil {
		return m.contentLinesThemed(innerW)
	}
	return m.contentLinesPlain(innerW)
}

func (m Model) contentLinesThemed(innerW int) []string {
	th := m.theme
	lines := []string{}

	// Title
	badge := th.BoxTitle.Render("Keybindings")
	title := lipgloss.PlaceHorizontal(innerW, lipgloss.Center, badge)
	lines = append(lines, title)
	lines = append(lines, th.BoxDiv.Render(strings.Repeat("─", innerW)))

	for gi, group := range m.Groups {
		if gi > 0 {
			lines = append(lines, "")
		}
		// Category header
		header := th.Bold.Render(group.Name)
		lines = append(lines, header)
		lines = append(lines, th.BoxDiv.Render(strings.Repeat("─", innerW)))

		for _, b := range group.Bindings {
			keyStr := th.Keycap.Render(fmt.Sprintf(" %s ", b.Key))
			// Calculate remaining width for description
			keyW := lipgloss.Width(keyStr)
			descBudget := innerW - keyW - 2 // 2 for spacing
			if descBudget < 5 {
				descBudget = 5
			}
			descStr := truncate(b.Description, descBudget)
			line := keyStr + "  " + th.BoxNormal.Render(descStr)
			lines = append(lines, line)
		}
	}

	// Footer hint
	lines = append(lines, "")
	footer := th.BoxFooter.Render("? / esc / q to close")
	lines = append(lines, lipgloss.PlaceHorizontal(innerW, lipgloss.Center, footer))

	return lines
}

func (m Model) contentLinesPlain(innerW int) []string {
	lines := []string{}
	lines = append(lines, centerText("Keybindings", innerW))
	lines = append(lines, strings.Repeat("─", innerW))

	for gi, group := range m.Groups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, group.Name)
		lines = append(lines, strings.Repeat("─", innerW))
		for _, b := range group.Bindings {
			keyStr := fmt.Sprintf("[%s]", b.Key)
			descBudget := innerW - len(keyStr) - 2
			if descBudget < 5 {
				descBudget = 5
			}
			line := keyStr + "  " + truncate(b.Description, descBudget)
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	lines = append(lines, centerText("? / esc / q to close", innerW))
	return lines
}

// BuildBindings returns contextual binding groups for the given context.
func BuildBindings(ctx Context) []Group {
	switch ctx.View {
	case domain.PrimaryViewPRDetail:
		return buildPRDetailBindings(ctx)
	case domain.PrimaryViewCommitDetail:
		return buildCommitDetailBindings()
	default:
		return buildDashboardBindings(ctx)
	}
}

func buildDashboardBindings(ctx Context) []Group {
	groups := []Group{
		{
			Name: "Navigation",
			Bindings: []Binding{
				{Key: "j/k", Description: "Move up/down"},
				{Key: "gg/G", Description: "Jump top/bottom"},
				{Key: "tab", Description: "Cycle focus"},
				{Key: "shift+tab", Description: "Cycle focus back"},
			},
		},
	}

	// Focus-specific group first
	switch ctx.Focus {
	case domain.FocusRepoPanel:
		groups = append(groups, Group{
			Name: "Repo Panel",
			Bindings: []Binding{
				{Key: "j/k", Description: "Move up/down"},
				{Key: "enter", Description: "Select repo"},
			},
		})
	case domain.FocusPRListPanel:
		groups = append(groups, Group{
			Name: "PR List",
			Bindings: []Binding{
				{Key: "j/k", Description: "Move up/down"},
				{Key: "h/l", Description: "Prev/next tab"},
				{Key: "enter", Description: "Open PR detail"},
			},
		})
	case domain.FocusPreviewPanel:
		groups = append(groups, Group{
			Name: "Preview",
			Bindings: []Binding{
				{Key: "j/k", Description: "Scroll preview"},
				{Key: "enter", Description: "Open PR detail"},
			},
		})
	}

	groups = append(groups, Group{
		Name: "Actions",
		Bindings: []Binding{
			{Key: "o", Description: "Open in browser"},
			{Key: "R", Description: "Refresh"},
		},
	})

	groups = append(groups, Group{
		Name: "Global",
		Bindings: []Binding{
			{Key: "ctrl+p", Description: "Jump to PR/Repo"},
			{Key: "q", Description: "Quit"},
			{Key: "?", Description: "Keybindings"},
		},
	})

	return groups
}

func buildCommitDetailBindings() []Group {
	return []Group{
		{
			Name: "Navigation",
			Bindings: []Binding{
				{Key: "j/k", Description: "Scroll / Move cursor"},
				{Key: "tab", Description: "Cycle panel"},
				{Key: "h/l", Description: "Focus files / content"},
				{Key: "gg/G", Description: "Jump top/bottom"},
				{Key: "shift+H/shift+L", Description: "Prev/next file"},
			},
		},
		{
			Name: "Actions",
			Bindings: []Binding{
				{Key: "o", Description: "Open in browser"},
				{Key: "y", Description: "Copy SHA"},
				{Key: "R", Description: "Refresh"},
				{Key: "esc / q", Description: "Back to PR detail"},
			},
		},
	}
}

func buildPRDetailBindings(ctx Context) []Group {
	groups := []Group{
		{
			Name: "Navigation",
			Bindings: []Binding{
				{Key: "j/k", Description: "Scroll / Move cursor"},
				{Key: "tab", Description: "Cycle panel"},
				{Key: "h/l", Description: "Focus files / content"},
				{Key: "gg/G", Description: "Jump top/bottom"},
			},
		},
		{
			Name: "Tabs",
			Bindings: []Binding{
			{Key: "1", Description: "Description"},
			{Key: "2", Description: "Diff"},
			{Key: "3", Description: "Comments"},
			{Key: "4", Description: "Commits"},
			},
		},
		{
			Name: "Actions",
			Bindings: func() []Binding {
				b := []Binding{
					{Key: "o", Description: "Open in browser"},
					{Key: "R", Description: "Refresh"},
					{Key: "C", Description: "Comment"},
					{Key: "a", Description: "Approve"},
					{Key: "v", Description: "Review"},
					{Key: "m", Description: "Merge"},
				}
				if ctx.PRState == domain.PRStateOpen {
					b = append(b, Binding{Key: "x", Description: "Close"})
				} else if ctx.PRState == domain.PRStateClosed {
					b = append(b, Binding{Key: "x", Description: "Reopen"})
				}
				if ctx.DraftCount > 0 {
					b = append(b, Binding{Key: "D", Description: "Discard all drafts"})
				}
				b = append(b, Binding{Key: "esc / q", Description: "Back to dashboard"})
				return b
			}(),
		},
		{
			Name: "Search",
			Bindings: []Binding{
				{Key: "/", Description: "Search diff"},
				{Key: "n/N", Description: "Next/prev match"},
			},
		},
	}

	// Tab-specific groups
	switch ctx.Tab {
	case prdetail.TabDiff:
		groups = append(groups, Group{
			Name: "Diff",
			Bindings: []Binding{
				{Key: "space", Description: "Visual mode"},
				{Key: "J/K", Description: "Jump 5 lines"},
				{Key: "ctrl+d/u", Description: "Half page"},
			},
		})
	case prdetail.TabComments:
		groups = append(groups, Group{
			Name: "Comments",
			Bindings: []Binding{
				{Key: "j/k", Description: "Next/prev comment"},
				{Key: "enter", Description: "Jump to code"},
				{Key: "r", Description: "Reply"},
			},
		})
	}

	groups = append(groups, Group{
		Name: "Global",
		Bindings: []Binding{
			{Key: "?", Description: "Keybindings"},
		},
	})

	return groups
}

// Helpers

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
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= 1 {
		return text[:width]
	}
	return text[:width-1] + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
