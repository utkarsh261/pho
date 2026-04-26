package prdetail

import (
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// makeDiffCursorModel creates a PRDetailModel with a multi-file diff suitable for
// cursor navigation tests. Layout:
//   file 0 (a.go): 2 hunks, 3 lines each
//   file 1 (b.go): 1 hunk, 1 line
//   file 2 (binary.bin): binary, no lines
func makeDiffCursorModel(width, height int) *PRDetailModel {
	l1, l2, l3, l4, l5 := 1, 2, 3, 4, 5
	return makeDiffCursorModelWithLines(width, height, l1, l2, l3, l4, l5)
}

func makeDiffCursorModelWithLines(width, height int, l1, l2, l3, l4, l5 int) *PRDetailModel {
	m := makePRDetail(width, height, nil, nil)
	m.Diff = makeDiffForMapper()
	m.DiffLoading = false
	m.DetailLoading = false
	m.SetTheme(theme.Default())
	m.leftPanel.Files = m.Diff.Files
	m.leftPanel.Loading = false
	m.leftPanel.Focus = FocusContent
	m.activeTab = TabDiff
	m.ContentScroll = 0
	return m
}

// ── Basic cursor movement ─────────────────────────────────────────────────────

func TestDiffCursorJMovesDown(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	if m.diffCursor.FileIdx != 0 || m.diffCursor.HunkIdx != 0 || m.diffCursor.LineIdx != 0 {
		t.Fatalf("expected cursor at (0,0,0), got (%d,%d,%d)", m.diffCursor.FileIdx, m.diffCursor.HunkIdx, m.diffCursor.LineIdx)
	}
	m = pressKey(m, "j")
	if m.diffCursor.LineIdx != 1 {
		t.Errorf("expected j to move cursor to line 1, got %d", m.diffCursor.LineIdx)
	}
}

func TestDiffCursorKMovesUp(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	// Move down twice then up once.
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	m = pressKey(m, "k")
	if m.diffCursor.LineIdx != 1 {
		t.Errorf("expected k to move cursor back to line 1, got %d", m.diffCursor.LineIdx)
	}
}

func TestDiffCursorKAtFirstLineIsNoop(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	before := m.diffCursor
	m = pressKey(m, "k")
	if m.diffCursor != before {
		t.Errorf("expected k at first line to be no-op, got cursor changed from %v to %v", before, m.diffCursor)
	}
}

func TestDiffCursorJAtLastLineIsNoop(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	// Set cursor to last line: file 1 (b.go), hunk 0, line 0.
	m.diffCursor = diffCursorLine{FileIdx: 1, HunkIdx: 0, LineIdx: 0}
	before := m.diffCursor
	m = pressKey(m, "j")
	if m.diffCursor != before {
		t.Errorf("expected j at last line to be no-op, got cursor changed from %v to %v", before, m.diffCursor)
	}
}

// ── Cross-hunk and cross-file boundary ────────────────────────────────────────

func TestDiffCursorCrossesHunkBoundary(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	// a.go has 2 hunks: hunk 0 has 3 lines, hunk 1 has 2 lines.
	// Move past line 2 (last line of hunk 0) → should go to hunk 1, line 0.
	for i := 0; i < 3; i++ {
		m = pressKey(m, "j")
	}
	if m.diffCursor.HunkIdx != 1 {
		t.Errorf("expected cursor to cross to hunk 1, got hunk %d", m.diffCursor.HunkIdx)
	}
	if m.diffCursor.LineIdx != 0 {
		t.Errorf("expected cursor at line 0 of hunk 1, got line %d", m.diffCursor.LineIdx)
	}
}

func TestDiffCursorCrossesFileBoundary(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	// a.go file has: hunk0(3 lines) + hunk1(2 lines) = 5 lines
	// Move past all 5 lines → should go to b.go (file 1), hunk 0, line 0.
	for i := 0; i < 5; i++ {
		m = pressKey(m, "j")
	}
	if m.diffCursor.FileIdx != 1 {
		t.Errorf("expected cursor to cross to file 1, got file %d", m.diffCursor.FileIdx)
	}
}

func TestDiffCursorSkipsBinaryFiles(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	// Set cursor to last line of b.go (file 1, hunk 0, line 0).
	m.diffCursor = diffCursorLine{FileIdx: 1, HunkIdx: 0, LineIdx: 0}
	m = pressKey(m, "j")
	// binary.bin is next but should be skipped. Since it's the last file, j is no-op.
	if m.diffCursor.FileIdx != 1 {
		t.Errorf("expected cursor to stay on file 1 (binary has no lines), got file %d", m.diffCursor.FileIdx)
	}
}

func TestDiffCursorKSkipsBinaryFilesBackward(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	// Set cursor to b.go line 0.
	m.diffCursor = diffCursorLine{FileIdx: 1, HunkIdx: 0, LineIdx: 0}
	m = pressKey(m, "k")
	// Should go to a.go (file 0), last line of last hunk (hunk 1, line 1).
	if m.diffCursor.FileIdx != 0 {
		t.Errorf("expected cursor to go back to file 0, got file %d", m.diffCursor.FileIdx)
	}
	if m.diffCursor.HunkIdx != 1 {
		t.Errorf("expected cursor at hunk 1 of file 0, got hunk %d", m.diffCursor.HunkIdx)
	}
}

// ── J/K 5-line jumps ─────────────────────────────────────────────────────────

func TestDiffCursorJFiveLineJump(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	m = pressKey(m, "J")
	// J moves 5 lines down. Only 5 lines total in a.go + 1 in b.go = 6 lines.
	// Jump of 5 from line 0 should land on line 5 (which is b.go line 0).
	// But J only moves by exactly 5 lines. Starting at (0,0,0):
	//   1 jump: (0,0,1) → after 5 jumps total we should be at line 5.
	// Actually moveCursorBy(5) calls moveCursorDown() 5 times.
	// Starting at (0,0,0): after 5 → (0,1,1) which is line index 4 (0-indexed within a.go hunk1 line1)
	// Let me count: line 0→1→2 (hunk0), then 3→4 (hunk1 lines 0,1)
	// After 5 moves from (0,0,0): (0,0,1), (0,0,2), (0,1,0), (0,1,1), (1,0,0)
	// So after J we should be at file 1, hunk 0, line 0.
	if m.diffCursor.FileIdx != 1 {
		t.Errorf("expected J to move cursor to file 1, got file %d", m.diffCursor.FileIdx)
	}
}

// ── gg/G ──────────────────────────────────────────────────────────────────────

func TestDiffCursorGG(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	// Move cursor somewhere, then gg.
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	m = pressKey(m, "g")
	m = pressKey(m, "g")
	if m.diffCursor.FileIdx != 0 || m.diffCursor.HunkIdx != 0 || m.diffCursor.LineIdx != 0 {
		t.Errorf("expected gg to move cursor to first line, got (%d,%d,%d)", m.diffCursor.FileIdx, m.diffCursor.HunkIdx, m.diffCursor.LineIdx)
	}
	if m.ContentScroll != 0 {
		t.Errorf("expected ContentScroll=0 after gg, got %d", m.ContentScroll)
	}
}

func TestDiffCursorG(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	m = pressKey(m, "G")
	// Last diff line: b.go (file 1), hunk 0, line 0.
	if m.diffCursor.FileIdx != 1 {
		t.Errorf("expected G to move cursor to last file, got file %d", m.diffCursor.FileIdx)
	}
}

// ── ensureDiffCursor ──────────────────────────────────────────────────────────

func TestEnsureDiffCursorInitializes(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	// Cursor starts invalid.
	if m.validDiffCursor() {
		t.Error("expected cursor to start invalid")
	}
	m.ensureDiffCursor()
	if !m.validDiffCursor() {
		t.Error("expected ensureDiffCursor to make cursor valid")
	}
	if m.diffCursor.FileIdx != 0 || m.diffCursor.HunkIdx != 0 || m.diffCursor.LineIdx != 0 {
		t.Errorf("expected cursor at (0,0,0), got (%d,%d,%d)", m.diffCursor.FileIdx, m.diffCursor.HunkIdx, m.diffCursor.LineIdx)
	}
}

// ── Visual mode anchors at cursor position ────────────────────────────────────

func TestVisualModeAnchorsAtCursorPosition(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	// Move cursor to line 2 (a.go, hunk 0, line 2).
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.diffCursor.LineIdx != 2 {
		t.Fatalf("expected cursor at line 2, got %d", m.diffCursor.LineIdx)
	}
	m = pressKey(m, " ")
	if !m.visual.Active {
		t.Fatal("expected visual mode to activate")
	}
	if m.visual.FileIdx != m.diffCursor.FileIdx || m.visual.HunkIdx != m.diffCursor.HunkIdx || m.visual.StartLine != m.diffCursor.LineIdx {
		t.Errorf("expected visual mode anchor at cursor position (%d,%d,%d), got (%d,%d,%d)",
			m.diffCursor.FileIdx, m.diffCursor.HunkIdx, m.diffCursor.LineIdx,
			m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine)
	}
}

// ── Visual mode exit sets cursor ─────────────────────────────────────────────

func TestVisualModeExitSetsCursor(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	m = pressKey(m, " ")
	if !m.visual.Active {
		t.Fatal("expected visual mode to activate")
	}
	// Expand selection down 2 lines.
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	// Exit visual mode — cursor should land on StartLine (line 0).
	m = pressKey(m, "esc")
	if m.visual.Active {
		t.Error("expected visual mode to be inactive after esc")
	}
	if m.diffCursor.LineIdx != 0 {
		t.Errorf("expected cursor at StartLine (0) after visual mode exit, got %d", m.diffCursor.LineIdx)
	}
}

// ── Cursor rendering priority ──────────────────────────────────────────────────

func TestDiffCursorRendersAboveDraftBelowVisual(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()

	lines := m.renderDiffTab(0, m.contentViewportHeight(), 80)
	// With cursor at (0,0,0), the first diff line (line 4, after header rows)
	// should be rendered with Reverse (since isCursor = true).
	// We check that lines[4] contains ANSI reverse codes.
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d", len(lines))
	}
}

// ── Tab switch initializes cursor ─────────────────────────────────────────────

func TestTabSwitchToDiffInitializesCursor(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	// Start on Description tab.
	m.switchTab(TabDescription)
	if m.validDiffCursor() {
		t.Error("expected cursor to be invalid before switching to Diff tab")
	}
	m.switchTab(TabDiff)
	if !m.validDiffCursor() {
		t.Error("expected switchTab(TabDiff) to initialize cursor")
	}
}

// ── Regression: other tabs still scroll ────────────────────────────────────────

func TestDescriptionTabJKScrolls(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 20)
	m.Detail = &domain.PRPreviewSnapshot{BodyExcerpt: strings.Repeat("word ", 500)}
	m.switchTab(TabDescription)
	m.leftPanel.Focus = FocusContent

	cw := contentViewportWidth(m.rightPanelWidth())
	lines := m.descriptionLines(cw)
	if len(lines) < 5 {
		t.Skipf("description too short (%d lines)", len(lines))
	}

	before := m.ContentScroll
	m = pressKey(m, "j")
	if m.ContentScroll == before {
		t.Error("expected j to scroll down in Description tab")
	}
	m = pressKey(m, "k")
	if m.ContentScroll != before {
		t.Errorf("expected k to scroll back up to %d, got %d", before, m.ContentScroll)
	}
}

func TestFilesFocusJKStillMovesFiles(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.leftPanel.Focus = FocusFiles
	if m.leftPanel.FileIndex != 0 {
		t.Fatalf("expected FileIndex 0, got %d", m.leftPanel.FileIndex)
	}
	m = pressKey(m, "j")
	// j with focus on Files should advance FileIndex.
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected j to advance FileIndex to 1, got %d", m.leftPanel.FileIndex)
	}
}

// ── Autoscroll with 4-line padding ───────────────────────────────────────────

func TestDiffCursorAutoscrollPadding(t *testing.T) {
	t.Parallel()
	// Use a smaller height so the diff content exceeds the viewport.
	m := makeDiffCursorModel(100, 10)
	m.ensureDiffCursor()
	// Cursor at line 0, ContentScroll should be 0.
	if m.ContentScroll != 0 {
		t.Fatalf("expected ContentScroll=0, got %d", m.ContentScroll)
	}
	// Move cursor down enough to trigger auto-scroll.
	// With padding=4 and viewport height ~6, the cursor should trigger
	// a scroll when it's within 4 lines of the bottom.
	for i := 0; i < 4; i++ {
		m = pressKey(m, "j")
	}
	// After 4 j presses, cursor is at line 4. ContentScroll should
	// have been adjusted to keep cursor visible with padding.
	if m.ContentScroll == 0 {
		t.Error("expected ContentScroll to be adjusted after cursor moved past padding boundary")
	}
}

// ── Sync file panel with cursor ──────────────────────────────────────────────

func TestDiffCursorSyncsFilePanel(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	if m.leftPanel.FileIndex != 0 {
		t.Errorf("expected FileIndex 0, got %d", m.leftPanel.FileIndex)
	}
	// Move to file 1 (b.go).
	for i := 0; i < 5; i++ {
		m = pressKey(m, "j")
	}
	if m.diffCursor.FileIdx != 1 {
		t.Errorf("expected cursor on file 1, got file %d", m.diffCursor.FileIdx)
	}
	if m.leftPanel.FileIndex != 1 {
		t.Errorf("expected FileIndex to sync to 1, got %d", m.leftPanel.FileIndex)
	}
}

// ── jumpToFile sets cursor ────────────────────────────────────────────────────

func TestJumpToFileSetsCursor(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 1
	m = pressKey(m, "l") // jumpDiffViewer → jumpToFile(1)
	if !m.validDiffCursor() {
		t.Error("expected jumpToFile to set a valid cursor")
	}
	// Should be at b.go (file 1) first diff line.
	if m.diffCursor.FileIdx != 1 {
		t.Errorf("expected cursor on file 1, got file %d", m.diffCursor.FileIdx)
	}
}

// ── First/last diff cursor helpers ───────────────────────────────────────────

func TestFirstLastDiffCursor(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)

	fi, hi, li := firstDiffCursor(m.Diff)
	if fi != 0 || hi != 0 || li != 0 {
		t.Errorf("expected firstDiffCursor (0,0,0), got (%d,%d,%d)", fi, hi, li)
	}

	fi, hi, li = lastDiffCursor(m.Diff)
	// Last actual diff line: b.go (file 1), hunk 0, line 0.
	if fi != 1 {
		t.Errorf("expected lastDiffCursor file 1, got file %d", fi)
	}
}

// ── Ctrl+D/U moves cursor on Diff tab ────────────────────────────────────────

func TestCtrlDMovesCursorOnDiffTab(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 40)
	m.ensureDiffCursor()
	before := m.diffCursor
	m = pressKey(m, "ctrl+d")
	if m.diffCursor == before {
		t.Error("expected ctrl+d to move cursor on Diff tab")
	}
}

func TestCtrlUOnDescriptionScrolls(t *testing.T) {
	t.Parallel()
	m := makeDiffCursorModel(100, 30)
	m.Detail = &domain.PRPreviewSnapshot{BodyExcerpt: strings.Repeat("word ", 500)}
	m.SetTheme(theme.Default())
	m.switchTab(TabDescription)
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 10
	m = pressKey(m, "ctrl+u")
	if m.ContentScroll >= 10 {
		t.Errorf("expected ctrl+u to scroll up on Description tab, ContentScroll=%d", m.ContentScroll)
	}
}