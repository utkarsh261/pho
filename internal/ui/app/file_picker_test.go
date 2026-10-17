package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

var keyCtrlP = tea.KeyMsg{Type: tea.KeyCtrlP}

// filePickerDiff has five files, a rename and a removal among them, each taller than the screen, so jumping
// to one has to scroll.
func filePickerDiff() diffmodel.DiffModel {
	file := func(path, status string) diffmodel.DiffFile {
		lines := make([]diffmodel.DiffLine, 60)
		for i := range lines {
			lines[i] = diffmodel.DiffLine{Kind: "addition", Raw: fmt.Sprintf("+%s line %d", path, i)}
		}
		return diffmodel.DiffFile{
			OldPath: path, NewPath: path, Status: status, Additions: len(lines),
			Hunks: []diffmodel.DiffHunk{{Header: "@@ -0,0 +1,60 @@", Lines: lines}},
		}
	}
	renamed := file("api/new_name.go", "renamed")
	renamed.OldPath = "api/old_name.go"
	// GitHub's file list can leave a removed file's new path empty.
	removed := file("api/gone.go", "removed")
	removed.NewPath = ""
	return diffmodel.DiffModel{
		Repo:     "acme/alpha",
		PRNumber: 123,
		Files: []diffmodel.DiffFile{
			file("web/login.go", "modified"),
			file("web/session.go", "added"),
			file("docs/README.md", "modified"),
			renamed,
			removed,
		},
		Stats: diffmodel.DiffStats{TotalFiles: 5, TotalAdditions: 300},
	}
}

func filePickerModel(t *testing.T, tab string) *Model {
	t.Helper()
	detail, _ := ansiPRDetailFixture()
	prSvc := &stubPRService{detailResult: detail, diffResult: filePickerDiff()}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{testutil.Repo("acme/alpha")}, prSvc)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	ansiKeys(m, tab)
	return m
}

func typeKeys(m *Model, s string) {
	for _, r := range s {
		key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestFilePickerJumpsToFile(t *testing.T) {
	m := filePickerModel(t, "2")
	drainCmds(m, key(m, keyCtrlP))
	if !m.filePicking() {
		t.Fatal("ctrl+p did not open the file picker from PR detail")
	}
	if got := m.palette.SelectedFile(); got != "web/login.go" {
		t.Errorf("picker starts on %q, want the current file", got)
	}
	if !strings.Contains(m.status.HintOverride, "Go to file") {
		t.Errorf("status hint = %q, want the picker's hints", m.status.HintOverride)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Go to file", "web/session.go", "docs/README.md"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}

	typeKeys(m, "readme")
	key(m, keyEnter)
	if m.filePicking() {
		t.Fatal("Enter left the picker open")
	}
	if got := m.prDetail.CurrentFile(); got != "docs/README.md" {
		t.Errorf("cursor is in %q, want docs/README.md", got)
	}
	if m.prDetail.ContentScroll == 0 {
		t.Error("the diff didn't scroll to the file")
	}
	if !strings.Contains(ansi.Strip(m.View()), "docs/README.md line 0") {
		t.Error("the file's first line isn't on screen")
	}
}

// TestFilePickerFromOtherTab checks a pick from the Description tab lands
// on the Diff tab.
func TestFilePickerFromOtherTab(t *testing.T) {
	m := filePickerModel(t, "1")
	drainCmds(m, key(m, keyCtrlP))
	if !m.filePicking() {
		t.Fatal("ctrl+p did not open the file picker on the Description tab")
	}
	key(m, keyDown)
	key(m, keyEnter)
	if tab := m.prDetail.ActiveTab(); tab != prdetail.TabDiff {
		t.Errorf("tab = %v, want Diff", tab)
	}
	if got := m.prDetail.CurrentFile(); got != "web/session.go" {
		t.Errorf("cursor is in %q, want web/session.go", got)
	}
}

func TestFilePickerEscLeavesDiffAlone(t *testing.T) {
	for _, closeKey := range []tea.KeyMsg{keyEsc, keyCtrlP} {
		t.Run(closeKey.String(), func(t *testing.T) {
			m := filePickerModel(t, "2")
			drainCmds(m, key(m, keyCtrlP))
			key(m, keyDown)
			key(m, closeKey)
			if m.filePicking() {
				t.Fatal("the picker is still open")
			}
			if m.currentView() != domain.PrimaryViewPRDetail {
				t.Fatalf("view = %s, want PR detail", m.currentView())
			}
			if got := m.prDetail.CurrentFile(); got != "web/login.go" {
				t.Errorf("cursor moved to %q", got)
			}
			if m.prDetail.ContentScroll != 0 {
				t.Errorf("diff scrolled to %d", m.prDetail.ContentScroll)
			}
		})
	}
}

func TestFilePickerNoMatchEnterStaysOpen(t *testing.T) {
	m := filePickerModel(t, "2")
	drainCmds(m, key(m, keyCtrlP))
	typeKeys(m, "zzz")
	key(m, keyEnter)
	if !m.filePicking() {
		t.Error("Enter with no matches closed the picker")
	}
}

func TestFilePickerAndHelpDontStack(t *testing.T) {
	m := filePickerModel(t, "2")
	m.Update(prdetail.ToggleKeymapOverlayMsg{})
	m.Update(prdetail.OpenFilePicker{})
	if m.filePicking() {
		t.Error("the picker opened over the help overlay")
	}
}

// TestCtrlPOnDashboardStillGoesToPRs checks the dashboard keeps its palette.
func TestCtrlPOnDashboardStillGoesToPRs(t *testing.T) {
	m := ansiDashboard(t, 120, 40)
	drainCmds(m, key(m, keyCtrlP))
	if !m.state.Search.OverlayOpen || m.filePicking() {
		t.Errorf("overlay open = %v, file picker = %v", m.state.Search.OverlayOpen, m.filePicking())
	}
}

// pickFile opens the picker, filters by query and presses Enter.
func pickFile(m *Model, query string) {
	drainCmds(m, key(m, keyCtrlP))
	typeKeys(m, query)
	key(m, keyEnter)
}

func TestFilePickerRenamedAndRemoved(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"new_name", "api/new_name.go"},
		{"gone", "api/gone.go"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			m := filePickerModel(t, "2")
			pickFile(m, tc.query)
			if got := m.prDetail.CurrentFile(); got != tc.want {
				t.Errorf("cursor is in %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFilePickerExpandsCollapsedFile checks picking a collapsed file opens
// it rather than landing on its placeholder.
func TestFilePickerExpandsCollapsedFile(t *testing.T) {
	m := filePickerModel(t, "2")
	ansiKeys(m, "z")
	if strings.Contains(ansi.Strip(m.View()), "web/login.go line 0") {
		t.Fatal("z did not collapse the first file")
	}
	pickFile(m, "login")
	if !strings.Contains(ansi.Strip(m.View()), "web/login.go line 0") {
		t.Error("the picked file stayed collapsed")
	}
}

// TestFilePickerDiffReloadedUnderneath checks a pick still lands by path
// after the diff reloads with its files in a new order, and does nothing
// when the picked file is gone.
func TestFilePickerDiffReloadedUnderneath(t *testing.T) {
	reload := func(m *Model, files ...diffmodel.DiffFile) {
		d := filePickerDiff()
		d.Files = files
		drainCmds(m, func() tea.Msg {
			return cmds.DiffLoaded{Repo: "acme/alpha", Number: 123, Diff: d}
		})
	}
	files := filePickerDiff().Files

	m := filePickerModel(t, "2")
	drainCmds(m, key(m, keyCtrlP))
	typeKeys(m, "readme")
	reload(m, files[2], files[0], files[1])
	key(m, keyEnter)
	if got := m.prDetail.CurrentFile(); got != "docs/README.md" {
		t.Errorf("cursor is in %q after reorder, want docs/README.md", got)
	}

	m = filePickerModel(t, "2")
	drainCmds(m, key(m, keyCtrlP))
	typeKeys(m, "readme")
	reload(m, files[0], files[1])
	key(m, keyEnter)
	if m.filePicking() {
		t.Error("the picker stayed open")
	}
	if got := m.prDetail.CurrentFile(); got != "web/login.go" || m.prDetail.ContentScroll != 0 {
		t.Errorf("cursor moved to %q (scroll %d) for a file no longer in the diff", got, m.prDetail.ContentScroll)
	}
}

func TestFilePickerInCommitView(t *testing.T) {
	detail, _ := ansiPRDetailFixture()
	commitDiff := filePickerDiff()
	commitDiff.Files = commitDiff.Files[1:3]
	prSvc := &stubPRService{
		detailResult: detail,
		diffResult:   filePickerDiff(),
		commits:      []domain.Commit{{SHA: "a1b2c3d4e5", MessageHeadline: "Add sessions", AuthorLogin: "octocat"}},
		commitDiff:   commitDiff,
	}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{testutil.Repo("acme/alpha")}, prSvc)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())
	ansiKeys(m, "4", "enter")
	if m.currentView() != domain.PrimaryViewCommitDetail {
		t.Fatalf("expected commit view, got %s", m.currentView())
	}

	drainCmds(m, key(m, keyCtrlP))
	if !m.filePicking() {
		t.Fatal("ctrl+p did not open the file picker in the commit view")
	}
	// The box's top border counts the listed files.
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Go to file") || !strings.Contains(view, " 1/2 ") {
		t.Errorf("picker doesn't list the commit's 2 files:\n%s", view)
	}
	typeKeys(m, "readme")
	key(m, keyEnter)
	if got := m.commitDetail.Inner().CurrentFile(); got != "docs/README.md" {
		t.Errorf("cursor is in %q, want docs/README.md", got)
	}
	if m.currentView() != domain.PrimaryViewCommitDetail {
		t.Errorf("view = %s, want the commit view", m.currentView())
	}
}
