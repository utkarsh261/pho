package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

var keyTab = tea.KeyMsg{Type: tea.KeyTab}

func repoLabel(name, desc string) domain.RepoLabel {
	return domain.RepoLabel{Label: domain.Label{Name: name, Color: "d73a4a"}, Description: desc}
}

var pickRepoLabels = []domain.RepoLabel{
	repoLabel("good first issue", "Easy one"),
	repoLabel("bug", "Something isn't working"),
	repoLabel("Area/Diff", ""),
	repoLabel("needs-review", "Waiting on a reviewer"),
	repoLabel("build", "CI and tooling"),
}

// newLabelPick opens the picker on a PR labelled zeta and bug, with the
// repo's labels loaded.
func newLabelPick(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.OpenLabelPick([]domain.Label{{Name: "zeta", Color: "0e8a16"}, {Name: "bug", Color: "d73a4a"}})
	m.SetRepoLabels(pickRepoLabels, false)
	return m
}

func rowTitles(m Model) []string {
	out := make([]string, len(m.results))
	for i, r := range m.results {
		out[i] = r.Title
		if r.Kind == labelCreateResult {
			out[i] = "+" + r.Title
		}
	}
	return out
}

func selectTitle(t *testing.T, m *Model, title string) {
	t.Helper()
	for i, r := range m.results {
		if r.Title == title {
			m.selectedIndex = i
			return
		}
	}
	t.Fatalf("no row %q in %v", title, rowTitles(*m))
}

func TestLabelPickOrdersAppliedFirstThenAlphabetical(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	want := "zeta bug Area/Diff build good first issue needs-review"
	if got := strings.Join(rowTitles(m), " "); got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
	if !m.IsLabelPick() || m.PendingLabelCount() != 0 {
		t.Errorf("IsLabelPick=%v pending=%d", m.IsLabelPick(), m.PendingLabelCount())
	}
}

func TestLabelPickBeforeRepoLabelsLoad(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.OpenLabelPick([]domain.Label{{Name: "bug"}})
	m = typeRunes(m, "brand-new")
	if len(m.results) != 0 {
		t.Errorf("no create row until the repo's labels are known, got %v", rowTitles(m))
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Loading…") || !strings.Contains(view, "Loading labels…") {
		t.Errorf("loading state missing:\n%s", view)
	}
}

func TestLabelPickTabTogglesWithoutReordering(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	before := strings.Join(rowTitles(m), " ")

	selectTitle(t, &m, "bug")
	m, _ = m.Update(keyTab) // unmark bug, move to Area/Diff
	m, _ = m.Update(keyTab) // mark Area/Diff
	if got := strings.Join(rowTitles(m), " "); got != before {
		t.Errorf("rows reordered after toggling: %q", got)
	}
	c := m.LabelChange()
	if strings.Join(c.Add, ",") != "Area/Diff" || strings.Join(c.Remove, ",") != "bug" || len(c.Create) != 0 {
		t.Errorf("change = %+v", c)
	}
	if m.PendingLabelCount() != 2 {
		t.Errorf("pending = %d", m.PendingLabelCount())
	}
	got := ""
	for _, l := range m.MarkedLabels() {
		got += l.Name + ","
	}
	if got != "zeta,Area/Diff," {
		t.Errorf("marked = %q", got)
	}
}

func TestLabelPickMarksSurviveFiltering(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	m = typeRunes(m, "good")
	m, _ = m.Update(keyTab)
	for range "good" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if c := m.LabelChange(); strings.Join(c.Add, ",") != "good first issue" {
		t.Errorf("mark lost across filter edits: %+v", c)
	}
}

func TestLabelPickSpaceTypesIntoQuery(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	m = typeRunes(m, "first issue")
	if m.query != "first issue" || strings.Join(rowTitles(m), "|") != "good first issue|+first issue" {
		t.Errorf("query %q rows %v", m.query, rowTitles(m))
	}
}

func TestLabelPickCreateRow(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)

	m = typeRunes(m, "BUG")
	if strings.Contains(strings.Join(rowTitles(m), " "), "+") {
		t.Errorf("existing label (any case) must not offer create: %v", rowTitles(m))
	}

	m.query, m.cursor = "", 0
	m = typeRunes(m, "needs  qa")
	titles := rowTitles(m)
	if titles[len(titles)-1] != "+needs qa" {
		t.Fatalf("rows = %v, want create row last with folded spaces", titles)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, `Create label ● “needs qa”`) {
		t.Errorf("create row not drawn:\n%s", view)
	}

	selectTitle(t, &m, "needs qa")
	m, _ = m.Update(keyTab)
	if m.query != "" {
		t.Errorf("query should clear after creating, got %q", m.query)
	}
	if got := m.results[m.selectedIndex].Title; got != "needs qa" {
		t.Errorf("selection = %q, want the new label", got)
	}
	c := m.LabelChange()
	if len(c.Create) != 1 || c.Create[0].Name != "needs qa" || c.Create[0].Color == "" || strings.Join(c.Add, ",") != "needs qa" {
		t.Errorf("change = %+v", c)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "[x] ● needs qa") || !strings.Contains(view, "new") {
		t.Errorf("new label row missing:\n%s", view)
	}

	// Tab again drops the not-yet-created label entirely.
	m, _ = m.Update(keyTab)
	if c := m.LabelChange(); !c.Empty() {
		t.Errorf("change after removing new label = %+v", c)
	}
	for _, r := range m.pickPool {
		if r.Title == "needs qa" {
			t.Error("new label should leave the list when unmarked")
		}
	}
}

func TestLabelPickCreateNameTooLong(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	m = typeRunes(m, strings.Repeat("x", maxLabelNameLen+1))
	selectTitle(t, &m, strings.Repeat("x", maxLabelNameLen+1))
	if m.ToggleMark(false) {
		t.Error("a too-long name must not be creatable")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Name too long (max 50)") {
		t.Errorf("reason not shown:\n%s", view)
	}
}

func TestLabelPickLoadErrorKeepsPRLabels(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.OpenLabelPick([]domain.Label{{Name: "bug"}})
	m.SetLabelsErr("rest: unexpected status 502")
	m = typeRunes(m, "bu")
	if got := strings.Join(rowTitles(m), " "); got != "bug" {
		t.Errorf("rows = %q; PR labels stay, no create row", got)
	}
	m.query, m.cursor = "", 0
	m = typeRunes(m, "zzz")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Couldn't load labels") {
		t.Errorf("error not shown:\n%s", view)
	}
}

func TestLabelPickRepoListDedupesByCase(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.OpenLabelPick([]domain.Label{{Name: "Bug", Color: "000000"}})
	m.SetRepoLabels([]domain.RepoLabel{repoLabel("bug", "Broken")}, false)
	if got := strings.Join(rowTitles(m), " "); got != "Bug" {
		t.Fatalf("rows = %q, want one row keeping the PR's spelling", got)
	}
	if it := m.labels.items["Bug"]; it.Description != "Broken" || it.Color != "000000" {
		t.Errorf("item = %+v", it)
	}
}

func TestLabelPickFullPRListKeepsToggles(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	m.SetPRLabelsLoading()
	if !m.LoadingPRLabels() {
		t.Fatal("should be loading the PR's labels")
	}
	selectTitle(t, &m, "build")
	m, _ = m.Update(keyTab) // add build

	// The full list shows the PR also has needs-review (beyond the first page).
	m.SetPRLabels([]domain.Label{{Name: "zeta"}, {Name: "bug"}, {Name: "needs-review"}})
	if m.LoadingPRLabels() {
		t.Error("full list should end the loading state")
	}
	c := m.LabelChange()
	if strings.Join(c.Add, ",") != "build" || len(c.Remove) != 0 {
		t.Errorf("change = %+v; build stays added, needs-review isn't removed", c)
	}
	if got := strings.Join(rowTitles(m)[:3], " "); got != "zeta bug needs-review" {
		t.Errorf("applied rows = %q", got)
	}
}

func TestLabelPickBorderNotes(t *testing.T) {
	t.Parallel()
	m := newLabelPick(t)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "2 applied") || !strings.Contains(view, "Labels") {
		t.Errorf("border:\n%s", view)
	}
	m, _ = m.Update(keyTab)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "1 pending") {
		t.Errorf("pending note missing:\n%s", view)
	}
	m.SetRepoLabels(nil, true)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "first 1000 shown") && !strings.Contains(view, "pending") {
		t.Errorf("truncation note missing:\n%s", view)
	}
}

func TestLabelPickRowsFitWidth(t *testing.T) {
	t.Parallel()
	for _, w := range []int{30, 50, 100} {
		m := NewModel(nil)
		m.SetTheme(theme.Default())
		m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m.OpenLabelPick([]domain.Label{{Name: strings.Repeat("long-name-", 6), Color: "d73a4a"}})
		m.SetRepoLabels([]domain.RepoLabel{repoLabel("🚀 release", strings.Repeat("words ", 30))}, false)
		for i, line := range strings.Split(m.View(), "\n") {
			if lw := ansi.StringWidth(line); lw > w {
				t.Fatalf("width %d: line %d is %d wide: %q", w, i, lw, ansi.Strip(line))
			}
		}
	}
}
