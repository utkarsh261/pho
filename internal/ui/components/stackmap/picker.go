package stackmap

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const pickerMaxWidth = 64

// Picker is a box over the screen listing a stack's PRs to jump between.
type Picker struct {
	Visible bool

	stack   *domain.Stack
	current int
	cursor  int
	theme   *theme.Theme
	width   int
	height  int
}

// Open shows st with the selection on the PR being looked at.
func (p *Picker) Open(st *domain.Stack, current int) {
	p.Visible = true
	p.stack = st
	p.current = current
	p.cursor = max(st.IndexOf(current), 0)
}

// Close hides the picker.
func (p *Picker) Close() {
	p.Visible = false
	p.stack = nil
}

func (p *Picker) SetTheme(th *theme.Theme) { p.theme = th }

func (p *Picker) SetSize(w, h int) { p.width, p.height = w, h }

// Move shifts the selection by delta, stopping at either end.
func (p *Picker) Move(delta int) {
	if p.stack == nil {
		return
	}
	p.cursor = min(max(p.cursor+delta, 0), len(p.stack.Members)-1)
}

// Stack is the stack on show, nil when closed.
func (p Picker) Stack() *domain.Stack {
	return p.stack
}

// Selected is the PR under the selection.
func (p Picker) Selected() (domain.StackMember, bool) {
	if p.stack == nil || p.cursor < 0 || p.cursor >= len(p.stack.Members) {
		return domain.StackMember{}, false
	}
	return p.stack.Members[p.cursor], true
}

// ViewOver draws the picker centred over bg.
func (p Picker) ViewOver(bg string) string {
	if !p.Visible || p.stack == nil || p.width < 20 || p.height < 6 {
		return bg
	}
	th := p.theme
	if th == nil {
		th = theme.Default()
	}
	innerW := min(pickerMaxWidth, p.width-4)
	rows := Rows(th, p.stack, Options{
		Width: innerW, Current: p.current, Cursor: p.cursor, Indent: true,
		Window: max((p.height-6)/2-1, 1),
	})

	border := lipgloss.NewStyle().Foreground(th.Primary)
	b := lipgloss.RoundedBorder()
	title := th.Header.Render(Header(p.stack))
	if p.stack.Native {
		title += th.FaintTxt.Render(" · GitHub")
	}
	title = truncate(title, innerW-3)
	top := border.Render(b.TopLeft+b.Top) + " " + title + " " +
		border.Render(strings.Repeat(b.Top, max(innerW-lipgloss.Width(title)-3, 0))+b.TopRight)
	box := []string{top}
	for _, r := range rows {
		pad := max(innerW-lipgloss.Width(r), 0)
		box = append(box, border.Render(b.Left)+r+strings.Repeat(" ", pad)+border.Render(b.Right))
	}
	box = append(box, border.Render(b.BottomLeft+strings.Repeat(b.Bottom, innerW)+b.BottomRight))

	boxW := innerW + 2
	lines := strings.Split(bg, "\n")
	startRow := max((len(lines)-len(box))/2, 0)
	startCol := max((p.width-boxW)/2, 0)
	for i, line := range box {
		r := startRow + i
		if r >= len(lines) {
			break
		}
		left := ansi.Cut(lines[r], 0, startCol)
		left += strings.Repeat(" ", max(startCol-lipgloss.Width(left), 0))
		lines[r] = left + line + ansi.Cut(lines[r], startCol+boxW, p.width)
	}
	return strings.Join(lines, "\n")
}
