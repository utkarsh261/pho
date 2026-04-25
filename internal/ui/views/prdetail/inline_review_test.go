package prdetail

import (
	"testing"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// makeDiffForMapper builds a DiffModel with every edge case for the round-trip test.
func makeDiffForMapper() *diffmodel.DiffModel {
	l1, l2, l3, l4, l5 := 1, 2, 3, 4, 5
	return &diffmodel.DiffModel{
		Files: []diffmodel.DiffFile{
			{
				OldPath: "a.go", NewPath: "a.go", Status: "modified",
				Hunks: []diffmodel.DiffHunk{
					{
						Header: "@@ -1,3 +1,3 @@",
						Lines: []diffmodel.DiffLine{
							{Kind: "context", Raw: " line1", NewLine: &l1, Anchors: []diffmodel.LineAnchor{{Path: "a.go", Side: "RIGHT", Line: &l1}}},
							{Kind: "addition", Raw: "+line2", NewLine: &l2, Anchors: []diffmodel.LineAnchor{{Path: "a.go", Side: "RIGHT", Line: &l2}}},
							{Kind: "deletion", Raw: "-line3", OldLine: &l3, Anchors: []diffmodel.LineAnchor{{Path: "a.go", Side: "LEFT", Line: &l3}}},
						},
					},
					{
						Header: "@@ -10,2 +10,2 @@",
						Lines: []diffmodel.DiffLine{
							{Kind: "context", Raw: " line4", NewLine: &l4, Anchors: []diffmodel.LineAnchor{{Path: "a.go", Side: "RIGHT", Line: &l4}}},
							{Kind: "context", Raw: " line5", NewLine: &l5, Anchors: []diffmodel.LineAnchor{{Path: "a.go", Side: "RIGHT", Line: &l5}}},
						},
					},
				},
			},
			{
				OldPath: "b.go", NewPath: "b.go", Status: "modified",
				Hunks: []diffmodel.DiffHunk{
					{
						Header: "@@ -1,1 +1,1 @@",
						Lines: []diffmodel.DiffLine{
							{Kind: "addition", Raw: "+line6", NewLine: &l1, Anchors: []diffmodel.LineAnchor{{Path: "b.go", Side: "RIGHT", Line: &l1}}},
						},
					},
				},
			},
			{
				OldPath: "binary.bin", NewPath: "binary.bin", Status: "modified", IsBinary: true,
			},
		},
	}
}

func makeInlineReviewModel(width, height int) *PRDetailModel {
	m := makePRDetail(width, height, nil, nil)
	m.Diff = makeDiffForMapper()
	m.DiffLoading = false
	m.DetailLoading = false
	m.SetTheme(theme.Default())
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 0
	return m
}

// ── Round-trip mapper tests ───────────────────────────────────────────────────

func TestDiffLineRoundTrip(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)

	for fi, f := range m.Diff.Files {
		if f.IsBinary {
			continue
		}
		for hi, hunk := range f.Hunks {
			for li := range hunk.Lines {
				row := m.diffLineToDisplayRow(fi, hi, li)
				gotFI, gotHI, gotLI, found := m.firstDiffLineAtOrBelow(row)
				if !found {
					t.Errorf("firstDiffLineAtOrBelow(%d) not found for (%d,%d,%d)", row, fi, hi, li)
					continue
				}
				if gotFI != fi || gotHI != hi || gotLI != li {
					t.Errorf("round-trip failed at (%d,%d,%d): row=%d, got=(%d,%d,%d)",
						fi, hi, li, row, gotFI, gotHI, gotLI)
				}
			}
		}
	}
}

// ── Visual mode tests ─────────────────────────────────────────────────────────

func TestVisualModeEnter(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "V")
	if !m.visual.Active {
		t.Fatal("expected visual mode active after V")
	}
	if m.visual.FileIdx != 0 || m.visual.HunkIdx != 0 || m.visual.StartLine != 0 || m.visual.EndLine != 0 {
		t.Errorf("unexpected visual selection: got (%d,%d,%d,%d), want (0,0,0,0)",
			m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine, m.visual.EndLine)
	}
}

func TestVisualModeJExpandsSelection(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "V")
	m = pressKey(m, "j")
	if m.visual.EndLine != 1 {
		t.Errorf("expected EndLine=1 after j, got %d", m.visual.EndLine)
	}
}

func TestVisualModeJClampsAtHunkBoundary(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "V")
	// Hunk 0 has 3 lines (indices 0,1,2). Expand to the end.
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.visual.EndLine != 2 {
		t.Fatalf("expected EndLine=2, got %d", m.visual.EndLine)
	}
	// Try to expand past boundary — should clamp.
	m = pressKey(m, "j")
	if m.visual.EndLine != 2 {
		t.Errorf("expected EndLine still 2 at boundary, got %d", m.visual.EndLine)
	}
}

func TestVisualModeKShrinksSelection(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "V")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.visual.EndLine != 2 {
		t.Fatalf("setup failed: expected EndLine=2, got %d", m.visual.EndLine)
	}
	m = pressKey(m, "k")
	if m.visual.EndLine != 1 {
		t.Errorf("expected EndLine=1 after k, got %d", m.visual.EndLine)
	}
}

func TestVisualModeKExitsAtSingleLine(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "V")
	if !m.visual.Active {
		t.Fatal("setup failed: visual mode not active")
	}
	m = pressKey(m, "k")
	if m.visual.Active {
		t.Error("expected visual mode exited after k on single-line selection")
	}
}

func TestVisualModeEscExits(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "V")
	m = pressKey(m, "j")
	m = pressKey(m, "esc")
	if m.visual.Active {
		t.Error("expected visual mode exited after esc")
	}
}

func TestVisualModeBlocksOtherKeys(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	scrollBefore := m.ContentScroll
	m = pressKey(m, "V")
	m = pressKey(m, "o")
	if m.ContentScroll != scrollBefore {
		t.Error("expected 'o' to be no-op in visual mode")
	}
}

// ── Draft tests ───────────────────────────────────────────────────────────────

func TestDraftCreationFromVisualMode(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, "V")
	m = pressKey(m, "c")
	if !m.compose.active {
		t.Fatal("expected compose active after c in visual mode")
	}
	if m.compose.mode != composeModeDraftInline {
		t.Errorf("expected composeModeDraftInline, got %v", m.compose.mode)
	}
}

func TestDraftSaveAndReplace(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, "V")
	m = pressKey(m, "c")
	m.compose.SetText("first draft")
	// Simulate pressing Enter in compose.
	m, _ = m.Update(submitComposeMsg{body: "first draft"})
	if len(m.drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(m.drafts))
	}
	if m.drafts[0].Body != "first draft" {
		t.Errorf("expected body 'first draft', got %q", m.drafts[0].Body)
	}
	// Re-select same range and replace.
	m = pressKey(m, "V")
	m = pressKey(m, "c")
	m.compose.SetText("updated draft")
	m, _ = m.Update(submitComposeMsg{body: "updated draft"})
	if len(m.drafts) != 1 {
		t.Fatalf("expected still 1 draft after replace, got %d", len(m.drafts))
	}
	if m.drafts[0].Body != "updated draft" {
		t.Errorf("expected body 'updated draft', got %q", m.drafts[0].Body)
	}
}

func TestDraftDiscardSingle(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	// Create a draft.
	m = pressKey(m, "V")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "draft body"})
	if len(m.drafts) != 1 {
		t.Fatalf("setup failed: expected 1 draft, got %d", len(m.drafts))
	}
	// Re-enter visual mode on same selection and discard.
	m = pressKey(m, "V")
	m = pressKey(m, "d")
	if len(m.drafts) != 0 {
		t.Errorf("expected 0 drafts after discard, got %d", len(m.drafts))
	}
}

func TestDraftDiscardAll(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	// Create two drafts on different lines.
	m = pressKey(m, "V")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "draft 1"})
	m = pressKey(m, "V")
	m = pressKey(m, "j")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "draft 2"})
	if len(m.drafts) != 2 {
		t.Fatalf("setup failed: expected 2 drafts, got %d", len(m.drafts))
	}
	// Discard all.
	m = pressKey(m, "D")
	if !m.confirmDiscardAll {
		t.Fatal("expected confirmDiscardAll=true after D")
	}
	m = pressKey(m, "y")
	if len(m.drafts) != 0 {
		t.Errorf("expected 0 drafts after D+y, got %d", len(m.drafts))
	}
	if m.confirmDiscardAll {
		t.Error("expected confirmDiscardAll=false after y")
	}
}

func TestDraftDiscardAllCancel(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.drafts = []domain.DraftInlineComment{{Body: "draft"}}
	m = pressKey(m, "D")
	m = pressKey(m, "n")
	if len(m.drafts) != 1 {
		t.Errorf("expected 1 draft after cancel, got %d", len(m.drafts))
	}
	if m.confirmDiscardAll {
		t.Error("expected confirmDiscardAll=false after n")
	}
}

// ── v/a branching tests ───────────────────────────────────────────────────────

func TestVNoDraftsOpensReviewComment(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, "v")
	if !m.compose.active {
		t.Fatal("expected compose active after v")
	}
	if m.compose.mode != composeModeReviewComment {
		t.Errorf("expected composeModeReviewComment, got %v", m.compose.mode)
	}
}

func TestVWithDraftsStillOpensReviewComment(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.drafts = []domain.DraftInlineComment{{Body: "draft"}}
	m = pressKey(m, "v")
	if !m.compose.active {
		t.Fatal("expected compose active after v with drafts")
	}
	if m.compose.mode != composeModeReviewComment {
		t.Errorf("expected composeModeReviewComment, got %v", m.compose.mode)
	}
}

func TestANoDraftsOpensApprove(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, "a")
	if !m.compose.active {
		t.Fatal("expected compose active after a")
	}
	if m.compose.mode != composeModeApprove {
		t.Errorf("expected composeModeApprove, got %v", m.compose.mode)
	}
}

// ── Comment entry code context tests ──────────────────────────────────────────

func TestCommentEntryWithCodeContext(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{
				Login: "alice", State: "COMMENTED",
				InlineComments: []domain.PreviewInlineComment{
					{Login: "alice", Body: "nice code", Path: "a.go", Line: 1},
				},
			},
		},
	}
	entries := m.commentEntries()
	if len(entries) == 0 {
		t.Fatal("expected entries, got none")
	}
	found := false
	for _, e := range entries {
		if e.path == "a.go" && e.line == 1 {
			found = true
			if e.contextLine == "" {
				t.Error("expected non-empty contextLine for inline comment")
			}
			break
		}
	}
	if !found {
		t.Error("expected entry with path=a.go, line=1")
	}
}

func TestDraftCommentEntry(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{}
	m.drafts = []domain.DraftInlineComment{
		{Body: "draft comment", Path: "a.go", Line: 1, ContextLine: " line1"},
	}
	entries := m.commentEntries()
	if len(entries) == 0 {
		t.Fatal("expected entries, got none")
	}
	if entries[0].login != "[DRAFT]" {
		t.Errorf("expected [DRAFT] login, got %q", entries[0].login)
	}
	if entries[0].contextLine != " line1" {
		t.Errorf("expected contextLine ' line1', got %q", entries[0].contextLine)
	}
}

// ── Status hint tests ─────────────────────────────────────────────────────────

func TestStatusHintNormal(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	hint := m.StatusHint()
	if hint == "" {
		t.Error("expected non-empty hint")
	}
	if !contains(hint, "V: Visual") {
		t.Errorf("expected hint to contain 'V: Visual', got %q", hint)
	}
}

func TestStatusHintWithDrafts(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.drafts = []domain.DraftInlineComment{{Body: "draft"}}
	hint := m.StatusHint()
	if !contains(hint, "D: Discard all drafts") {
		t.Errorf("expected hint to contain 'D: Discard all drafts', got %q", hint)
	}
}

func TestStatusHintVisualMode(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.visual.Active = true
	hint := m.StatusHint()
	if !contains(hint, "j/k: Select lines") {
		t.Errorf("expected hint to contain 'j/k: Select lines', got %q", hint)
	}
}

func TestStatusHintConfirmDiscard(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.drafts = []domain.DraftInlineComment{{Body: "draft"}}
	m.confirmDiscardAll = true
	hint := m.StatusHint()
	if !contains(hint, "(y/n)") {
		t.Errorf("expected hint to contain '(y/n)', got %q", hint)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
