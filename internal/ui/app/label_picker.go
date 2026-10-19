package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
)

const labelPickHint = "Enter: Toggle | Tab: Toggle & next | Esc: Save & close"

// labelPickTarget is the PR a label picker was opened for. Label lists that
// arrive for any other PR, or for an earlier opening, are dropped.
type labelPickTarget struct {
	repo      domain.Repository
	number    int
	requestID uint64
}

func (t labelPickTarget) matches(host, repo string, number int, requestID uint64) bool {
	return requestID == t.requestID && number == t.number &&
		strings.EqualFold(host, t.repo.Host) && strings.EqualFold(repo, repoName(t.repo))
}

// repoName is a repository's "owner/name", as label messages carry it.
func repoName(r domain.Repository) string {
	if r.FullName != "" {
		return r.FullName
	}
	if r.Owner != "" && r.Name != "" {
		return r.Owner + "/" + r.Name
	}
	return r.Name
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
		// The preview holds only the first page of the PR's labels; load the
		// rest so their rows show as applied.
		m.palette.SetPRLabelsLoading()
		loads = append(loads, cmds.LoadPRLabelsCmd(d.PRService, d.Repo, d.Summary.Number, m.labelPickSeq))
	}
	m.syncStatus()
	return tea.Batch(loads...)
}

// handleLabelPickKey runs the picker, which stays open for any number of
// changes, like GitHub's label menu: Enter toggles the highlighted label, Tab
// toggles it and moves down, Esc saves everything and closes, and other keys
// edit the filter or move the selection.
func (m *Model) handleLabelPickKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case msg.Type == tea.KeyEsc:
		return m.saveLabelPick()
	case msg.String() == "ctrl+p":
		return nil
	case msg.Type == tea.KeyEnter:
		m.palette.ToggleMark(false)
		return nil
	}
	m.palette, _ = m.palette.Update(msg)
	return nil
}

// saveLabelPick closes the picker and applies what changed. Only toggled
// labels are sent, so this is safe even before a PR's full label list (past
// the first page) has loaded: re-adding a label the PR has is a no-op.
func (m *Model) saveLabelPick() tea.Cmd {
	change := m.palette.LabelChange()
	target := m.palette.MarkedLabels()
	complete := m.palette.BaselineComplete()
	m.closeOverlay()
	if change.Empty() {
		return nil // nothing toggled, or toggled back to where it started
	}

	d := m.prDetail
	if m.currentView() != domain.PrimaryViewPRDetail || d == nil ||
		!m.labelPick.matches(d.Repo.Host, repoName(d.Repo), d.Summary.Number, m.labelPick.requestID) {
		return nil // the PR on screen changed under the picker
	}
	cmd := d.BeginLabelApply(change, target, complete)
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
			// Saving stays safe (only toggled labels are sent); the border
			// says the rows past the first page may be unmarked.
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
