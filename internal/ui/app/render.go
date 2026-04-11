package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/utk/git-term/internal/domain"
)

func (m *Model) renderDashboard() string {
	if m.layout.Current.Width <= 0 || m.layout.Current.Height <= 0 {
		return ""
	}

	bodyH := m.bodyHeight()
	body := m.composeBody(bodyH)
	status := m.status.View()
	if strings.TrimSpace(body) == "" {
		return status
	}
	if strings.TrimSpace(status) == "" {
		return body
	}
	return body + "\n" + status
}

func (m *Model) composeBody(height int) string {
	type panelCol struct {
		contentW int
		view     string
		focus    domain.FocusTarget
	}
	columns := make([]panelCol, 0, 3)
	if w := m.layout.Current.Repo; w > 0 {
		columns = append(columns, panelCol{contentW: w, view: m.repoPanel.View(), focus: domain.FocusRepoPanel})
	}
	if w := m.layout.Current.PR; w > 0 {
		columns = append(columns, panelCol{contentW: w, view: m.prList.View(), focus: domain.FocusPRListPanel})
	}
	if w := m.layout.Current.Preview; w > 0 {
		columns = append(columns, panelCol{contentW: w, view: m.preview.View(), focus: domain.FocusPreviewPanel})
	}
	if len(columns) == 0 {
		return ""
	}

	// Build boxed panels. Each box = content + 2 borders + 1 gap.
	boxed := make([]string, len(columns))
	for i, col := range columns {
		boxed[i] = buildBox(col.view, col.contentW, height, col.focus == m.focus)
	}

	// Join panels side-by-side.
	if len(boxed) == 1 {
		return boxed[0]
	}
	return joinStrings(boxed, height)
}

// buildBox wraps content in a 4-sided border box.
// contentW = content area width (excl. borders).
// height = total box height (incl. top+bottom borders).
// Returns a string that is (contentW + 3) chars wide:
//   1 left border + contentW + 1 right border + 1 gap space.
func buildBox(view string, contentW, height int, focused bool) string {
	bc := lipgloss.Color("#7C3AED")
	if !focused {
		bc = lipgloss.Color("#374151")
	}
	border := lipgloss.NewStyle().Foreground(bc).Render

	contentH := height - 2
	if contentH < 1 {
		contentH = 1
	}

	lines := make([]string, 0, height)

	// Top: ┌───┐
	lines = append(lines, border("┌"+strings.Repeat("─", contentW)+"┐"))

	contentLines := strings.Split(view, "\n")
	for i := 0; i < contentH; i++ {
		line := ""
		if i < len(contentLines) {
			line = contentLines[i]
		}
		vis := lipgloss.Width(line)
		if vis < contentW {
			line += strings.Repeat(" ", contentW-vis)
		} else if vis > contentW {
			line = lipgloss.NewStyle().MaxWidth(contentW).Render(line)
		}
		lines = append(lines, border("│")+line+border("│"))
	}

	// Bottom: └───┘
	lines = append(lines, border("└"+strings.Repeat("─", contentW)+"┘"))

	// Append 1-char gap to every line (including last panel for symmetry).
	box := strings.Join(lines, "\n")
	gapped := strings.ReplaceAll(box, "\n", " \n") + " "
	return gapped
}

// joinStrings concatenates pre-built panel strings side-by-side.
// All panels already have their trailing gap char.
func joinStrings(panels []string, height int) string {
	allLines := make([]string, height)
	for _, panel := range panels {
		panelLines := strings.Split(panel, "\n")
		for i := 0; i < height && i < len(panelLines); i++ {
			allLines[i] += panelLines[i]
		}
	}
	return strings.Join(allLines, "\n")
}

type columnView struct {
	width int
	view  string
}

func joinColumns(columns []columnView, height int) string {
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		parts := make([]string, 0, len(columns))
		for _, col := range columns {
			parts = append(parts, lineAt(col.view, i, col.width))
		}
		lines[i] = strings.Join(parts, "")
	}
	return strings.Join(lines, "\n")
}

func joinColumn(col columnView, height int) string {
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		lines[i] = lineAt(col.view, i, col.width)
	}
	return strings.Join(lines, "\n")
}

func lineAt(view string, index, width int) string {
	lines := strings.Split(view, "\n")
	if index < 0 || index >= len(lines) {
		return strings.Repeat(" ", width)
	}
	return padLine(lines[index], width)
}

func padLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := lipgloss.Width(text)
	if visible == width {
		return text
	}
	if visible > width {
		return lipgloss.NewStyle().MaxWidth(width).Render(text)
	}
	return text + strings.Repeat(" ", width-visible)
}
