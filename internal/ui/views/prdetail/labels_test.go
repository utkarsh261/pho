package prdetail

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

// runCmd runs cmd and any batch inside it, returning the messages that
// aren't themselves batches.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func hasMsg[T any](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

func keyL() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")} }

func labelModel() *PRDetailModel {
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.Detail.Labels = []domain.Label{{Name: "bug", Color: "d73a4a"}, {Name: "area/diff"}}
	m.Detail.LabelCount = 2
	return m
}

func names(ls []domain.Label) string {
	s := ""
	for _, l := range ls {
		s += l.Name + ","
	}
	return s
}

func TestLabelKeyOpensPicker(t *testing.T) {
	t.Parallel()
	m := labelModel()
	_, cmd := m.Update(keyL())
	if !hasMsg[OpenLabelPicker](runCmd(cmd)) {
		t.Fatal("L should ask for the label picker")
	}
}

func TestLabelKeyGuards(t *testing.T) {
	t.Parallel()

	commit := labelModel()
	commit.CommitMode = true
	if _, cmd := commit.Update(keyL()); hasMsg[OpenLabelPicker](runCmd(cmd)) {
		t.Error("L must do nothing in commit view")
	}

	loading := labelModel()
	loading.Detail = nil
	if _, cmd := loading.Update(keyL()); hasMsg[OpenLabelPicker](runCmd(cmd)) || loading.StatusHint() != "PR still loading" {
		t.Errorf("loading PR: hint %q", loading.StatusHint())
	}

	busy := labelModel()
	busy.closeStep = closeStepConfirm
	if _, cmd := busy.Update(keyL()); hasMsg[OpenLabelPicker](runCmd(cmd)) {
		t.Error("L must not open over the close prompt")
	}

	searching := labelModel()
	searching.searchActive = true
	if _, cmd := searching.Update(keyL()); hasMsg[OpenLabelPicker](runCmd(cmd)) {
		t.Error("L types into search while searching")
	}
}

func TestBeginLabelApplyIsOptimistic(t *testing.T) {
	t.Parallel()
	m := labelModel()
	change := domain.LabelChange{
		Create: []domain.Label{{Name: "needs-qa", Color: "1d76db"}},
		Add:    []string{"needs-qa"},
		Remove: []string{"area/diff"},
	}
	target := []domain.Label{{Name: "bug"}, {Name: "needs-qa", Color: "1d76db"}}
	msgs := runCmd(m.BeginLabelApply(change, target))

	if names(m.Detail.Labels) != "bug,needs-qa," || m.Detail.LabelCount != 2 {
		t.Errorf("labels now %q (%d)", names(m.Detail.Labels), m.Detail.LabelCount)
	}
	if m.StatusHint() != "Updating labels..." {
		t.Errorf("hint = %q", m.StatusHint())
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs = %v", msgs)
	}
	if _, ok := msgs[0].(cmds.LabelsApplied); !ok {
		t.Fatalf("msg = %T", msgs[0])
	}
	if m.BeginLabelApply(change, target) != nil {
		t.Error("a second apply while one is running must be refused")
	}
	if _, cmd := m.Update(keyL()); hasMsg[OpenLabelPicker](runCmd(cmd)) || m.StatusHint() != "Updating labels..." {
		t.Errorf("L while updating: hint %q", m.StatusHint())
	}
}

func TestLabelsAppliedSuccessKeepsLabelsUntilReloadConfirms(t *testing.T) {
	t.Parallel()
	m := labelModel()
	m.BeginLabelApply(domain.LabelChange{Add: []string{"new"}}, []domain.Label{{Name: "bug"}, {Name: "area/diff"}, {Name: "new"}})

	// A reload issued before the change lands carries the old labels.
	stale := *m.Detail
	stale.Labels = []domain.Label{{Name: "bug"}, {Name: "area/diff"}}
	stale.LabelCount = 2
	staleID := m.detailRequestID

	_, cmd := m.Update(cmds.LabelsApplied{Host: "github.com", Repo: "org/repo", Number: 42, RequestID: m.labelApply.reqID})
	if !hasMsg[cmds.PRDetailLoaded](runCmd(cmd)) {
		t.Error("success should reload the PR")
	}
	m.Update(cmds.PRDetailLoaded{Host: "github.com", Repo: "org/repo", Number: 42, RequestID: staleID, Detail: stale})
	if names(m.Detail.Labels) != "bug,area/diff,new," {
		t.Errorf("stale reload overwrote the change: %q", names(m.Detail.Labels))
	}

	// The reload after the change is the truth, whatever it says.
	fresh := stale
	fresh.Labels = []domain.Label{{Name: "bug"}, {Name: "new"}}
	m.Update(cmds.PRDetailLoaded{Host: "github.com", Repo: "org/repo", Number: 42, RequestID: m.detailRequestID, Detail: fresh})
	if names(m.Detail.Labels) != "bug,new," || m.labelApply.active {
		t.Errorf("confirming reload ignored: %q active=%v", names(m.Detail.Labels), m.labelApply.active)
	}
	if m.StatusHint() == "Updating labels..." {
		t.Error("hint should clear after success")
	}
}

func TestLabelsAppliedFailureRestores(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		hint string
	}{
		{fmtErr("no permission to edit labels in org/repo", domain.ErrLabelPermission), "No permission to edit labels in org/repo"},
		{errors.New("name is too long"), "Failed to update labels: name is too long"},
	} {
		m := labelModel()
		m.BeginLabelApply(domain.LabelChange{Remove: []string{"bug"}}, []domain.Label{{Name: "area/diff"}})
		_, cmd := m.Update(cmds.LabelsApplied{Host: "github.com", Repo: "org/repo", Number: 42, RequestID: m.labelApply.reqID, Err: tc.err})
		if names(m.Detail.Labels) != "bug,area/diff," || m.Detail.LabelCount != 2 {
			t.Errorf("labels not restored: %q", names(m.Detail.Labels))
		}
		if m.StatusHint() != tc.hint {
			t.Errorf("hint = %q, want %q", m.StatusHint(), tc.hint)
		}
		if !hasMsg[cmds.PRDetailLoaded](runCmd(cmd)) {
			t.Error("failure should still reload (part may have landed)")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.StatusHint() == tc.hint {
			t.Error("error should clear on the next key")
		}
	}
}

func TestLabelsAppliedIgnoresOtherRequests(t *testing.T) {
	t.Parallel()
	m := labelModel()
	m.BeginLabelApply(domain.LabelChange{Add: []string{"x"}}, []domain.Label{{Name: "bug"}, {Name: "area/diff"}, {Name: "x"}})
	id := m.labelApply.reqID
	for _, msg := range []cmds.LabelsApplied{
		{Host: "github.com", Repo: "org/repo", Number: 43, RequestID: id, Err: errors.New("x")},
		{Host: "github.com", Repo: "other/repo", Number: 42, RequestID: id, Err: errors.New("x")},
		{Host: "github.com", Repo: "org/repo", Number: 42, RequestID: id + 1, Err: errors.New("x")},
	} {
		m.Update(msg)
		if !m.labelApply.executing || names(m.Detail.Labels) != "bug,area/diff,x," {
			t.Errorf("%+v should be ignored", msg)
		}
	}
}

type wrappedErr struct {
	msg   string
	inner error
}

func (e wrappedErr) Error() string { return e.msg }
func (e wrappedErr) Unwrap() error { return e.inner }

func fmtErr(msg string, inner error) error { return wrappedErr{msg, inner} }
