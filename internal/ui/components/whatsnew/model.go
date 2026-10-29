// Package whatsnew is the release notes panel opened with W.
package whatsnew

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/ui/markdown"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const (
	loadingText = "Loading release notes…"
	errorText   = "Couldn't load release notes."
	closeHint   = "j/k scroll · W / esc / q to close"
)

type Model struct {
	Visible bool

	releases []release.Release
	loaded   bool
	failed   bool
	offset   int

	width  int
	height int
	theme  *theme.Theme
	md     *markdown.Renderer
}

func NewModel() Model {
	return Model{md: markdown.New()}
}

func (m *Model) SetTheme(th *theme.Theme) {
	m.theme = th
	if th != nil && m.md != nil {
		m.md.SetStyle(th.Markdown)
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.clampOffset()
}

func (m *Model) Open() {
	m.Visible = true
	m.offset = 0
}

func (m Model) Loaded() bool {
	return m.loaded
}

func (m *Model) SetReleases(rs []release.Release) {
	m.releases = rs
	m.loaded = true
	m.failed = false
	m.offset = 0
}

func (m *Model) SetError() {
	m.failed = true
	m.loaded = false
}

func (m Model) Failed() bool {
	return m.failed
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "W", "esc", "q":
		m.Visible = false
	case "j", "down":
		m.offset++
	case "k", "up":
		m.offset--
	case "pgdown", "ctrl+d", " ":
		m.offset += m.bodyHeight()
	case "pgup", "ctrl+u":
		m.offset -= m.bodyHeight()
	case "g", "home":
		m.offset = 0
	case "G", "end":
		m.offset = len(m.bodyLines())
	}
	m.clampOffset()
	return m, nil
}

func (m Model) ViewOver(bg string) string {
	if !m.Visible {
		return bg
	}
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return bg
	}
	box := m.renderBox(boxW, boxH)

	bgLines := strings.Split(bg, "\n")
	boxLines := strings.Split(box, "\n")
	startRow := max(0, (m.height-boxH)/2)
	startCol := max(0, (m.width-boxW)/2)

	for i, boxLine := range boxLines {
		row := startRow + i
		if row >= len(bgLines) {
			break
		}
		left := ansi.Cut(bgLines[row], 0, startCol)
		right := ansi.Cut(bgLines[row], startCol+boxW, m.width)
		bgLines[row] = left + boxLine + right
	}
	return strings.Join(bgLines, "\n")
}

func (m Model) boxSize() (int, int) {
	if m.width <= 0 || m.height <= 0 {
		return 0, 0
	}
	boxW := min(80, m.width-4)
	boxW = max(boxW, 30)
	boxH := len(m.bodyLinesAt(boxW-2)) + len(m.header(boxW-2)) + len(m.footer(boxW-2)) + 2
	boxH = min(boxH, m.height-4)
	boxH = max(boxH, 10)
	return boxW, boxH
}

func (m Model) bodyHeight() int {
	boxW, boxH := m.boxSize()
	innerW := boxW - 2
	return max(1, boxH-2-len(m.header(innerW))-len(m.footer(innerW)))
}

func (m Model) bodyLines() []string {
	boxW, _ := m.boxSize()
	return m.bodyLinesAt(boxW - 2)
}

func (m Model) bodyLinesAt(innerW int) []string {
	switch {
	case m.loaded && len(m.releases) > 0:
		md := release.Markdown(m.releases)
		if m.md == nil {
			return strings.Split(md, "\n")
		}
		return m.md.Render(md, innerW)
	case m.loaded:
		return []string{m.muted(" No release notes yet.")}
	case m.failed:
		return []string{m.muted(" " + errorText)}
	default:
		return []string{m.muted(" " + loadingText)}
	}
}

func (m *Model) clampOffset() {
	maxOff := max(0, len(m.bodyLines())-m.bodyHeight())
	m.offset = min(max(0, m.offset), maxOff)
}

func (m Model) header(innerW int) []string {
	if m.theme == nil {
		return []string{centre("What's new in pho", innerW), strings.Repeat("─", innerW)}
	}
	title := lipgloss.PlaceHorizontal(innerW, lipgloss.Center, m.theme.BoxTitle.Render("What's new in pho"))
	return []string{title, m.theme.BoxDiv.Render(strings.Repeat("─", innerW))}
}

func (m Model) footer(innerW int) []string {
	link := "Full notes: " + strings.TrimPrefix(release.ReleasesURL, "https://")
	lines := []string{""}
	for _, l := range []string{link, closeHint} {
		if m.theme != nil {
			l = m.theme.BoxFooter.Render(l)
		}
		lines = append(lines, lipgloss.PlaceHorizontal(innerW, lipgloss.Center, l))
	}
	return lines
}

func (m Model) muted(s string) string {
	if m.theme == nil {
		return s
	}
	return m.theme.BoxFooter.Render(s)
}

func (m Model) renderBox(boxW, boxH int) string {
	innerW := boxW - 2
	innerH := boxH - 2
	header := m.header(innerW)
	footer := m.footer(innerW)
	bodyH := max(0, innerH-len(header)-len(footer))

	body := m.bodyLinesAt(innerW)
	more := m.offset+bodyH < len(body)
	end := min(len(body), m.offset+bodyH)
	visible := append([]string(nil), body[min(m.offset, end):end]...)
	for len(visible) < bodyH {
		visible = append(visible, "")
	}
	if more && bodyH > 0 {
		arrow := "↓"
		if m.theme != nil {
			arrow = m.theme.BoxFooter.Render(arrow)
		}
		last := ansi.Truncate(visible[bodyH-1], innerW-2, "")
		pad := max(0, innerW-1-lipgloss.Width(last))
		visible[bodyH-1] = last + strings.Repeat(" ", pad) + arrow
	}

	content := make([]string, 0, innerH)
	content = append(content, header...)
	content = append(content, visible...)
	content = append(content, footer...)
	for i, l := range content {
		content[i] = ansi.Truncate(l, innerW, "…")
	}

	if m.theme != nil {
		return m.theme.BoxBorder.Width(innerW).Height(innerH).Render(strings.Join(content, "\n"))
	}
	lines := make([]string, 0, boxH)
	lines = append(lines, "┌"+strings.Repeat("─", innerW)+"┐")
	for _, l := range content {
		lines = append(lines, "│"+l+strings.Repeat(" ", max(0, innerW-lipgloss.Width(l)))+"│")
	}
	lines = append(lines, "└"+strings.Repeat("─", innerW)+"┘")
	return strings.Join(lines, "\n")
}

func centre(s string, w int) string {
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, s)
}
