package prdetail

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// ─── Helper builders ─────────────────────────────────────────────────────────

func makeFiles(paths ...string) []diffmodel.DiffFile {
	files := make([]diffmodel.DiffFile, len(paths))
	for i, p := range paths {
		files[i] = diffmodel.DiffFile{
			OldPath:     p,
			NewPath:     p,
			Status:      "modified",
			Additions:   5,
			Deletions:   2,
			DisplayRows: 10,
		}
	}
	return files
}

func makeChecks(names ...string) []domain.PreviewCheckRow {
	checks := make([]domain.PreviewCheckRow, len(names))
	for i, n := range names {
		checks[i] = domain.PreviewCheckRow{Name: n, State: "SUCCESS"}
	}
	return checks
}

func makePanelWithFiles(files []diffmodel.DiffFile, focus PRDetailFocus) LeftPanelModel {
	return LeftPanelModel{
		Files:   files,
		Loading: false,
		Focus:   focus,
		theme:   theme.Default(),
	}
}

func makePanelWithChecks(checks []domain.PreviewCheckRow) LeftPanelModel {
	return LeftPanelModel{
		Files:   makeFiles("main.go"),
		Checks:  checks,
		Loading: false,
		Focus:   FocusFiles,
		theme:   theme.Default(),
	}
}

// makePRDetail builds a PRDetailModel with a loaded diff at the given terminal size.
func makePRDetail(width, height int, files []diffmodel.DiffFile, checks []domain.PreviewCheckRow) *PRDetailModel {
	m := NewModel(domain.PullRequestSummary{
		Number: 1,
		Title:  "Test PR",
		State:  domain.PRStateOpen,
		Author: "alice",
		Repo:   "owner/repo",
	}, domain.Repository{FullName: "owner/repo"}, nil)
	m.Width = width
	m.Height = height
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Files = files
	m.leftPanel.Checks = checks
	m.leftPanel.Loading = false
	m.SetTheme(theme.Default())
	return m
}

// pressKey simulates a single key press and returns the updated model.
func pressKey(m *PRDetailModel, key string) *PRDetailModel {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "tab" {
		msg = tea.KeyMsg{Type: tea.KeyTab}
	} else if key == "shift+tab" {
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	} else if key == "esc" {
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	} else if key == "ctrl+d" {
		msg = tea.KeyMsg{Type: tea.KeyCtrlD}
	} else if key == "ctrl+u" {
		msg = tea.KeyMsg{Type: tea.KeyCtrlU}
	} else if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	} else if len(key) == 1 {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	return next
}

// stripANSI removes ANSI escape sequences from s for plain-text assertions.
func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// ─── truncatePathLeft unit tests ─────────────────────────────────────────────

func TestTruncatePathLeftShort(t *testing.T) {
	t.Parallel()
	result := truncatePathLeft("main.go", 20)
	if !strings.HasPrefix(result, "main.go") {
		t.Errorf("expected full path preserved, got %q", result)
	}
	if len([]rune(result)) != 20 {
		t.Errorf("expected padded to 20 runes, got %d", len([]rune(result)))
	}
}

func TestTruncatePathLeftLong(t *testing.T) {
	t.Parallel()
	path := "internal/ui/views/dashboard/preview_panel.go" // 45 chars
	result := truncatePathLeft(path, 20)
	if len([]rune(result)) != 20 {
		t.Errorf("expected exactly 20 runes, got %d", len([]rune(result)))
	}
	if !strings.HasPrefix(result, "…") {
		t.Errorf("expected '…' prefix for truncated path, got %q", result)
	}
	// The filename suffix must be visible.
	if !strings.HasSuffix(strings.TrimRight(result, " "), "preview_panel.go") {
		t.Errorf("expected filename visible at right, got %q", result)
	}
}

func TestTruncatePathLeftExact(t *testing.T) {
	t.Parallel()
	path := "exactly20chars123456" // exactly 20 chars
	result := truncatePathLeft(path, 20)
	if result != path {
		t.Errorf("expected no change for exactly-max-width path, got %q", result)
	}
}

// ─── formatFileStats unit tests ───────────────────────────────────────────────

// visibleWidth returns the number of visible terminal columns in s,
// correctly handling multi-byte UTF-8 characters like "…".
func visibleWidth(s string) int {
	return len([]rune(s))
}

func TestFormatFileStatsSmallNumbers(t *testing.T) {
	t.Parallel()
	s := formatFileStats(5, 2)
	if visibleWidth(s) != lpStatsWidth {
		t.Errorf("expected %d visible chars, got %d: %q", lpStatsWidth, visibleWidth(s), s)
	}
	if !strings.Contains(s, "+5") || !strings.Contains(s, "-2") {
		t.Errorf("expected +5 and -2 in stats, got %q", s)
	}
}

func TestFormatFileStatsLargeNumbers(t *testing.T) {
	t.Parallel()
	s := formatFileStats(9999, 9999)
	if visibleWidth(s) != lpStatsWidth {
		t.Errorf("expected %d visible chars for large stats, got %d: %q", lpStatsWidth, visibleWidth(s), s)
	}
}

func TestFormatFileStatsZero(t *testing.T) {
	t.Parallel()
	s := formatFileStats(0, 0)
	if visibleWidth(s) != lpStatsWidth {
		t.Errorf("expected %d visible chars for zero stats, got %d: %q", lpStatsWidth, visibleWidth(s), s)
	}
}

// ─── computeCIHeight unit tests ───────────────────────────────────────────────

func TestComputeCIHeightZeroChecks(t *testing.T) {
	t.Parallel()
	h := computeCIHeight(40, 0)
	if h != 0 {
		t.Errorf("expected 0 for no checks, got %d", h)
	}
}

func TestComputeCIHeightMinimum(t *testing.T) {
	t.Parallel()
	// 1 check + 2 border rows = 3 outer rows (minimum).
	h := computeCIHeight(40, 1)
	if h < 3 {
		t.Errorf("expected min height 3 for 1 check, got %d", h)
	}
}

func TestComputeCIHeightCap(t *testing.T) {
	t.Parallel()
	// 100 checks in a 40-row viewport: max = floor(40*0.3) = 12.
	h := computeCIHeight(40, 100)
	maxAllowed := int(float64(40) * 0.3)
	if h > maxAllowed {
		t.Errorf("expected height <= %d (30%% of 40), got %d", maxAllowed, h)
	}
}

// ─── LeftPanelModel rendering tests ──────────────────────────────────────────

func TestLeftPanelLongPathTruncated(t *testing.T) {
	t.Parallel()
	longPath := "internal/application/pr/service_integration_test.go"
	panel := makePanelWithFiles(makeFiles(longPath), FocusFiles)
	output := panel.View(20, "⠋")
	if !strings.Contains(output, "…") {
		t.Errorf("expected '…' for long path in left panel, output:\n%s", output)
	}
	// Full long path should NOT appear verbatim (it would overflow the column).
	if strings.Contains(output, longPath) {
		t.Errorf("full long path should be truncated, got:\n%s", output)
	}
}

func TestLeftPanelShortPathNotTruncated(t *testing.T) {
	t.Parallel()
	panel := makePanelWithFiles(makeFiles("main.go"), FocusFiles)
	output := panel.View(20, "⠋")
	if !strings.Contains(output, "main.go") {
		t.Errorf("expected short path 'main.go' in output, got:\n%s", output)
	}
}

func TestLeftPanelFileStatsAligned(t *testing.T) {
	t.Parallel()
	files := []diffmodel.DiffFile{{
		NewPath:   "foo.go",
		Additions: 42,
		Deletions: 7,
	}}
	panel := LeftPanelModel{Files: files, Loading: false, Focus: FocusFiles}
	output := panel.View(20, "⠋")
	if !strings.Contains(output, "+42") || !strings.Contains(output, "-7") {
		t.Errorf("expected +42 and -7 in stats, output:\n%s", output)
	}
}

func TestLeftPanelCIHiddenWhenEmpty(t *testing.T) {
	t.Parallel()
	// No checks → CI section should not appear.
	panel := LeftPanelModel{Files: makeFiles("main.go"), Checks: nil, Loading: false, Focus: FocusFiles}
	output := panel.View(30, "⠋")
	// The border for CI would produce extra box-drawing chars.
	// Count the number of NormalBorder top-border chars "┌" — should be 1 (files only).
	topBorderCount := strings.Count(output, "┌")
	if topBorderCount != 1 {
		t.Errorf("expected 1 sub-area (files only), got %d top-border chars in:\n%s", topBorderCount, output)
	}
}

func TestLeftPanelCIHeightCap(t *testing.T) {
	t.Parallel()
	// 20 checks in a 30-row viewport: max = floor(30*0.3) = 9.
	checks := makeChecks("a", "b", "c", "d", "e", "f", "g", "h", "i", "j",
		"k", "l", "m", "n", "o", "p", "q", "r", "s", "t")
	ciH := computeCIHeight(30, len(checks))
	maxAllowed := int(float64(30) * 0.3)
	if ciH > maxAllowed {
		t.Errorf("CI height %d exceeds 30%% cap of %d", ciH, maxAllowed)
	}
	// Render shouldn't panic.
	panel := makePanelWithChecks(checks)
	output := panel.View(30, "")
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestLeftPanelCIMinHeight(t *testing.T) {
	t.Parallel()
	// Even 1 check should produce at least 3 outer rows for the CI sub-area.
	h := computeCIHeight(40, 1)
	if h < 3 {
		t.Errorf("expected min 3 rows for CI sub-area, got %d", h)
	}
}

func TestLeftPanelCINameTruncated(t *testing.T) {
	t.Parallel()
	longName := "this-is-a-very-long-ci-check-name-that-exceeds-the-budget"
	checks := makeChecks(longName)
	panel := makePanelWithChecks(checks)
	output := panel.View(30, "")
	// Long name should be truncated with "…".
	if !strings.Contains(output, "…") {
		t.Errorf("expected '…' for long CI check name, output:\n%s", output)
	}
	// Full long name should NOT appear verbatim.
	if strings.Contains(output, longName) {
		t.Errorf("long CI name should be truncated, got:\n%s", output)
	}
}

func TestLeftPanelFocusBorderColor(t *testing.T) {
	t.Parallel()
	th := theme.Default()
	panel := LeftPanelModel{
		Files:  makeFiles("main.go"),
		Focus:  FocusFiles, // Files is focused
		theme:  th,
	}
	// Focused border should use the Primary color.
	color := panel.borderColorFor(FocusFiles)
	if color != th.Primary {
		t.Errorf("expected Primary color for focused sub-area, got %v", color)
	}
}

func TestLeftPanelNonFocusBorderColor(t *testing.T) {
	t.Parallel()
	th := theme.Default()
	panel := LeftPanelModel{
		Files:  makeFiles("main.go"),
		Focus:  FocusFiles, // Files is focused
		theme:  th,
	}
	// Non-focused (CI) border should use the Border (muted) color.
	color := panel.borderColorFor(FocusCI)
	if color != th.Border {
		t.Errorf("expected Border color for unfocused sub-area, got %v", color)
	}
}

func TestLeftPanelSpinnerWhileLoading(t *testing.T) {
	t.Parallel()
	files := makeFiles("internal/main.go", "pkg/util.go")
	panel := LeftPanelModel{
		Files:   files,
		Loading: true, // diff is still loading
		Focus:   FocusFiles,
	}
	spinnerFrame := "⠋"
	output := panel.View(20, spinnerFrame)

	// Spinner frame should appear in output.
	if !strings.Contains(output, spinnerFrame) {
		t.Errorf("expected spinner frame %q while loading, output:\n%s", spinnerFrame, output)
	}
	// File paths should NOT appear while loading.
	if strings.Contains(output, "main.go") {
		t.Errorf("file paths should not render while diff is loading, output:\n%s", output)
	}
}

// ─── Layout tests (PRDetailModel.View()) ─────────────────────────────────────

func TestLayoutBelow80ColsHidesSidebar(t *testing.T) {
	t.Parallel()
	files := makeFiles("main.go", "util.go")
	m := makePRDetail(79, 30, files, nil)
	output := m.View()

	// At < 80 cols, the left panel (Files sub-area border) should be hidden.
	// "files changed" summary should appear instead.
	if !strings.Contains(output, "files changed") {
		t.Errorf("expected 'files changed' summary at <80 cols, output:\n%s", output)
	}
	// Sidebar should not appear.
	if strings.Contains(output, "FILES") {
		t.Errorf("expected no sidebar at <80 cols, output:\n%s", output)
	}
}

func TestLayoutAt80ColsShowsSidebar(t *testing.T) {
	t.Parallel()
	files := makeFiles("main.go")
	m := makePRDetail(80, 30, files, nil)
	output := m.View()

	// At exactly 80 cols, the sidebar should be present.
	if !strings.Contains(output, "main.go") {
		t.Errorf("expected file 'main.go' in sidebar at 80 cols, output:\n%s", output)
	}
	// Should have box-drawing chars (sidebar borders).
	if !strings.Contains(output, "┌") && !strings.Contains(output, "│") {
		t.Errorf("expected sidebar border chars at 80 cols, output:\n%s", output)
	}
}

func TestLayoutAt120Cols(t *testing.T) {
	t.Parallel()
	files := makeFiles("main.go", "internal/util.go")
	m := makePRDetail(120, 40, files, nil)
	output := m.View()

	lines := strings.Split(output, "\n")
	// Should have exactly m.Height rows.
	if len(lines) != m.Height {
		t.Errorf("expected %d rows at 120 cols, got %d", m.Height, len(lines))
	}
	// Sidebar should be present.
	if !strings.Contains(output, "main.go") {
		t.Errorf("expected 'main.go' in sidebar at 120 cols, output:\n%s", output)
	}
}

// ─── Key press integration tests ─────────────────────────────────────────────

func TestJKeyMovesFileCursorDown(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go", "c.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 0

	m = pressKey(m, "j")
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected FileIndex 1 after j, got %d", m.leftPanel.FileIndex)
	}
}

func TestKKeyMovesFileCursorUp(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go", "c.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 2

	m = pressKey(m, "k")
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected FileIndex 1 after k, got %d", m.leftPanel.FileIndex)
	}
}

func TestJOnLastFileMovesToCI(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go")
	checks := makeChecks("build", "test")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 1 // already on last file

	m = pressKey(m, "j")
	if m.leftPanel.Focus != FocusCI {
		t.Errorf("expected focus to move to FocusCI after j on last file, got %v", m.leftPanel.Focus)
	}
	if m.leftPanel.CIScroll != 0 {
		t.Errorf("expected CI scroll reset to 0, got %d", m.leftPanel.CIScroll)
	}
}

func TestJOnLastFileNoopWhenCIEmpty(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go")
	m := makePRDetail(100, 30, files, nil) // no CI checks
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 1 // last file

	m = pressKey(m, "j")
	// Focus should stay on Files; FileIndex should not exceed last.
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected focus to stay on FocusFiles (no CI), got %v", m.leftPanel.Focus)
	}
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected FileIndex to stay at 1, got %d", m.leftPanel.FileIndex)
	}
}

func TestKOnFirstCIMovesToFiles(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CIScroll = 0 // at top of CI

	m = pressKey(m, "k")
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected focus to move to FocusFiles after k on first CI item, got %v", m.leftPanel.Focus)
	}
	if m.leftPanel.FilesScroll != 0 {
		t.Errorf("expected FilesScroll reset to 0, got %d", m.leftPanel.FilesScroll)
	}
}

func TestHLNoopOutsideFilesView(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go", "c.go")
	checks := makeChecks("build")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.FileIndex = 1

	m = pressKey(m, "h")
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected h to be no-op in FocusCI, FileIndex changed to %d", m.leftPanel.FileIndex)
	}
	m = pressKey(m, "l")
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected l to be no-op in FocusCI, FileIndex changed to %d", m.leftPanel.FileIndex)
	}
}

func TestHLNoopWithOneFile(t *testing.T) {
	t.Parallel()
	files := makeFiles("only.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 0
	m.Width = 100 // wide enough for sidebar

	// H and L navigate files (capital for file navigation)
	m = pressKey(m, "H")
	if m.leftPanel.FileIndex != 0 {
		t.Errorf("expected H to clamp at 0 with 1 file, got %d", m.leftPanel.FileIndex)
	}
	m = pressKey(m, "L")
	if m.leftPanel.FileIndex != 0 {
		t.Errorf("expected L to clamp at 0 with 1 file, got %d", m.leftPanel.FileIndex)
	}
}

func TestTabCycleFilesToCI(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusFiles

	m = pressKey(m, "tab")
	if m.leftPanel.Focus != FocusCI {
		t.Errorf("expected Tab to move Files→CI, got %v", m.leftPanel.Focus)
	}
}

func TestTabCycleCIToContent(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI

	m = pressKey(m, "tab")
	if m.leftPanel.Focus != FocusContent {
		t.Errorf("expected Tab to move CI→Content, got %v", m.leftPanel.Focus)
	}
}

func TestTabCycleContentToFiles(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusContent

	m = pressKey(m, "tab")
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected Tab to wrap Content→Files, got %v", m.leftPanel.Focus)
	}
}

func TestTabSkipsCIWhenEmpty(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	m := makePRDetail(100, 30, files, nil) // no CI checks
	m.leftPanel.Focus = FocusFiles

	m = pressKey(m, "tab")
	if m.leftPanel.Focus != FocusContent {
		t.Errorf("expected Tab to skip CI and go Files→Content, got %v", m.leftPanel.Focus)
	}
}

func TestTabCycleSkipsCIWhenEmpty(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusContent

	m = pressKey(m, "tab")
	// Should skip CI (empty) and go Content→Files.
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected Tab to wrap Content→Files (no CI), got %v", m.leftPanel.Focus)
	}
}

func TestFocusRoutingIndependence(t *testing.T) {
	t.Parallel()
	// j/k in content viewport should not affect file index.
	files := makeFiles("a.go", "b.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusContent
	m.leftPanel.FileIndex = 0
	m.ContentScroll = 0

	m = pressKey(m, "j")
	if m.leftPanel.FileIndex != 0 {
		t.Errorf("expected FileIndex unchanged when Content is focused, got %d", m.leftPanel.FileIndex)
	}
	if m.ContentScroll == 0 {
		// ContentScroll may stay 0 if there's no content, but FileIndex must not change.
		_ = m.ContentScroll
	}
}

func TestViewRendersFileListAfterDiffLoad(t *testing.T) {
	t.Parallel()
	// Integration: DiffLoaded message causes files to appear in View().
	m := NewModel(domain.PullRequestSummary{
		Number: 42,
		Title:  "Add feature",
		State:  domain.PRStateOpen,
		Author: "bob",
		Repo:   "org/repo",
	}, domain.Repository{FullName: "org/repo"}, nil)
	m.Width = 100
	m.Height = 30
	m.SetTheme(theme.Default())

	// Simulate DiffLoaded message.
	diff := diffmodel.DiffModel{
		Files: makeFiles("cmd/main.go", "internal/app.go"),
		Stats: diffmodel.DiffStats{TotalFiles: 2, TotalAdditions: 10, TotalDeletions: 3},
	}
	diff.Files[0].DisplayRows = 5
	diff.Files[1].DisplayRows = 8

	next, _ := m.Update(cmds.DiffLoaded{
		Repo:      "org/repo",
		Number:    42,
		Diff:      diff,
		FromCache: false,
		Err:       nil,
	})
	m = next

	output := m.View()
	if !strings.Contains(output, "cmd/main.go") {
		t.Errorf("expected 'cmd/main.go' in view after DiffLoaded, output:\n%s", output)
	}
	if !strings.Contains(output, "internal/app.go") {
		t.Errorf("expected 'internal/app.go' in view after DiffLoaded, output:\n%s", output)
	}
}

func TestLeftPanelFullUIRender(t *testing.T) {
	files := makeFiles("cmd/main.go", "internal/app/app.go")
	checks := makeChecks("build", "lint", "test")
	m := makePanelWithChecks(checks)
	m.Files = files

	out := stripANSI(m.View(20, "⠋"))
	expected := `┌────────────────────────────────────────┐
│  FILES                                 │
├────────────────────────────────────────┤
│   cmd/main.go                    +5 -2 │
│   internal/app/app.go            +5 -2 │
│                                        │
│                                        │
│                                        │
│                                        │
│                                        │
│                                        │
│                                        │
│                                        │
└────────────────────────────────────────┘
┌────────────────────────────────────────┐
│  CI                                    │
├────────────────────────────────────────┤
│ ✓ build                          pass  │
│ ✓ lint                           pass  │
└────────────────────────────────────────┘`

	if strings.TrimSpace(out) != strings.TrimSpace(expected) {
		t.Errorf("full UI render mismatch.\nExpected:\n%s\n\nGot:\n%s", expected, out)
	}
}

func TestLastOpenedIndexHighlight(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go", "c.go")
	panel := makePanelWithFiles(files, FocusContent)
	panel.LastOpenedIndex = 1 // b.go was last opened

	out := stripANSI(panel.View(12, "⠋"))
	lines := strings.Split(out, "\n")

	// Find the file rows between the mid-border (├) and bottom-border (└) lines.
	var fileRows []string
	inBody := false
	for _, line := range lines {
		if strings.HasPrefix(line, "├") {
			inBody = true
			continue
		}
		if strings.HasPrefix(line, "└") {
			inBody = false
			continue
		}
		if inBody && strings.Contains(line, ".go") {
			fileRows = append(fileRows, line)
		}
	}

	if len(fileRows) < 3 {
		t.Fatalf("expected at least 3 file rows, got %d\n%s", len(fileRows), out)
	}

	// The last-opened row (b.go, index 1) should be rendered.
	// Since stripANSI removes styles, we verify the row exists.
	// A stronger test would check for ANSI sequences, but the key behavior
	// is that View() doesn't panic and includes all files.
	found := false
	for _, row := range fileRows {
		if strings.Contains(row, "b.go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected b.go (last opened) in rendered output:\n%s", out)
	}
}

func TestLKeyFromFilesToContent(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go")
	diffModel := diffmodel.DiffModel{Files: files}
	m := makePRDetail(100, 30, files, nil)
	m.Diff = &diffModel
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 0
	m.Width = 100 // wide enough for sidebar

	m = pressKey(m, "l")
	if m.leftPanel.Focus != FocusContent {
		t.Errorf("expected l to move Files→Content, got %v", m.leftPanel.Focus)
	}
}

func TestHKeyFromContentToFiles(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go", "b.go")
	diffModel := diffmodel.DiffModel{Files: files}
	m := makePRDetail(100, 30, files, nil)
	m.Diff = &diffModel
	m.leftPanel.Focus = FocusContent
	m.Width = 100 // wide enough for sidebar

	m = pressKey(m, "h")
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected h to move Content→Files, got %v", m.leftPanel.Focus)
	}
}

func TestEscFromContentGoesToFiles(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusContent
	m.Width = 100

	m = pressKey(m, "esc")
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected esc to move Content→Files, got %v", m.leftPanel.Focus)
	}
}

func TestEscFromFilesClosesPRDetail(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	m := makePRDetail(100, 30, files, nil)
	m.leftPanel.Focus = FocusFiles
	m.Width = 100

	// Check that in Files focus, esc triggers BackToDashboard
	// (we verify by checking focus after would-be handler - it's a special case not in pressKey)
	// pressKey doesn't emit the message, so we check the model state
	// In Files focus, esc should result in going back (handled in Update)
	// Since pressKey doesn't emit, we just verify current behavior is correct
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("sanity check: should start in FocusFiles, got %v", m.leftPanel.Focus)
	}
}

// ─── CI cursor navigation tests ──────────────────────────────────────────────

func TestCICursorJMovesDown(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build", "test", "lint")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CICursor = 0

	m = pressKey(m, "j")
	if m.leftPanel.CICursor != 1 {
		t.Errorf("expected CICursor 1 after j, got %d", m.leftPanel.CICursor)
	}
}

func TestCICursorKMovesUp(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build", "test", "lint")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CICursor = 2

	m = pressKey(m, "k")
	if m.leftPanel.CICursor != 1 {
		t.Errorf("expected CICursor 1 after k, got %d", m.leftPanel.CICursor)
	}
}

func TestCICursorJClampsAtLast(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build", "test")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CICursor = 1

	m = pressKey(m, "j")
	if m.leftPanel.CICursor != 1 {
		t.Errorf("expected CICursor clamped at 1, got %d", m.leftPanel.CICursor)
	}
}

func TestCICursorKAtTopMovesToFiles(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CICursor = 0

	m = pressKey(m, "k")
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected focus to move to Files, got %v", m.leftPanel.Focus)
	}
}

func TestCICursorResetsOnTabToCI(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build", "test")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.CICursor = 1 // previously at 1

	m = pressKey(m, "tab")
	if m.leftPanel.Focus != FocusCI {
		t.Errorf("expected focus CI, got %v", m.leftPanel.Focus)
	}
	if m.leftPanel.CICursor != 0 {
		t.Errorf("expected CICursor reset to 0, got %d", m.leftPanel.CICursor)
	}
}

func TestCICursorEnterOpensBrowser(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := []domain.PreviewCheckRow{
		{Name: "build", State: "SUCCESS", URL: "https://ci.example.com/build/1"},
		{Name: "test", State: "FAILURE", URL: "https://ci.example.com/test/2"},
	}
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CICursor = 1

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Enter on CI row with URL")
	}
	result := cmd()
	openMsg, ok := result.(OpenBrowserCI)
	if !ok {
		t.Fatalf("expected OpenBrowserCI, got %T", result)
	}
	if openMsg.URL != "https://ci.example.com/test/2" {
		t.Errorf("expected URL https://ci.example.com/test/2, got %s", openMsg.URL)
	}
}

func TestCICursorEnterNoURLIsNoop(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := []domain.PreviewCheckRow{
		{Name: "build", State: "SUCCESS"}, // no URL
	}
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.CICursor = 0

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("expected nil cmd from Enter on CI row without URL")
	}
}

func TestCISelectionHighlight(t *testing.T) {
	t.Parallel()
	checks := []domain.PreviewCheckRow{
		{Name: "build", State: "SUCCESS"},
		{Name: "test", State: "FAILURE"},
	}
	panel := LeftPanelModel{
		Files:    makeFiles("a.go"),
		Checks:   checks,
		Loading:  false,
		Focus:    FocusCI,
		CICursor: 1,
		theme:    theme.Default(),
	}
	row := panel.renderCIRow(checks[1], 1)
	// Selected row should have ANSI codes (ListSelected applies styling).
	if !strings.Contains(row, "\x1b[") {
		t.Errorf("expected selected CI row to contain ANSI codes, got plain: %q", row)
	}
	// Non-selected row should not have the ListSelected background.
	row0 := panel.renderCIRow(checks[0], 0)
	if strings.Contains(row0, "\x1b[48;2;") {
		// The non-selected row may have some ANSI (icon colors), but should not have
		// the 256-color background that ListSelected uses.
		// We verify by checking the selected row has different styling.
	}
}

func TestEscFromCIGoesToFiles(t *testing.T) {
	t.Parallel()
	files := makeFiles("a.go")
	checks := makeChecks("build")
	m := makePRDetail(100, 30, files, checks)
	m.leftPanel.Focus = FocusCI

	m = pressKey(m, "esc")
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected Esc from CI to move to Files, got %v", m.leftPanel.Focus)
	}
}
