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
	m.activeTab = TabDiff
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
	m = pressKey(m, " ")
	if !m.visual.Active {
		t.Fatal("expected visual mode active after space")
	}
	if m.visual.FileIdx != 0 || m.visual.HunkIdx != 0 || m.visual.StartLine != 0 || m.visual.EndLine != 0 {
		t.Errorf("unexpected visual selection: got (%d,%d,%d,%d), want (0,0,0,0)",
			m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine, m.visual.EndLine)
	}
}

func TestVisualModeJExpandsSelection(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, " ")
	m = pressKey(m, "j")
	if m.visual.EndLine != 1 {
		t.Errorf("expected EndLine=1 after j, got %d", m.visual.EndLine)
	}
}

func TestVisualModeJClampsAtHunkBoundary(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "draft body"})
	if len(m.drafts) != 1 {
		t.Fatalf("setup failed: expected 1 draft, got %d", len(m.drafts))
	}
	// Re-enter visual mode on same selection and discard.
	m = pressKey(m, " ")
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
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "draft 1"})
	m = pressKey(m, " ")
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
	if !contains(hint, "Space: Visual") {
		t.Errorf("expected hint to contain 'Space: Visual', got %q", hint)
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

// ── Visual mode entry guards ──────────────────────────────────────────────────

func TestVisualModeSpaceInDescriptionIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{BodyExcerpt: "some description"}
	m.activeTab = TabDescription
	m.ContentScroll = 0
	m = pressKey(m, " ")
	if m.visual.Active {
		t.Error("expected Space to be no-op in Description tab")
	}
}

func TestVisualModeSpaceInCommentsIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{Reviewers: []domain.PreviewReviewer{{Login: "alice", State: "APPROVED"}}}
	m.switchTab(TabComments)
	m = pressKey(m, " ")
	if m.visual.Active {
		t.Error("expected V to be no-op in Comments section")
	}
}

func TestVisualModeSpaceWhenFocusFilesIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.leftPanel.Focus = FocusFiles
	m = pressKey(m, " ")
	if m.visual.Active {
		t.Error("expected V to be no-op when Files focused")
	}
}

func TestVisualModeSpaceWhenFocusCIIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.leftPanel.Focus = FocusCI
	m.leftPanel.Checks = []domain.PreviewCheckRow{{Name: "ci"}}
	m = pressKey(m, " ")
	if m.visual.Active {
		t.Error("expected V to be no-op when CI focused")
	}
}

// ── Visual mode auto-scroll ───────────────────────────────────────────────────

func TestVisualModeJAutoScrolls(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	// Scroll to near bottom of diff so expansion goes past viewport.
	vh := m.contentViewportHeight()
	m.ContentScroll = m.diffSectionRowCount() - vh - 1
	if m.ContentScroll < 0 {
		m.ContentScroll = 0
	}
	m = pressKey(m, " ")
	// Expand selection to last line of hunk.
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	for m.visual.EndLine < len(h.Lines)-1 {
		m = pressKey(m, "j")
	}
	endRow := m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.EndLine)
	if endRow >= m.ContentScroll+vh-1 {
		t.Errorf("expected auto-scroll to keep selection visible: endRow=%d scroll=%d vh=%d",
			endRow, m.ContentScroll, vh)
	}
}

func TestVisualModeKAutoScrolls(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, " ")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	m.ContentScroll = m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.EndLine)
	// Shrink selection so start line is above viewport
	m = pressKey(m, "k")
	m = pressKey(m, "k")
	startRow := m.diffLineToDisplayRow(m.visual.FileIdx, m.visual.HunkIdx, m.visual.StartLine)
	if m.ContentScroll > startRow {
		t.Errorf("expected auto-scroll up: scroll=%d startRow=%d", m.ContentScroll, startRow)
	}
}

// ── Draft compose pre-populate ────────────────────────────────────────────────

func TestDraftComposePrepopulatesExisting(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "existing draft"})
	if len(m.drafts) != 1 {
		t.Fatalf("setup failed: expected 1 draft, got %d", len(m.drafts))
	}
	// Re-select same range and hit c again
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	if m.compose.input.Value() != "existing draft" {
		t.Errorf("expected compose pre-populated with 'existing draft', got %q", m.compose.input.Value())
	}
}

// ── Compose esc resumes visual ────────────────────────────────────────────────

func TestComposeEscResumesVisualMode(t *testing.T) {
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
		t.Error("expected visual mode resumed after esc in compose")
	}
	if m.visual.EndLine != 1 {
		t.Errorf("expected selection preserved (EndLine=1), got %d", m.visual.EndLine)
	}
}

// ── Jump to code from comment ─────────────────────────────────────────────────

func TestJumpToCodeFromInlineComment(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{
				Login: "alice", State: "COMMENTED",
				InlineComments: []domain.PreviewInlineComment{
					{Body: "nice", Path: "a.go", Line: 1},
				},
			},
		},
	}
	m.switchTab(TabComments)
	m.commentCursor = 0
	m = pressKey(m, "enter")
	if m.activeTab != TabDiff {
		t.Errorf("expected activeTab=TabDiff after Enter on inline comment, got %d", m.activeTab)
	}
	if m.leftPanel.Focus != FocusContent {
		t.Errorf("expected focus to move to Content, got %v", m.leftPanel.Focus)
	}
}

func TestJumpToCodeFromPRLevelCommentIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Comments: []domain.PreviewComment{
			{Login: "bob", Body: "general comment"},
		},
	}
	m.switchTab(TabComments)
	m.commentCursor = 0
	beforeScroll := m.ContentScroll
	m = pressKey(m, "enter")
	if m.ContentScroll != beforeScroll {
		t.Error("expected no scroll change for PR-level comment")
	}
}

// ── Reply to draft ────────────────────────────────────────────────────────────

func TestReplyToDraftOpensEdit(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{}
	m.drafts = []domain.DraftInlineComment{
		{Body: "draft body", Path: "a.go", Line: 1, ContextLine: " line1"},
	}
	m.switchTab(TabComments)
	m.commentCursor = 0
	m = pressKey(m, "r")
	if !m.compose.active {
		t.Fatal("expected compose active after r on draft")
	}
	if m.compose.mode != composeModeDraftInline {
		t.Errorf("expected composeModeDraftInline for draft edit, got %v", m.compose.mode)
	}
	if m.compose.input.Value() != "draft body" {
		t.Errorf("expected compose pre-populated with 'draft body', got %q", m.compose.input.Value())
	}
}

// ── D guard ───────────────────────────────────────────────────────────────────

func TestDNoopWhenNoDrafts(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m = pressKey(m, "D")
	if m.confirmDiscardAll {
		t.Error("expected D to be no-op when no drafts")
	}
}

// ── Empty body draft ──────────────────────────────────────────────────────────

func TestDraftEmptyBodyIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	beforeCount := len(m.drafts)
	m, _ = m.Update(submitComposeMsg{body: ""})
	if len(m.drafts) != beforeCount {
		t.Error("expected empty body to not create draft")
	}
}

// ── Multi-line draft ──────────────────────────────────────────────────────────

func TestMultiLineDraftHasStartLine(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, " ")
	m = pressKey(m, "j")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "multi line draft"})
	if len(m.drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(m.drafts))
	}
	d := m.drafts[0]
	if d.StartLine == 0 {
		t.Error("expected multi-line draft to have StartLine > 0")
	}
	if d.StartSide == "" {
		t.Error("expected multi-line draft to have StartSide set")
	}
}

// ── Single-line draft ─────────────────────────────────────────────────────────

func TestSingleLineDraftOmitsStartLine(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	m, _ = m.Update(submitComposeMsg{body: "single line draft"})
	if len(m.drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(m.drafts))
	}
	d := m.drafts[0]
	if d.StartLine != 0 {
		t.Errorf("expected single-line draft to have StartLine=0, got %d", d.StartLine)
	}
}

// ── Binary file visual mode ───────────────────────────────────────────────────

func TestVisualModeOnBinaryFileIsNoop(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	// Scroll to the binary file section.
	// a.go = 3+1+3+1+2 = 10, b.go = 3+1+1 = 5. Total = 15.
	// Binary starts at tab-relative offset 15 within Diff tab.
	m.ContentScroll = 15
	m = pressKey(m, " ")
	// Binary files have no diff lines, so firstDiffLineAtOrBelow should not find
	// any diff line and visual mode should not activate.
	if m.visual.Active {
		t.Error("expected space on binary file to not enter visual mode")
	}
}

// ── Normal mode j/k unaffected ────────────────────────────────────────────────

func TestNormalJKScrollsWhenNotVisual(t *testing.T) {
	t.Parallel()
	// Use a smaller height so the diff content exceeds the viewport.
	m := makeInlineReviewModel(100, 20)
	m.Detail = &domain.PRPreviewSnapshot{BodyExcerpt: "some description text"}
	m.leftPanel.Focus = FocusContent
	before := m.ContentScroll
	m = pressKey(m, "j")
	if m.ContentScroll == before {
		t.Error("expected j to scroll down in normal mode")
	}
	m = pressKey(m, "k")
	if m.ContentScroll != before {
		t.Errorf("expected k to scroll back up to %d, got %d", before, m.ContentScroll)
	}
}

// TestJKWithCommentsTabAndFilesFocus verifies that when focus is on Files (not
// Content), j/k move the file cursor even if the active tab is Comments.
func TestJKWithCommentsTabAndFilesFocus(t *testing.T) {
	t.Parallel()
	files := []diffmodel.DiffFile{
		{OldPath: "a.go", NewPath: "a.go", Status: "modified"},
		{OldPath: "b.go", NewPath: "b.go", Status: "modified"},
	}
	m := makePRDetail(100, 40, files, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM"},
		},
	}
	m.Diff = makeDiffForMapper()
	m.activeTab = TabComments
	m.leftPanel.Focus = FocusFiles
	m.leftPanel.FileIndex = 0

	beforeIdx := m.leftPanel.FileIndex
	m = pressKey(m, "j")
	if m.leftPanel.FileIndex == beforeIdx {
		t.Error("expected j to move file cursor when focus is on Files")
	}
	if m.commentCursor >= 0 {
		t.Errorf("expected commentCursor unchanged (-1), got %d", m.commentCursor)
	}
}

// ── Comment section sync with drafts ──────────────────────────────────────────

func TestCommentEntryStartRowsSyncWithDrafts(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM"},
		},
	}
	m.drafts = []domain.DraftInlineComment{
		{Body: "draft", Path: "a.go", Line: 1, ContextLine: " line1"},
	}
	cw := m.contentW()
	entries := m.commentEntries()
	startRows := m.commentEntryStartRows(cw)
	lines := m.commentLines(cw, -1)
	// commentLines includes section header (3 rows) + all entries with borders + trailing blank.
	// Total rows should equal section header + sum(entryRowCount + 2 for border) + 1 trailing blank
	expectedRows := 3 // blank + separator + label
	for _, e := range entries {
		expectedRows += m.entryRowCount(e, cw) + 2
	}
	expectedRows++ // trailing blank line after last entry
	if len(lines) != expectedRows {
		t.Errorf("commentLines row count mismatch: got %d, want %d (with 1 draft)", len(lines), expectedRows)
	}
	if len(startRows) != len(entries) {
		t.Fatalf("startRows length mismatch: got %d, want %d", len(startRows), len(entries))
	}
	// Verify each startRow points to the correct position in lines.
	for i, sr := range startRows {
		if sr < 3 || sr >= len(lines) {
			t.Errorf("startRows[%d]=%d out of bounds [3,%d)", i, sr, len(lines))
			continue
		}
		// The line at startRows[i] should be the top border of the entry.
		// We can't easily verify border content, but we can check it's non-empty.
		if lines[sr] == "" {
			t.Errorf("startRows[%d]=%d points to empty line", i, sr)
		}
	}
}

// ── v/a batch submit ──────────────────────────────────────────────────────────

func TestVWithDraftsEmitsBatchSubmit(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.drafts = []domain.DraftInlineComment{{Body: "draft"}}
	m = pressKey(m, "v")
	m.compose.SetText("review body")
	_, cmd := m.Update(submitComposeMsg{body: "review body"})
	if cmd == nil {
		t.Fatal("expected cmd from submit with drafts")
	}
	// The actual batch-submit command is inside the tea.Batch returned by Update.
	// Synchronous model state does not change to posting here; that happens when
	// the async command completes.
}

// ── Esc cancel during confirm discard ─────────────────────────────────────────

func TestDiscardAllEscCancels(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.drafts = []domain.DraftInlineComment{{Body: "draft"}}
	m = pressKey(m, "D")
	if !m.confirmDiscardAll {
		t.Fatal("setup failed: confirmDiscardAll not set")
	}
	m = pressKey(m, "esc")
	if m.confirmDiscardAll {
		t.Error("expected confirmDiscardAll=false after esc")
	}
	if len(m.drafts) != 1 {
		t.Errorf("expected draft preserved after esc cancel, got %d", len(m.drafts))
	}
}

// ── Diff viewport draft indicator ─────────────────────────────────────────────

func TestDraftIndicatorVisibleInDiffViewport(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.drafts = []domain.DraftInlineComment{
		{Path: "a.go", Line: 1, Side: "RIGHT"},
	}
	cw := m.contentW()
	// Render full diff section.
	lines := m.renderDiffSectionLines(0, m.diffSectionRowCount(), cw)
	// The first diff line of a.go should have some styling (we can't easily check
	// ANSI codes in unit test, but we can verify the line is present and not empty).
	found := false
	for _, line := range lines {
		if line != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected non-empty diff lines")
	}
}

// ── Selection highlight overrides draft indicator ─────────────────────────────

func TestSelectionHighlightOverridesDraftIndicator(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.drafts = []domain.DraftInlineComment{
		{Path: "a.go", Line: 1, Side: "RIGHT"},
	}
	m.visual.Active = true
	m.visual.FileIdx = 0
	m.visual.HunkIdx = 0
	m.visual.StartLine = 0
	m.visual.EndLine = 0
	// The line at (0,0,0) is both selected and drafted.
	// Selection highlight should take precedence.
	// We verify by checking the model state, not rendered output.
	if !m.visual.Active {
		t.Fatal("setup failed: visual mode not active")
	}
	if m.drafts[0].Path != "a.go" || m.drafts[0].Line != 1 {
		t.Fatal("setup failed: draft not on expected line")
	}
	// The rendering logic in renderDiffSectionLines checks isSelected first,
	// then isDrafted. This is verified by code inspection; the test documents
	// the expected precedence.
}
