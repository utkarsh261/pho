package prdetail

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// These tests verify behavior that is fixed or improved by the refactor.
// Tests with t.Skip are enabled phase-by-phase.

// ── Phase 1: Bounds guards ────────────────────────────────────────────────────

func TestVisualModeSurvivesDiffShrinking(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, " ") // enter visual mode at first diff line
	if !m.visual.Active {
		t.Fatal("setup failed: visual mode not active")
	}
	// Manually push visual state to indices that will be stale after shrinking.
	m.visual.FileIdx = 2 // will be out of bounds after shrinking to 1 file
	m.visual.HunkIdx = 1 // will be out of bounds after shrinking to 1 hunk
	m.visual.EndLine = 2 // will be out of bounds after shrinking to 1 line
	// Shrink the diff: replace with a single file, single hunk, single line.
	m.Diff = makeDiffForMapper()
	m.Diff.Files = m.Diff.Files[:1]
	m.Diff.Files[0].Hunks = m.Diff.Files[0].Hunks[:1]
	m.Diff.Files[0].Hunks[0].Lines = m.Diff.Files[0].Hunks[0].Lines[:1]
	// These must not panic even though visual indices are now stale.
	m = pressKey(m, "j")
	m = pressKey(m, "k")
	m = pressKey(m, "esc")
	if m.visual.Active {
		t.Error("expected visual mode exited after esc")
	}
}

func TestExpandVisualSelectionDownGuardsMissingFile(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.visual.Active = true
	m.visual.FileIdx = 99 // out of bounds
	m.visual.HunkIdx = 0
	m.visual.StartLine = 0
	m.visual.EndLine = 0
	// Must not panic.
	m.expandVisualSelectionDown()
}

func TestShrinkVisualSelectionUpGuardsMissingFile(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.visual.Active = true
	m.visual.FileIdx = 99 // out of bounds
	m.visual.HunkIdx = 0
	m.visual.StartLine = 0
	m.visual.EndLine = 1 // trigger the diffLineToDisplayRow path
	// Must not panic.
	m.shrinkVisualSelectionUp()
}

func TestBuildDraftGuardsMissingFile(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.visual.Active = true
	m.visual.FileIdx = 99
	m.visual.HunkIdx = 0
	m.visual.StartLine = 0
	m.visual.EndLine = 0
	draft := m.buildDraftFromVisualSelection("body")
	if draft.ID != "" {
		t.Error("expected empty draft when file index is stale")
	}
}

func TestDraftOverlapsGuardsMissingFile(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.visual.Active = true
	m.visual.FileIdx = 99
	m.visual.HunkIdx = 0
	m.visual.StartLine = 0
	m.visual.EndLine = 0
	// Must not panic.
	_ = m.draftOverlapsSelection()
}

func TestFindDraftGuardsMissingFile(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.visual.Active = true
	m.visual.FileIdx = 99
	m.visual.HunkIdx = 0
	m.visual.StartLine = 0
	m.visual.EndLine = 0
	// Must not panic.
	_ = m.findDraftForSelection()
}

func TestRemoveDraftAtOutOfBounds(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	if m.removeDraftAt(0, 0, 99, 99) {
		t.Error("expected false for out-of-bounds lines")
	}
}

// ── Phase 2: Diff line lookup index ───────────────────────────────────────────

func TestLookupDiffLineMissing(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	if got := m.lookupDiffLine("missing.go", 999); got != "" {
		t.Errorf("expected empty string for missing line, got %q", got)
	}
}

// ── Phase 3: Cache commentEntries ─────────────────────────────────────────────

func TestCommentEntriesReturnsEquivalentResults(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	a := m.commentEntries()
	b := m.commentEntries()
	if len(a) != len(b) {
		t.Fatalf("commentEntries returned different lengths: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].login != b[i].login || a[i].body != b[i].body {
			t.Errorf("commentEntries[%d] differ: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestCommentEntriesIsCached(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	a := m.commentEntries()
	b := m.commentEntries()
	if &a[0] != &b[0] {
		t.Error("expected commentEntries to return the same cached slice")
	}
}

// ── Phase 5: ComposeClosedMsg ─────────────────────────────────────────────────

func TestComposeEscEmitsClosedMsg(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	if !m.compose.active {
		t.Fatal("setup failed: compose not active")
	}
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.compose.Update(msg)
	if cmd == nil {
		t.Fatal("expected cmd from compose esc handler")
	}
	// Execute the cmd and check the emitted message.
	emitted := cmd()
	_, ok := emitted.(composeClosedMsg)
	if !ok {
		t.Fatalf("expected composeClosedMsg, got %T", emitted)
	}
}

func TestComposeEscResumesVisualModeNoFlicker(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, " ")
	m = pressKey(m, "j")
	m = pressKey(m, "c")
	if !m.compose.active {
		t.Fatal("setup failed: compose not active")
	}
	m = pressKey(m, "esc")
	if m.compose.active {
		t.Error("expected compose closed after esc")
	}
	if !m.visual.Active {
		t.Error("expected visual mode still active after esc closes draft-inline compose")
	}
	if m.visual.EndLine != 1 {
		t.Errorf("expected selection preserved (EndLine=1), got %d", m.visual.EndLine)
	}
}
