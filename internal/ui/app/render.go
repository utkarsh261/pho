package app

import (
	"strings"
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
	columns := make([]columnView, 0, 3)
	if w := m.layout.Current.Repo; w > 0 {
		columns = append(columns, columnView{width: w, view: m.repoPanel.View()})
	}
	if w := m.layout.Current.PR; w > 0 {
		columns = append(columns, columnView{width: w, view: m.prList.View()})
	}
	if w := m.layout.Current.Preview; w > 0 {
		columns = append(columns, columnView{width: w, view: m.preview.View()})
	}
	if len(columns) == 0 {
		return ""
	}
	if len(columns) == 1 {
		return joinColumn(columns[0], height)
	}
	return joinColumns(columns, height)
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
	runes := []rune(text)
	if len(runes) > width {
		runes = runes[:width]
	}
	if len(runes) == width {
		return string(runes)
	}
	return string(runes) + strings.Repeat(" ", width-len(runes))
}
