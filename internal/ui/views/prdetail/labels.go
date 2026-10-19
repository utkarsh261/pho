package prdetail

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

// OpenLabelPicker asks the root model to open the label picker.
type OpenLabelPicker struct{}

// labelApplyState tracks a label change from the picker: the request in
// flight, and the optimistic labels shown until a reload confirms them.
type labelApplyState struct {
	executing bool
	reqID     uint64

	// active keeps target on screen over reloads that may predate the change.
	active      bool
	target      []domain.Label
	targetCount int
	prev        []domain.Label
	prevCount   int
	// confirmBy is the detail request issued after the change succeeded; a
	// reload at least that new shows the change, so the override can go.
	confirmBy uint64
}

// labelKey handles L: open the label picker, or say why not.
func (m *PRDetailModel) labelKey() tea.Cmd {
	if m.CommitMode {
		return nil
	}
	// While a label change runs, the status bar already says so.
	switch {
	case m.Detail == nil:
		m.diffNotice = "PR still loading"
	case m.PRService == nil:
		return nil
	case m.isAnyActionInProgress():
		m.diffNotice = "Finish the current action first"
	default:
		return func() tea.Msg { return OpenLabelPicker{} }
	}
	return nil
}

// CanEditLabels reports whether a label change can start now: the PR is
// loaded and no other action (including a label change) is running.
func (m *PRDetailModel) CanEditLabels() bool {
	return !m.CommitMode && m.Detail != nil && m.PRService != nil && !m.isAnyActionInProgress()
}

// Labels returns the PR's labels as shown and its total label count (the
// list may hold only the first page).
func (m *PRDetailModel) Labels() ([]domain.Label, int) {
	if m.Detail == nil {
		return nil, 0
	}
	return m.Detail.Labels, max(m.Detail.LabelCount, len(m.Detail.Labels))
}

// BeginLabelApply shows target as the PR's labels straight away and sends
// change. target is the label list after the change as far as the picker
// knows it; complete says the picker knew every label on the PR, so target is
// the whole list. Otherwise the count is a best guess until the reload.
func (m *PRDetailModel) BeginLabelApply(change domain.LabelChange, target []domain.Label, complete bool) tea.Cmd {
	if change.Empty() || !m.CanEditLabels() {
		return nil
	}
	prev, prevCount := m.Labels()
	count := len(target)
	if !complete {
		count = max(prevCount+len(change.Add)-len(change.Remove), len(target))
	}

	m.labelApply.reqID++
	m.labelApply = labelApplyState{
		executing:   true,
		reqID:       m.labelApply.reqID,
		active:      true,
		target:      append([]domain.Label(nil), target...),
		targetCount: count,
		prev:        append([]domain.Label(nil), prev...),
		prevCount:   prevCount,
	}
	m.labelErr = ""
	m.showLabels(target, count)
	return cmds.ApplyLabelsCmd(m.PRService, m.Repo, m.Summary.Number, change, m.labelApply.reqID)
}

func (m *PRDetailModel) showLabels(labels []domain.Label, count int) {
	if m.Detail == nil {
		return
	}
	m.Detail.Labels = append([]domain.Label(nil), labels...)
	m.Detail.LabelCount = count
}

// handleLabelsApplied settles a label change: on success it reloads the PR
// and keeps the optimistic labels until that reload lands; on failure it puts
// the old labels back and reloads anyway, since part of the change may have
// gone through.
func (m *PRDetailModel) handleLabelsApplied(msg cmds.LabelsApplied) tea.Cmd {
	if !m.matchesPR(msg.Host, msg.Repo, msg.Number) || !m.labelApply.executing || msg.RequestID != m.labelApply.reqID {
		return nil
	}
	m.labelApply.executing = false
	if msg.Err != nil {
		m.labelApply.active = false
		m.showLabels(m.labelApply.prev, m.labelApply.prevCount)
		m.labelErr = labelErrText(msg.Err)
		return m.loadPRDetailCmd(true)
	}
	cmd := m.loadPRDetailCmd(true)
	m.labelApply.confirmBy = m.detailRequestID
	return cmd
}

// reconcileLabels runs on every detail reload: while a change is pending it
// keeps the optimistic labels over a payload that may predate the change.
func (m *PRDetailModel) reconcileLabels(requestID uint64) {
	a := &m.labelApply
	if !a.active {
		return
	}
	m.showLabels(a.target, a.targetCount)
	// GitHub's GraphQL reads can trail a REST write, so even the reload
	// issued after the change may predate it: keep the change on screen for
	// that one, as the resolve toggle does, and let later reloads speak.
	if !a.executing && a.confirmBy != 0 && requestID >= a.confirmBy {
		a.active = false
	}
}

func labelErrText(err error) string {
	if errors.Is(err, domain.ErrLabelPermission) {
		return upperFirst(err.Error())
	}
	return "Failed to update labels: " + err.Error()
}

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + strings.TrimPrefix(s, string(r))
}
