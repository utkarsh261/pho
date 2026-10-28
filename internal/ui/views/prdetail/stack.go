package prdetail

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// StackHop asks the root model to replace this PR with another in its stack.
type StackHop struct {
	Number int
	Notice string // shown on the PR hopped to
}

// OpenStackPicker asks the root model to open the stack picker.
type OpenStackPicker struct{}

// SetStack sets the stack this PR belongs to; nil when it isn't stacked.
func (m *PRDetailModel) SetStack(st *domain.Stack) {
	m.Stack = st
}

// SetNotice shows a one-key status message.
func (m *PRDetailModel) SetNotice(s string) {
	m.diffNotice = s
}

// PanelFocus is the panel that has focus: files, CI or content.
func (m *PRDetailModel) PanelFocus() PRDetailFocus {
	return m.leftPanel.Focus
}

// StartOn shows tab, with focus on the same panel, on a freshly opened PR,
// so hopping along a stack keeps your place.
func (m *PRDetailModel) StartOn(tab ContentTab, focus PRDetailFocus) tea.Cmd {
	if m.CommitMode {
		return nil
	}
	if focus != FocusCI {
		m.leftPanel.Focus = focus
	}
	if tab == m.activeTab {
		return nil
	}
	needLoad := tab == TabCommits && !m.commitsLoaded && !m.commitsLoading && m.PRService != nil
	m.switchTab(tab)
	if needLoad {
		return cmds.LoadPRCommitsCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.HeadRefOID, false)
	}
	return nil
}

// stackHop moves one PR toward the trunk, or away from it when up is set.
func (m *PRDetailModel) stackHop(up bool) tea.Cmd {
	if m.Stack == nil {
		m.diffNotice = "Not part of a stack"
		return nil
	}
	mem, ok := m.Stack.Member(m.Summary.Number)
	if !ok {
		return nil
	}
	hop := StackHop{}
	switch {
	case up && len(mem.Children) == 0:
		m.diffNotice = "Top of stack"
		return nil
	case up:
		hop.Number = mem.Children[0]
		if n := len(mem.Children); n > 1 {
			hop.Notice = fmt.Sprintf("1 of %d PRs on #%d · S: Choose", n, mem.Number)
		}
	case mem.Parent == 0:
		m.diffNotice = "Bottom of stack · based on " + m.Stack.Trunk
		return nil
	default:
		hop.Number = mem.Parent
	}
	return func() tea.Msg { return hop }
}

func (m *PRDetailModel) stackPickKey() tea.Cmd {
	if m.Stack == nil {
		m.diffNotice = "Not part of a stack"
		return nil
	}
	return func() tea.Msg { return OpenStackPicker{} }
}

// stackPath is the line through the stack this PR is on: its bases down to
// the bottom, then the newest PR on top of each up to the top.
func (m *PRDetailModel) stackPath() (path []int, current int) {
	st := m.Stack
	mem, ok := st.Member(m.Summary.Number)
	if !ok {
		return nil, 0
	}
	for p := mem.Parent; p != 0; {
		path = append([]int{p}, path...)
		pm, ok := st.Member(p)
		if !ok {
			break
		}
		p = pm.Parent
	}
	current = len(path)
	path = append(path, mem.Number)
	for c := mem.Children; len(c) > 0 && len(path) <= len(st.Members); {
		path = append(path, c[0])
		cm, ok := st.Member(c[0])
		if !ok {
			break
		}
		c = cm.Children
	}
	return path, current
}

// headerTopBorder opens the header box with the stack's breadcrumb set into
// the rule: "╭─ main › #126 › #127 › #128  { } ───╮". Like the labels in the
// bottom rule, it keeps the header the same height.
func (m *PRDetailModel) headerTopBorder(th *theme.Theme, innerW int) string {
	border := lipgloss.NewStyle().Foreground(th.Border)
	b := lipgloss.RoundedBorder()
	plain := border.Render(b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight)
	if m.Stack == nil || m.CommitMode {
		return plain
	}
	crumbs := m.stackCrumbs(th, innerW-8) // "─ " before, "  { } " and "─" after
	if crumbs == "" {
		return plain
	}
	crumbs += "  " + th.FaintTxt.Render("{ }")
	fill := max(innerW-lipgloss.Width(crumbs)-3, 0)
	return border.Render(b.TopLeft+b.Top) + " " + crumbs + " " + border.Render(strings.Repeat(b.Top, fill)+b.TopRight)
}

// stackCrumbs renders the stack path within width, collapsing the middle
// around the current PR when it doesn't fit.
func (m *PRDetailModel) stackCrumbs(th *theme.Theme, width int) string {
	path, cur := m.stackPath()
	if len(path) == 0 {
		return ""
	}
	sep := th.FaintTxt.Render(" › ")
	render := func(keep func(i int) bool) string {
		parts := []string{th.FaintTxt.Render(m.Stack.Trunk)}
		gap := false
		for i, n := range path {
			if !keep(i) {
				if !gap {
					parts = append(parts, th.FaintTxt.Render("…"))
				}
				gap = true
				continue
			}
			gap = false
			s := th.MutedTxt.Render(fmt.Sprintf("#%d", n))
			if i == cur {
				s = th.PrimaryTxt.Bold(true).Render(fmt.Sprintf("#%d", n))
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, sep)
	}
	last := len(path) - 1
	for _, keep := range []func(int) bool{
		func(int) bool { return true },
		func(i int) bool { return i == 0 || i == last || (i >= cur-1 && i <= cur+1) },
		func(i int) bool { return i == cur },
	} {
		if s := render(keep); lipgloss.Width(s) <= width {
			return s
		}
	}
	mem, _ := m.Stack.Member(m.Summary.Number)
	if s := th.MutedTxt.Render(fmt.Sprintf("stack %d/%d", mem.Layer, m.Stack.Height())); lipgloss.Width(s) <= width {
		return s
	}
	return ""
}
