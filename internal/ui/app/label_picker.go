package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
)

const labelPickHint = "Tab: Toggle | Enter: Apply | Esc: Cancel"

// labelPickTarget is the PR a label picker was opened for. Label lists that
// arrive for any other PR, or for an earlier opening, are dropped.
type labelPickTarget struct {
	repo      domain.Repository
	number    int
	requestID uint64
}

func (t labelPickTarget) matches(host, repo string, number int, requestID uint64) bool {
	return requestID == t.requestID && number == t.number &&
		strings.EqualFold(host, t.repo.Host) && strings.EqualFold(repo, t.repo.FullName)
}

// labelPicking reports whether the label picker is open.
func (m *Model) labelPicking() bool {
	return m.state.Search.OverlayOpen && m.palette.IsLabelPick()
}

// openLabelPicker opens the palette as a chooser of the PR's labels and
// starts loading the repository's. L reaches here as a message one step
// after the key, so it stays shut if another overlay opened in between.
func (m *Model) openLabelPicker() tea.Cmd {
	if m.state.Search.OverlayOpen || m.keymapOverlay.Visible ||
		(m.createPR != nil && m.createPR.Active()) {
		return nil
	}
	d := m.prDetail
	if m.currentView() != domain.PrimaryViewPRDetail || d == nil || !d.CanEditLabels() {
		return nil
	}
	current, total := d.Labels()

	m.labelPickSeq++
	m.labelPick = labelPickTarget{repo: d.Repo, number: d.Summary.Number, requestID: m.labelPickSeq}
	m.previousFocus = m.focus
	m.state.Search.OverlayOpen = true
	m.focus = domain.FocusCmdPalette
	m.palette = overlay.NewModel(m.deps.Search)
	m.palette.SetTheme(m.theme)
	m.palette.OpenLabelPick(current)
	m.palette, _ = m.palette.Update(tea.WindowSizeMsg{Width: m.layout.Width, Height: m.layout.Height})

	loads := []tea.Cmd{cmds.LoadRepoLabelsCmd(d.PRService, d.Repo, d.Summary.Number, m.labelPickSeq)}
	if total > len(current) {
		// The preview holds only the first page of the PR's labels; wait for
		// the rest before applying, so the baseline is right.
		m.palette.HoldLabelApply()
		loads = append(loads, cmds.LoadPRLabelsCmd(d.PRService, d.Repo, d.Summary.Number, m.labelPickSeq))
	}
	m.syncStatus()
	return tea.Batch(loads...)
}

// handleLabelPickKey runs the picker: Tab toggles, Enter applies (toggling
// the highlighted row first when nothing is pending), Esc cancels, and other
// keys edit the filter or move the selection.
func (m *Model) handleLabelPickKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case msg.Type == tea.KeyEsc:
		m.closeOverlay()
		return nil
	case msg.String() == "ctrl+p":
		return nil
	case msg.Type == tea.KeyEnter:
		return m.applyLabelPick()
	}
	m.palette, _ = m.palette.Update(msg)
	return nil
}

func (m *Model) applyLabelPick() tea.Cmd {
	if m.palette.ApplyHeld() {
		return nil // still loading the PR's full label list
	}
	if m.palette.PendingLabelCount() == 0 && !m.palette.ToggleMark(false) {
		return nil
	}
	change := m.palette.LabelChange()
	if change.Empty() {
		return nil // e.g. toggled a row back to where it started
	}
	target := m.palette.MarkedLabels()
	m.closeOverlay()

	d := m.prDetail
	if m.currentView() != domain.PrimaryViewPRDetail || d == nil ||
		!m.labelPick.matches(d.Repo.Host, d.Repo.FullName, d.Summary.Number, m.labelPick.requestID) {
		return nil // the PR on screen changed under the picker
	}
	cmd := d.BeginLabelApply(change, target)
	m.syncStatus()
	return cmd
}

// handleLabelLists feeds label lists to the picker that asked for them.
func (m *Model) handleLabelLists(msg tea.Msg) {
	switch msg := msg.(type) {
	case cmds.RepoLabelsLoaded:
		if !m.labelPicking() || !m.labelPick.matches(msg.Host, msg.Repo, msg.Number, msg.RequestID) {
			return
		}
		if msg.Err != nil {
			m.palette.SetLabelsErr(msg.Err.Error())
			return
		}
		m.palette.SetRepoLabels(msg.Labels.Labels, msg.Labels.Truncated)
	case cmds.PRLabelsLoaded:
		if !m.labelPicking() || !m.labelPick.matches(msg.Host, msg.Repo, msg.Number, msg.RequestID) {
			return
		}
		if msg.Err != nil {
			// Without the full list a change could be computed against the
			// wrong baseline, so apply stays held; Esc still closes.
			m.palette.SetPRLabelsErr(msg.Err.Error())
			return
		}
		m.palette.SetPRLabels(msg.Labels)
	}
}

// handleLabelsApplied passes a finished label change to the PR view and
// drops the dashboard's cached copy of that PR (not the whole repo, which
// would also drop unsent review drafts). It does so on failure too, since
// part of the change may have landed.
func (m *Model) handleLabelsApplied(msg cmds.LabelsApplied) tea.Cmd {
	if m.prDetail == nil {
		return nil
	}
	next, cmd := m.prDetail.Update(msg)
	m.prDetail = next
	m.syncStatus()
	if m.deps.Dashboard != nil {
		if repo, ok := m.findRepoByHostAndFullName(msg.Host, msg.Repo); ok {
			_ = m.deps.Dashboard.InvalidatePR(context.Background(), repo, msg.Number)
		}
	}
	return cmd
}
