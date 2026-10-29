package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

const stackPickHint = "j/k: Move | Enter: Open | Esc: Close"

// openStackPicker shows the stack of the PR on screen: the open PR in
// detail, or the selected one on the dashboard.
func (m *Model) openStackPicker() tea.Cmd {
	if m.state.Search.OverlayOpen || m.keymapOverlay.Visible || m.whatsNew.Visible ||
		(m.createPR != nil && m.createPR.Active()) {
		return nil
	}
	var pr domain.PullRequestSummary
	switch m.currentView() {
	case domain.PrimaryViewPRDetail:
		if m.prDetail == nil {
			return nil
		}
		pr = m.prDetail.Summary
	case domain.PrimaryViewDashboard:
		sel, ok := m.currentSelectedPR()
		if !ok {
			return nil
		}
		pr = sel
	default:
		return nil
	}
	st := m.stacks.Lookup(pr.Repo, pr.Number)
	if st == nil {
		return nil
	}
	m.stackPicker.SetTheme(m.theme)
	m.stackPicker.SetSize(m.layout.Width, m.layout.Height)
	m.stackPicker.Open(st, pr.Number)
	m.syncStatus()
	return nil
}

func (m *Model) handleStackPickKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q", "S":
		m.stackPicker.Close()
	case "j", "down":
		m.stackPicker.Move(1)
	case "k", "up":
		m.stackPicker.Move(-1)
	case "enter":
		mem, ok := m.stackPicker.Selected()
		st := m.stackPicker.Stack()
		m.stackPicker.Close()
		m.syncStatus()
		if !ok {
			return nil
		}
		if m.currentView() == domain.PrimaryViewPRDetail {
			return m.handleStackHop(prdetail.StackHop{Number: mem.Number})
		}
		return m.openStackMember(mem, st)
	}
	m.syncStatus()
	return nil
}

// handleStackHop swaps the open PR for another in its stack, on the same
// tab, so a stack reads like consecutive pages. The dashboard cursor
// follows, so Esc lands on the last PR read.
func (m *Model) handleStackHop(msg prdetail.StackHop) tea.Cmd {
	old := m.prDetail
	if old == nil || old.Stack == nil || old.Summary.Number == msg.Number {
		return nil
	}
	mem, ok := old.Stack.Member(msg.Number)
	if !ok {
		return nil
	}
	tab, focus := old.ActiveTab(), old.PanelFocus()
	if m.currentView() == domain.PrimaryViewPRDetail {
		m.popView() // replace the detail view rather than stacking another
	}
	open := m.openStackMember(mem, old.Stack)
	start := m.prDetail.StartOn(tab, focus)
	if msg.Notice != "" {
		m.prDetail.SetNotice(msg.Notice)
		m.syncStatus()
	}
	return tea.Batch(open, start)
}

// openStackMember opens a stack member in PR detail and moves the dashboard
// cursor onto it when it is listed. The preview, hidden behind the detail
// view, catches up when the dashboard is back on screen.
func (m *Model) openStackMember(mem domain.StackMember, st *domain.Stack) tea.Cmd {
	summary := domain.PullRequestSummary{Number: mem.Number, Title: mem.Title, State: mem.State, IsDraft: mem.IsDraft}
	if mem.Summary != nil {
		summary = *mem.Summary
	}
	if summary.Repo == "" && st != nil {
		summary.Repo = st.Repo
	}
	if m.prList.SelectNumber(mem.Number) {
		m.state.Dashboard.SelectedIndex = m.prList.Cursor
		m.previewBehind = true
	}
	return m.openPRDetailForJump(summary)
}
