package app

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

var (
	keyL   = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}}
	keyTab = tea.KeyMsg{Type: tea.KeyTab}
)

// labelPickerModel opens PR #123 (labelled bug) with a repo that has bug,
// build and docs.
func labelPickerModel(t *testing.T) (*Model, *stubPRService) {
	t.Helper()
	detail, diff := ansiPRDetailFixture()
	detail.Labels = []domain.Label{{Name: "bug", Color: "d73a4a"}}
	detail.LabelCount = 1
	prSvc := &stubPRService{detailResult: detail, diffResult: diff, repoLabels: domain.RepoLabelList{Labels: []domain.RepoLabel{
		{Label: domain.Label{Name: "bug", Color: "d73a4a"}, Description: "Broken"},
		{Label: domain.Label{Name: "build", Color: "0e8a16"}},
		{Label: domain.Label{Name: "docs", Color: "1d76db"}},
	}}}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{testutil.Repo("acme/alpha")}, prSvc)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	return m, prSvc
}

func dashboardStub(m *Model) *stubDashboardService {
	return m.deps.Dashboard.(*stubDashboardService)
}

func TestLabelPickerOpensOverDetail(t *testing.T) {
	m, _ := labelPickerModel(t)
	drainCmds(m, key(m, keyL))
	if !m.labelPicking() {
		t.Fatal("L did not open the label picker")
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Labels", "[x] ● bug", "[ ] ● build", "Broken", "Tab", "Enter", "Esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "#123") {
		t.Error("picker should draw over the PR detail, not the dashboard")
	}
}

func TestLabelPickerEnterTogglesHighlightedAndApplies(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	drainCmds(m, key(m, keyL))
	typeKeys(m, "docs")
	drainCmds(m, key(m, keyEnter))

	if m.labelPicking() {
		t.Error("Enter should close the picker")
	}
	if len(prSvc.applied) != 1 || strings.Join(prSvc.applied[0].Add, ",") != "docs" || len(prSvc.applied[0].Remove) != 0 {
		t.Fatalf("applied = %+v", prSvc.applied)
	}
	if got := dashboardStub(m).invalidatePRCalls; len(got) != 1 || got[0] != "acme/alpha#123" {
		t.Errorf("InvalidatePR calls = %v", got)
	}
	if len(dashboardStub(m).invalidateRepoCalls) != 0 {
		t.Error("a label change must not invalidate the whole repo (it would drop review drafts)")
	}
}

func TestLabelPickerTabMarksThenEnterAppliesAll(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	drainCmds(m, key(m, keyL))
	key(m, keyTab) // unmark bug, move to build
	key(m, keyTab) // mark build
	if !strings.Contains(ansi.Strip(m.View()), "2 pending") {
		t.Errorf("pending note missing:\n%s", ansi.Strip(m.View()))
	}
	drainCmds(m, key(m, keyEnter))
	if len(prSvc.applied) != 1 {
		t.Fatalf("applied = %+v", prSvc.applied)
	}
	c := prSvc.applied[0]
	if strings.Join(c.Add, ",") != "build" || strings.Join(c.Remove, ",") != "bug" {
		t.Errorf("change = %+v", c)
	}
}

func TestLabelPickerCreatesLabel(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	drainCmds(m, key(m, keyL))
	typeKeys(m, "needs-qa")
	if !strings.Contains(ansi.Strip(m.View()), "Create label") {
		t.Fatalf("create row missing:\n%s", ansi.Strip(m.View()))
	}
	drainCmds(m, key(m, keyEnter))
	if len(prSvc.applied) != 1 {
		t.Fatalf("applied = %+v", prSvc.applied)
	}
	c := prSvc.applied[0]
	if len(c.Create) != 1 || c.Create[0].Name != "needs-qa" || strings.Join(c.Add, ",") != "needs-qa" {
		t.Errorf("change = %+v", c)
	}
}

func TestLabelPickerEscAppliesNothing(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	drainCmds(m, key(m, keyL))
	key(m, keyTab)
	key(m, keyEsc)
	if m.labelPicking() || len(prSvc.applied) != 0 {
		t.Errorf("picking %v, applied %+v", m.labelPicking(), prSvc.applied)
	}
}

func TestLabelPickerFailureShowsError(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	prSvc.applyErr = permissionErr{}
	drainCmds(m, key(m, keyL))
	typeKeys(m, "docs")
	drainCmds(m, key(m, keyEnter))
	if got := m.status.HintOverride; got != "No permission to edit labels in acme/alpha" {
		t.Errorf("hint = %q", got)
	}
	if len(dashboardStub(m).invalidatePRCalls) != 0 {
		t.Error("nothing to invalidate after a failure")
	}
}

type permissionErr struct{}

func (permissionErr) Error() string { return "no permission to edit labels in acme/alpha" }
func (permissionErr) Unwrap() error { return domain.ErrLabelPermission }

func TestLabelPickerDropsStaleLists(t *testing.T) {
	m, _ := labelPickerModel(t)
	cmd := key(m, keyL)
	// Close and reopen before the first opening's lists arrive.
	drainCmds(m, cmd)
	key(m, keyEsc)
	drainCmds(m, key(m, keyL))
	before := len(m.palette.MarkedLabels())

	m.Update(cmds.RepoLabelsLoaded{Host: "github.com", Repo: "acme/alpha", Number: 123, RequestID: m.labelPick.requestID - 1,
		Labels: domain.RepoLabelList{Labels: []domain.RepoLabel{{Label: domain.Label{Name: "stale"}}}}})
	m.Update(cmds.RepoLabelsLoaded{Host: "github.com", Repo: "acme/alpha", Number: 999, RequestID: m.labelPick.requestID,
		Labels: domain.RepoLabelList{Labels: []domain.RepoLabel{{Label: domain.Label{Name: "other-pr"}}}}})
	view := ansi.Strip(m.View())
	if strings.Contains(view, "stale") || strings.Contains(view, "other-pr") || len(m.palette.MarkedLabels()) != before {
		t.Errorf("stale lists reached the picker:\n%s", view)
	}
}

func TestLabelPickerRepoLabelsError(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	prSvc.repoLabelsErr = errors.New("rest: unexpected status 502")
	drainCmds(m, key(m, keyL))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Couldn't load labels") || !strings.Contains(view, "[x] ● bug") {
		t.Errorf("error state:\n%s", view)
	}
}

func TestLabelPickerHoldsApplyUntilFullListLoads(t *testing.T) {
	m, prSvc := labelPickerModel(t)
	m.prDetail.Detail.LabelCount = 30 // more than the preview's first page
	prSvc.prLabels = []domain.Label{{Name: "bug"}, {Name: "docs"}}

	// L asks for the picker with a message; opening it returns the loads.
	var cmd tea.Cmd
	for _, msg := range runMsgs(key(m, keyL)) {
		_, cmd = m.Update(msg)
	}
	if !m.palette.ApplyHeld() {
		t.Fatal("apply should be held until the PR's full list loads")
	}
	key(m, keyEnter)
	if len(prSvc.applied) != 0 || !m.labelPicking() {
		t.Fatal("Enter must wait for the full list")
	}
	drainCmds(m, cmd)
	if m.palette.ApplyHeld() {
		t.Fatal("full list should release the hold")
	}
	if got := len(m.palette.MarkedLabels()); got != 2 {
		t.Errorf("baseline has %d labels, want 2 (bug, docs)", got)
	}
}

func TestLabelPickerNotInCommitViewOrDashboard(t *testing.T) {
	m := ansiDashboard(t, 120, 40)
	drainCmds(m, key(m, keyL))
	if m.labelPicking() {
		t.Error("L must not open the label picker on the dashboard")
	}
}

func TestLabelPickerFromKeymapOverlay(t *testing.T) {
	m, _ := labelPickerModel(t)
	drainCmds(m, key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}))
	if _, ok := m.keymapOverlay.DirectKey(keyL); !m.keymapOverlay.Visible || !ok {
		t.Fatal("the keybindings page should list L as a direct key")
	}
	drainCmds(m, key(m, keyL))
	if m.keymapOverlay.Visible || !m.labelPicking() {
		t.Error("L from the keybindings page should open the label picker")
	}
}

func TestLabelPickerSurvivesResize(t *testing.T) {
	m, _ := labelPickerModel(t)
	drainCmds(m, key(m, keyL))
	_, _ = m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	if !m.labelPicking() {
		t.Fatal("resize closed the picker")
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 70 {
			t.Fatalf("line %d is %d wide after resize", i, w)
		}
	}
}

// runMsgs runs cmd and any batch inside it, returning the resulting messages.
func runMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runMsgs(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}
