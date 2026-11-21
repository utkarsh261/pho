package prdetail

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
)

// customKeyIn runs cmd, and any batch inside it, looking for a CustomKey.
// Commands that block (ticks) are given up on after a short wait.
func customKeyIn(cmd tea.Cmd) (CustomKey, bool) {
	if cmd == nil {
		return CustomKey{}, false
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(50 * time.Millisecond):
		return CustomKey{}, false
	}
	switch msg := msg.(type) {
	case CustomKey:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if k, ok := customKeyIn(c); ok {
				return k, true
			}
		}
	}
	return CustomKey{}, false
}

func pressRune(m *PRDetailModel, r rune) (*PRDetailModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

func TestCustomKeyFiresInNormalMode(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	_, cmd := pressRune(m, 'w')
	k, ok := customKeyIn(cmd)
	if !ok || k.Key != "w" {
		t.Fatalf("got %+v, %v; want CustomKey w", k, ok)
	}
}

// A key typed or answered in any input or prompt mode must never run a
// custom command.
func TestCustomKeyInertOutsideNormalMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(m *PRDetailModel)
	}{
		{"search", func(m *PRDetailModel) { m.searchActive = true }},
		{"visual", func(m *PRDetailModel) { m.visual.Active = true }},
		{"compose", func(m *PRDetailModel) { m.compose.Open(composeModeNew, commentEntry{}, 0) }},
		{"compose posting", func(m *PRDetailModel) {
			m.compose.Open(composeModeNew, commentEntry{}, 0)
			m.compose.status = composeStatusPosting
		}},
		// The box shows the error and still takes text.
		{"compose error", func(m *PRDetailModel) {
			m.compose.Open(composeModeNew, commentEntry{}, 0)
			m.compose.status = composeStatusError
			m.compose.errMsg = "boom"
		}},
		{"edit prompt", func(m *PRDetailModel) { m.editPrompt = "Edit: [t]itle or [b]ody?" }},
		{"editor prompt", func(m *PRDetailModel) { m.editorPrompt = "Check out branch? (y/n)" }},
		{"close confirm", func(m *PRDetailModel) { m.closeStep = closeStepConfirm; m.closeTarget = "CLOSE" }},
		{"merge select", func(m *PRDetailModel) { m.mergeStep = mergeStepSelectMethod }},
		{"discard confirm", func(m *PRDetailModel) { m.confirmDiscardAll = true }},
		{"checkout in flight", func(m *PRDetailModel) { m.checkoutInFlight = true }},
		{"load error", func(m *PRDetailModel) { m.Detail = nil; m.LoadErr = errors.New("boom") }},
		{"commit mode", func(m *PRDetailModel) { m.CommitMode = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newEditModel()
			tt.setup(m)
			_, cmd := pressRune(m, 'w')
			if k, ok := customKeyIn(cmd); ok {
				t.Fatalf("custom key %q fired in %s", k.Key, tt.name)
			}
		})
	}
}

func TestCustomKeyNotSentForSpecialKeys(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if _, ok := customKeyIn(cmd); ok {
		t.Fatal("special key reached the custom key path")
	}
}

// The CI log keeps its own keys but passes free ones on, as the keymap
// overlay shown there promises.
func TestCustomKeyFiresInCILog(t *testing.T) {
	t.Parallel()
	m := openLog(t, newCILogModel(&ciLogPRService{log: domain.JobLog{Lines: []string{"x"}}}))
	_, cmd := pressRune(m, 'w')
	if k, ok := customKeyIn(cmd); !ok || k.Key != "w" {
		t.Fatalf("got %+v, %v; want CustomKey w", k, ok)
	}
	if !m.ciLog.active {
		t.Error("custom key closed the CI log")
	}
	_, cmd = pressRune(m, 'j')
	if _, ok := customKeyIn(cmd); ok {
		t.Error("the log's own j reached the custom key path")
	}
}
