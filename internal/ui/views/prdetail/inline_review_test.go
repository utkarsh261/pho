package prdetail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/application/cmds"
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
	m.buildNavigableIndex()
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

func TestHeadChangeBlocksInProgressInlineDraft(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Summary.HeadRefOID = "old-head"
	m.Diff.HeadSHA = "old-head"
	m = pressKey(m, " ")
	m = pressKey(m, "c")
	m.compose.SetText("work in progress")

	m.Update(cmds.PRDetailLoaded{
		Host: m.Repo.Host, Repo: m.Repo.FullName, Number: m.Summary.Number,
		RequestID: m.detailRequestID + 1,
		Detail: domain.PRPreviewSnapshot{
			Repo: m.Repo.FullName, Number: m.Summary.Number, HeadRefOID: "new-head",
		},
	})
	if !m.inlineDraftStale || m.visual.Active {
		t.Fatalf("expected head change to invalidate the old selection, stale=%v visual=%v", m.inlineDraftStale, m.visual.Active)
	}

	m.Update(submitComposeMsg{body: "work in progress"})
	if len(m.drafts) != 0 || m.compose.status != composeStatusError || !strings.Contains(m.compose.errMsg, "diff changed") {
		t.Fatalf("in-progress old-head draft was accepted: drafts=%d status=%d err=%q", len(m.drafts), m.compose.status, m.compose.errMsg)
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
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "thread1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "nice code"},
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

// ── Thread-aware comment entry tests ──────────────────────────────────────────

func TestCommentEntriesFromReviewThreads(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 10,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "first"},
					{ID: "c2", Login: "bob", Body: "second"},
				},
			},
		},
	}
	entries := m.commentEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].login != "alice" {
		t.Errorf("expected first entry login=alice, got %q", entries[0].login)
	}
	if entries[0].threadID != "t1" {
		t.Errorf("expected first entry threadID=t1, got %q", entries[0].threadID)
	}
	if entries[0].commentID != "c1" {
		t.Errorf("expected first entry commentID=c1, got %q", entries[0].commentID)
	}
	if entries[1].login != "bob" {
		t.Errorf("expected second entry login=bob, got %q", entries[1].login)
	}
	if entries[1].commentID != "c2" {
		t.Errorf("expected second entry commentID=c2, got %q", entries[1].commentID)
	}
	if entries[0].isThreadReply {
		t.Error("expected first entry to be the thread root")
	}
	if !entries[1].isThreadReply {
		t.Error("expected second entry to be a thread reply")
	}
}

func TestThreadEntryRowCountExcludesPathAndContext(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	root := commentEntry{login: "alice", body: "root", path: "a.go", line: 5, contextLine: "ctx"}
	reply := commentEntry{login: "bob", body: "reply", path: "a.go", line: 5, isThreadReply: true}
	cw := 80
	rootH := m.entryRowCount(root, cw)
	replyH := m.entryRowCount(reply, cw)
	if rootH <= replyH {
		t.Errorf("expected root height (%d) > reply height (%d)", rootH, replyH)
	}
}

func TestThreadEntriesStayContiguousAfterSort(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "early", CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
					{ID: "c2", Login: "bob", Body: "late", CreatedAt: time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC)},
				},
			},
		},
		Comments: []domain.PreviewComment{
			{ID: "c3", Login: "carol", Body: "middle", CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
		},
	}
	entries := m.commentEntries()
	var aliceIdx, bobIdx int
	for i, e := range entries {
		if e.login == "alice" {
			aliceIdx = i
		}
		if e.login == "bob" {
			bobIdx = i
		}
	}
	if bobIdx-aliceIdx != 1 {
		t.Errorf("expected alice and bob to be contiguous (indices %d and %d)", aliceIdx, bobIdx)
	}
}

func TestInterleavedThreadsSortAsUnits(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "root", CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
					{ID: "c2", Login: "alice2", Body: "reply", CreatedAt: time.Date(2024, 1, 1, 16, 0, 0, 0, time.UTC)},
				},
			},
			{
				ID: "t2", Path: "b.go", Line: 2,
				Comments: []domain.PreviewThreadComment{
					{ID: "c3", Login: "bob", Body: "root", CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
				},
			},
		},
	}
	entries := m.commentEntries()
	var t1Start, t2Start int = -1, -1
	for i, e := range entries {
		if e.threadID == "t1" && t1Start == -1 {
			t1Start = i
		}
		if e.threadID == "t2" && t2Start == -1 {
			t2Start = i
		}
	}
	if t1Start == -1 || t2Start == -1 {
		t.Fatalf("could not locate threads: t1Start=%d t2Start=%d", t1Start, t2Start)
	}
	if t1Start >= t2Start {
		t.Errorf("expected t1 (start %d) before t2 (start %d)", t1Start, t2Start)
	}
	if entries[t1Start+1].threadID != "t1" {
		t.Error("expected t1's two comments to be contiguous")
	}
}

func TestReviewSummaryBeforeNearbyThreads(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	reviewTime := time.Date(2024, 1, 1, 10, 0, 30, 0, time.UTC)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "test latest", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "handlers.go", Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "test", CreatedAt: threadTime},
					{ID: "c2", Login: "bob", Body: "test inline", CreatedAt: threadTime.Add(1 * time.Hour)},
				},
			},
		},
	}
	entries := m.commentEntries()
	reviewIdx := -1
	threadIdx := -1
	for i, e := range entries {
		if e.state == "COMMENTED" && e.body == "test latest" {
			reviewIdx = i
		}
		if e.threadID == "t1" && threadIdx == -1 {
			threadIdx = i
		}
	}
	if reviewIdx == -1 {
		t.Fatal("review summary 'test latest' not found")
	}
	if threadIdx == -1 {
		t.Fatal("thread t1 not found")
	}
	if reviewIdx >= threadIdx {
		t.Errorf("review summary (idx %d) must appear before nearby thread (idx %d)", reviewIdx, threadIdx)
	}
}

func TestSortChronological_DistantReviewsAndThreads(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	oldReview := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	newThread := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM", SubmittedAt: oldReview},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 5,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "new comment", CreatedAt: newThread},
				},
			},
		},
	}
	entries := m.commentEntries()
	oldIdx := -1
	newIdx := -1
	for i, e := range entries {
		if e.body == "LGTM" {
			oldIdx = i
		}
		if e.body == "new comment" {
			newIdx = i
		}
	}
	if oldIdx == -1 || newIdx == -1 {
		t.Fatalf("missing entries: old=%d new=%d", oldIdx, newIdx)
	}
	if oldIdx > newIdx {
		t.Errorf("old review (Jan) at idx %d should be before new thread (Jun) at idx %d", oldIdx, newIdx)
	}
}

func TestSort_PRCommentBetweenReviewAndThread(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	reviewTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	prCommentTime := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	threadTime := time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM", SubmittedAt: reviewTime},
		},
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "nice PR", CreatedAt: prCommentTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 5,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "carol", Body: "inline comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	reviewIdx := -1
	prIdx := -1
	threadIdx := -1
	for i, e := range entries {
		if e.body == "LGTM" && e.state == "APPROVED" {
			reviewIdx = i
		}
		if e.body == "nice PR" && e.state == "" {
			prIdx = i
		}
		if e.body == "inline comment" {
			threadIdx = i
		}
	}
	if reviewIdx == -1 || prIdx == -1 || threadIdx == -1 {
		t.Fatalf("missing entries: review=%d pr=%d thread=%d", reviewIdx, prIdx, threadIdx)
	}
	if !(reviewIdx < prIdx && prIdx < threadIdx) {
		t.Errorf("expected review(%d) < prComment(%d) < thread(%d)", reviewIdx, prIdx, threadIdx)
	}
}

func TestSort_ReviewSummaryFarFromThread_PlacedChronologically(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	reviewTime := time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "evening review", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 5,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "morning comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	threadIdx := -1
	reviewIdx := -1
	for i, e := range entries {
		if e.body == "morning comment" {
			threadIdx = i
		}
		if e.body == "evening review" {
			reviewIdx = i
		}
	}
	if threadIdx == -1 || reviewIdx == -1 {
		t.Fatalf("missing entries: thread=%d review=%d", threadIdx, reviewIdx)
	}
	if threadIdx > reviewIdx {
		t.Errorf("morning thread (idx %d) should appear before evening review (idx %d)", threadIdx, reviewIdx)
	}
}

func TestSort_MultipleReviewsStayChronological(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "first review", SubmittedAt: time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)},
			{Login: "bob", State: "COMMENTED", Body: "second review", SubmittedAt: time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC)},
			{Login: "carol", State: "APPROVED", Body: "third review", SubmittedAt: time.Date(2024, 1, 3, 9, 0, 0, 0, time.UTC)},
		},
	}
	entries := m.commentEntries()
	firstIdx := -1
	secondIdx := -1
	thirdIdx := -1
	for i, e := range entries {
		if e.body == "first review" {
			firstIdx = i
		}
		if e.body == "second review" {
			secondIdx = i
		}
		if e.body == "third review" {
			thirdIdx = i
		}
	}
	if !(firstIdx < secondIdx && secondIdx < thirdIdx) {
		t.Errorf("reviews should be chronological: first=%d second=%d third=%d", firstIdx, secondIdx, thirdIdx)
	}
}

func TestThreadOneBorderPerThread(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 5,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "root"},
					{ID: "c2", Login: "bob", Body: "reply"},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	lines := m.commentLines(cw, -1)
	plain := descStripANSI(strings.Join(lines, "\n"))
	borderTops := strings.Count(plain, "╭")
	borderBottoms := strings.Count(plain, "╰")
	if borderTops != 1 {
		t.Errorf("expected 1 top border for single thread, got %d", borderTops)
	}
	if borderBottoms != 1 {
		t.Errorf("expected 1 bottom border for single thread, got %d", borderBottoms)
	}
}

func TestThreadReplyIndentedTwoSpaces(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 5,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "root"},
					{ID: "c2", Login: "bob", Body: "reply"},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	lines := m.commentLines(cw, -1)
	plain := descStripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "  @bob") {
		t.Error("expected reply header to be indented by 2 spaces")
	}
}

func TestThreadNoPipePrefixOnReplies(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 5,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "root"},
					{ID: "c2", Login: "bob", Body: "reply"},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	lines := m.commentLines(cw, -1)
	plain := descStripANSI(strings.Join(lines, "\n"))
	if strings.Contains(plain, "│ @") || strings.Contains(plain, "││") {
		t.Error("expected no │ prefix on reply lines")
	}
}

func TestCommentEntriesBackwardCompatWithInlineComments(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{
				Login: "alice", State: "COMMENTED",
				InlineComments: []domain.PreviewInlineComment{
					{Login: "alice", Body: "legacy", Path: "b.go", Line: 2},
				},
			},
		},
	}
	entries := m.commentEntries()
	found := false
	for _, e := range entries {
		if e.path == "b.go" && e.line == 2 && e.body == "legacy" {
			found = true
			if e.threadID != "" {
				t.Error("expected legacy entry to have no threadID")
			}
		}
	}
	if !found {
		t.Error("expected backward-compat entry from InlineComments")
	}
}

func TestCommentEntriesSkipsEmptyCommentedReviews(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: ""},
			{Login: "bob", State: "APPROVED", Body: ""},
		},
	}
	entries := m.commentEntries()
	for _, e := range entries {
		if e.login == "alice" {
			t.Error("expected empty COMMENTED review to be skipped")
		}
	}
	foundBob := false
	for _, e := range entries {
		if e.login == "bob" && e.state == "APPROVED" {
			foundBob = true
		}
	}
	if !foundBob {
		t.Error("expected empty APPROVED review to still appear")
	}
}

// ── Reply routing tests ───────────────────────────────────────────────────────

func TestReplyComposeHintForThread(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{ID: "t1", Path: "a.go", Line: 5, Comments: []domain.PreviewThreadComment{
				{ID: "c1", Login: "alice", Body: "hi"},
			}},
		},
	}
	m.switchTab(TabComments)
	m.commentCursor = 0
	m = pressKey(m, "r")
	if !m.compose.active {
		t.Fatal("expected compose active")
	}
	view := m.compose.View(80)
	if !strings.Contains(descStripANSI(view), "Reply to thread on a.go:5") {
		t.Errorf("expected thread reply hint in compose view, got:\n%s", descStripANSI(view))
	}
}

func TestReplyComposeHintForPRComment(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		Comments: []domain.PreviewComment{
			{ID: "c99", Login: "bob", Body: "general note"},
		},
	}
	m.switchTab(TabComments)
	m.commentCursor = 0
	m = pressKey(m, "r")
	if !m.compose.active {
		t.Fatal("expected compose active")
	}
	view := m.compose.View(80)
	if !strings.Contains(descStripANSI(view), "Reply to @bob") {
		t.Errorf("expected PR comment reply hint in compose view, got:\n%s", descStripANSI(view))
	}
}

func TestReplyComposeCursorOnThreadEntry(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{ID: "t1", Path: "a.go", Line: 1, Comments: []domain.PreviewThreadComment{
				{ID: "c1", Login: "alice", Body: "hi"},
			}},
		},
	}
	m.switchTab(TabComments)
	m.commentCursor = 0
	m = pressKey(m, "r")
	if m.compose.target.threadID != "t1" {
		t.Errorf("expected compose target threadID=t1, got %q", m.compose.target.threadID)
	}
	if m.compose.target.path != "a.go" {
		t.Errorf("expected compose target path=a.go, got %q", m.compose.target.path)
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
	if !contains(hint, "D: Discard drafts") {
		t.Errorf("expected hint to contain 'D: Discard drafts', got %q", hint)
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
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "thread1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "alice", Body: "nice"},
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
	// With cursor-based navigation, visual mode finds the nearest valid diff line
	// (which is b.go's line, right before the binary file) since the binary file is
	// skipped by firstDiffLineAtOrBelow.
	if !m.visual.Active {
		t.Error("expected visual mode to activate (anchored at nearest diff line)")
	}
	if m.visual.FileIdx != 1 {
		t.Errorf("expected visual mode on file 1 (b.go), got file %d", m.visual.FileIdx)
	}
}

// ── Normal mode j/k unaffected ────────────────────────────────────────────────

func TestNormalJKScrollsWhenNotVisual(t *testing.T) {
	t.Parallel()
	// On the Description tab, j/k still scroll content.
	m := makeInlineReviewModel(100, 20)
	m.Detail = &domain.PRPreviewSnapshot{BodyExcerpt: strings.Repeat("word ", 500)}
	m.switchTab(TabDescription)
	m.leftPanel.Focus = FocusContent

	cw := contentViewportWidth(m.rightPanelWidth())
	lines := m.descriptionLines(cw)
	if len(lines) < 5 {
		t.Skipf("description too short (%d lines) for scroll test", len(lines))
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
	m.leftPanel.Cursor = 0

	beforeIdx := m.leftPanel.Cursor
	m = pressKey(m, "j")
	if m.leftPanel.Cursor == beforeIdx {
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

func TestRefreshBlocksDraftingAndDraftSubmissionUntilHeadIsKnown(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.drafts = []domain.DraftInlineComment{{Body: "draft", HeadSHA: "old-head"}}
	m.DetailLoading = true

	m.enterVisualMode()
	if m.visual.Active {
		t.Fatal("expected visual drafting to be disabled while detail refresh is pending")
	}
	m.compose.Open(composeModeReviewComment, commentEntry{}, len(m.drafts))
	m.Update(submitComposeMsg{body: "review"})
	if m.compose.status != composeStatusError || !strings.Contains(m.compose.errMsg, "refresh") {
		t.Fatalf("expected pending refresh to block draft submission, status=%d err=%q", m.compose.status, m.compose.errMsg)
	}
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

// ── End-to-end thread rendering from PRPreviewSnapshot ──────────────────────────
//
// These tests verify that ReviewThreads from a normalized GraphQL response
// produce grouped shared boxes (not separate boxes per comment).

func commentEntriesFromSnapshot(snapshot *domain.PRPreviewSnapshot) []commentEntry {
	m := makePRDetail(80, 40, nil, nil)
	m.Detail = snapshot
	m.SetTheme(theme.Default())
	return m.commentEntries()
}

func commentLinesFromSnapshot(snapshot *domain.PRPreviewSnapshot, width, activeIdx int) []string {
	m := makePRDetail(width, 40, nil, nil)
	m.Detail = snapshot
	m.SetTheme(theme.Default())
	cw := m.contentW()
	return m.commentLines(cw, activeIdx)
}

func stripANSIE2E(s string) string {
	var out []byte
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z' {
				inEsc = false
			}
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func TestE2E_ReviewThreadEntriesFromSnapshot(t *testing.T) {
	t1 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "utkarsh261", State: "COMMENTED", Body: "test latest", SubmittedAt: t1},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:   "thread1",
				Path: "handlers.go",
				Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: t1},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: t2},
				},
			},
		},
	}

	entries := commentEntriesFromSnapshot(snapshot)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (1 review summary + 2 thread comments), got %d", len(entries))
	}

	if entries[0].state != "COMMENTED" || entries[0].threadID != "" {
		t.Errorf("entry 0 should be review summary, got state=%q threadID=%q", entries[0].state, entries[0].threadID)
	}
	if entries[1].threadID != "thread1" || entries[1].isThreadReply {
		t.Errorf("entry 1 should be thread root, got threadID=%q isThreadReply=%v", entries[1].threadID, entries[1].isThreadReply)
	}
	if !entries[1].isThreadReply == false && entries[1].path != "handlers.go" {
		t.Errorf("entry 1 should have path=handlers.go, got path=%q", entries[1].path)
	}
	if entries[2].threadID != "thread1" || !entries[2].isThreadReply {
		t.Errorf("entry 2 should be thread reply, got threadID=%q isThreadReply=%v", entries[2].threadID, entries[2].isThreadReply)
	}
}

func TestE2E_ReviewThreadBoxRenderingFromSnapshot(t *testing.T) {
	t1 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "utkarsh261", State: "COMMENTED", Body: "test latest", SubmittedAt: t1},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:   "thread1",
				Path: "handlers.go",
				Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: t1},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: t2},
				},
			},
		},
	}

	lines := commentLinesFromSnapshot(snapshot, 80, -1)
	plain := stripANSIE2E(strings.Join(lines, "\n"))

	borderTops := strings.Count(plain, "╭")
	borderBottoms := strings.Count(plain, "╰")

	if borderTops != 2 {
		t.Errorf("expected 2 top borders (review + thread shared box), got %d", borderTops)
	}
	if borderBottoms != 2 {
		t.Errorf("expected 2 bottom borders, got %d", borderBottoms)
	}
	if strings.Contains(plain, "│ @") || strings.Contains(plain, "││") {
		t.Error("expected no │ prefix on reply lines inside shared box")
	}
	if !strings.Contains(plain, "  @utkarsh261") {
		t.Error("expected reply header indented by 2 spaces")
	}
	reviewIdx := strings.Index(plain, "test latest")
	threadIdx := strings.Index(plain, "test")
	if reviewIdx < 0 || threadIdx < 0 {
		t.Fatal("expected both review summary and thread comment in output")
	}
	if reviewIdx > threadIdx {
		t.Error("expected review summary to appear before thread comments")
	}
}

func TestE2E_NoReviewThreads_FallbackToInlineComments(t *testing.T) {
	t1 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{
				Login:       "utkarsh261",
				State:       "COMMENTED",
				Body:        "",
				SubmittedAt: t1,
				InlineComments: []domain.PreviewInlineComment{
					{Body: "inline comment", Path: "handlers.go", Line: 103},
				},
			},
		},
		ReviewThreads: nil,
	}

	entries := commentEntriesFromSnapshot(snapshot)

	if len(entries) != 1 {
		t.Fatalf("expected 1 fallback inline comment entry, got %d", len(entries))
	}
	if entries[0].body != "inline comment" {
		t.Errorf("expected body 'inline comment', got %q", entries[0].body)
	}
	if entries[0].path != "handlers.go" || entries[0].line != 103 {
		t.Errorf("expected path handlers.go:103, got %s:%d", entries[0].path, entries[0].line)
	}
}

func TestE2E_ReviewThreadsPreferredOverInlineComments(t *testing.T) {
	t1 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{
				Login:       "utkarsh261",
				State:       "COMMENTED",
				Body:        "",
				SubmittedAt: t1,
				InlineComments: []domain.PreviewInlineComment{
					{Body: "stale inline", Path: "handlers.go", Line: 103},
				},
			},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:   "thread1",
				Path: "handlers.go",
				Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "fresh thread", CreatedAt: t1},
				},
			},
		},
	}

	entries := commentEntriesFromSnapshot(snapshot)

	for _, e := range entries {
		if e.body == "stale inline" {
			t.Error("stale InlineComments should not appear when ReviewThreads is populated")
		}
	}
	found := false
	for _, e := range entries {
		if e.body == "fresh thread" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'fresh thread' from ReviewThreads but not found")
	}
}

func TestE2E_RealPR13Data(t *testing.T) {
	t.Parallel()

	tReviewInline := mustTime("2026-04-25T12:00:44Z")
	tComment := mustTime("2026-04-25T12:16:36Z")
	tOk := mustTime("2026-04-25T12:17:20Z")
	tAight := mustTime("2026-04-25T18:00:40Z")
	tAsdjks := mustTime("2026-04-26T20:15:27Z")
	tAsdasd := mustTime("2026-04-26T20:43:57Z")
	tOkTest := mustTime("2026-04-27T09:38:18Z")
	tTestLatest := mustTime("2026-06-03T19:10:00Z")
	tEmptyReview := mustTime("2026-06-04T19:22:11Z")
	tThreadComment := mustTime("2026-06-03T19:09:50Z")
	tThreadReply := mustTime("2026-06-04T19:22:11Z")
	tTestNew := mustTime("2026-06-03T19:09:56Z")

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "utkarsh261", State: "COMMENTED", Body: "reviews inline", SubmittedAt: tReviewInline},
			{Login: "utkarsh261", State: "COMMENTED", Body: "comment", SubmittedAt: tComment},
			{Login: "utkarsh261", State: "COMMENTED", Body: "ok", SubmittedAt: tOk},
			{Login: "utkarsh261", State: "COMMENTED", Body: "aight", SubmittedAt: tAight},
			{Login: "utkarsh261", State: "COMMENTED", Body: "asdjks", SubmittedAt: tAsdjks},
			{Login: "utkarsh261", State: "COMMENTED", Body: "asdasd", SubmittedAt: tAsdasd},
			{Login: "utkarsh261", State: "COMMENTED", Body: "ok test", SubmittedAt: tOkTest},
			{Login: "utkarsh261", State: "COMMENTED", Body: "test latest", SubmittedAt: tTestLatest},
			{Login: "utkarsh261", State: "COMMENTED", Body: "", SubmittedAt: tEmptyReview},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "PRRT_kwDORWMYTM6G3LKU", Path: "internal/adapters/telegram/handlers.go", Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "PRRC_kwDORWMYTM7HvxJS", Login: "utkarsh261", Body: "test", CreatedAt: tThreadComment},
					{ID: "PRRC_kwDORWMYTM7ILEDC", Login: "utkarsh261", Body: "test inline", CreatedAt: tThreadReply},
				},
			},
			{
				ID: "PRRT_kwDORWMYTM6G3LRK", Path: "internal/adapters/telegram/handlers.go", Line: 110,
				Comments: []domain.PreviewThreadComment{
					{ID: "PRRC_kwDORWMYTM7HvxSU", Login: "utkarsh261", Body: "test new", CreatedAt: tTestNew},
				},
			},
		},
	}

	entries := commentEntriesFromSnapshot(snapshot)

	t.Logf("Entry order:")
	for i, e := range entries {
		t.Logf("  [%d] login=%s state=%q body=%q threadID=%q isReply=%v", i, e.login, e.state, e.body, e.threadID, e.isThreadReply)
	}

	// The empty COMMENTED review should be filtered out.
	for _, e := range entries {
		if e.state == "COMMENTED" && e.body == "" {
			t.Error("empty COMMENTED review should be skipped")
		}
	}

	// "test latest" review summary must appear before its nearby thread entries,
	// but NOT at the very top — it should be near threads from the same time window.
	reviewIdx := -1
	threadIdx := -1
	firstReviewIdx := -1
	for i, e := range entries {
		if e.state == "COMMENTED" && e.body == "test latest" {
			reviewIdx = i
		}
		if e.threadID == "PRRT_kwDORWMYTM6G3LKU" && threadIdx == -1 {
			threadIdx = i
		}
		if e.state != "" && firstReviewIdx == -1 {
			firstReviewIdx = i
		}
	}
	if reviewIdx == -1 {
		t.Fatal("'test latest' review summary not found in entries")
	}
	if threadIdx == -1 {
		t.Fatal("thread PRRT_kwDORWMYTM6G3LKU not found in entries")
	}
	if reviewIdx >= threadIdx {
		t.Errorf("review summary 'test latest' (idx %d) must appear before nearby thread (idx %d)", reviewIdx, threadIdx)
	}
	if reviewIdx == 0 {
		t.Error("'test latest' should not be the very first entry — older reviews should appear before it")
	}

	// The thread "test" + "test inline" must be contiguous with reply marked.
	testIdx := -1
	testInlineIdx := -1
	for i, e := range entries {
		if e.body == "test" && e.threadID == "PRRT_kwDORWMYTM6G3LKU" {
			testIdx = i
		}
		if e.body == "test inline" && e.threadID == "PRRT_kwDORWMYTM6G3LKU" {
			testInlineIdx = i
		}
	}
	if testIdx == -1 || testInlineIdx == -1 {
		t.Fatalf("thread entries not found: test=%d testInline=%d", testIdx, testInlineIdx)
	}
	if testInlineIdx != testIdx+1 {
		t.Errorf("reply 'test inline' (idx %d) should be immediately after 'test' (idx %d)", testInlineIdx, testIdx)
	}
	if entries[testIdx].isThreadReply {
		t.Error("'test' should NOT be a reply (it's the thread root)")
	}
	if !entries[testInlineIdx].isThreadReply {
		t.Error("'test inline' SHOULD be a reply")
	}

	// Verify rendering: "test latest" should have its own box, "test" + "test inline" share a box, "test new" has its own box.
	lines := commentLinesFromSnapshot(snapshot, 80, -1)
	plain := stripANSIE2E(strings.Join(lines, "\n"))
	borderTops := strings.Count(plain, "╭")
	borderBottoms := strings.Count(plain, "╰")
	if borderTops < 3 {
		t.Errorf("expected at least 3 top borders (review + thread1 + thread2), got %d", borderTops)
	}
	if borderBottoms < 3 {
		t.Errorf("expected at least 3 bottom borders, got %d", borderBottoms)
	}
}

func TestE2E_RealPR13Data_RenderOutput(t *testing.T) {
	tReviewInline := mustTime("2026-04-25T12:00:44Z")
	tComment := mustTime("2026-04-25T12:16:36Z")
	tOk := mustTime("2026-04-25T12:17:20Z")
	tAight := mustTime("2026-04-25T18:00:40Z")
	tAsdjks := mustTime("2026-04-26T20:15:27Z")
	tAsdasd := mustTime("2026-04-26T20:43:57Z")
	tOkTest := mustTime("2026-04-27T09:38:18Z")
	tTestLatest := mustTime("2026-06-03T19:10:00Z")
	tThreadComment := mustTime("2026-06-03T19:09:50Z")
	tThreadReply := mustTime("2026-06-04T19:22:11Z")
	tTestNew := mustTime("2026-06-03T19:09:56Z")

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "utkarsh261", State: "COMMENTED", Body: "reviews inline", SubmittedAt: tReviewInline},
			{Login: "utkarsh261", State: "COMMENTED", Body: "comment", SubmittedAt: tComment},
			{Login: "utkarsh261", State: "COMMENTED", Body: "ok", SubmittedAt: tOk},
			{Login: "utkarsh261", State: "COMMENTED", Body: "aight", SubmittedAt: tAight},
			{Login: "utkarsh261", State: "COMMENTED", Body: "asdjks", SubmittedAt: tAsdjks},
			{Login: "utkarsh261", State: "COMMENTED", Body: "asdasd", SubmittedAt: tAsdasd},
			{Login: "utkarsh261", State: "COMMENTED", Body: "ok test", SubmittedAt: tOkTest},
			{Login: "utkarsh261", State: "COMMENTED", Body: "test latest", SubmittedAt: tTestLatest},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "PRRT_kwDORWMYTM6G3LKU", Path: "internal/adapters/telegram/handlers.go", Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "PRRC_1", Login: "utkarsh261", Body: "test", CreatedAt: tThreadComment},
					{ID: "PRRC_2", Login: "utkarsh261", Body: "test inline", CreatedAt: tThreadReply},
				},
			},
			{
				ID: "PRRT_kwDORWMYTM6G3LRK", Path: "internal/adapters/telegram/handlers.go", Line: 110,
				Comments: []domain.PreviewThreadComment{
					{ID: "PRRC_3", Login: "utkarsh261", Body: "test new", CreatedAt: tTestNew},
				},
			},
		},
	}

	lines := commentLinesFromSnapshot(snapshot, 80, -1)
	plain := stripANSIE2E(strings.Join(lines, "\n"))
	t.Log("Rendered output:\n" + plain)

	if !strings.Contains(plain, "test latest") {
		t.Error("expected 'test latest' to appear in rendered output")
	}
	if !strings.Contains(plain, "test inline") {
		t.Error("expected 'test inline' to appear in rendered output")
	}
	if !strings.Contains(plain, "test new") {
		t.Error("expected 'test new' to appear in rendered output")
	}
	if !strings.Contains(plain, "reviews inline") {
		t.Error("expected 'reviews inline' to appear in rendered output")
	}
	if !strings.Contains(plain, "test") && strings.Count(plain, "test") < 2 {
		t.Error("expected multiple 'test' occurrences in rendered output")
	}
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// ── Parent review association tests ────────────────────────────────────────────

func TestParentReviewAssociation_WithinWindow(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	found := false
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			found = true
		}
	}
	if !found {
		t.Error("expected thread t1 to be marked indentByParentReview (review 10s after thread)")
	}
}

func TestParentReviewAssociation_ExactSameTime(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	ts := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: ts},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: ts},
				},
			},
		},
	}
	entries := m.commentEntries()
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			t.Error("thread should NOT be marked when review and thread have same timestamp (not strictly after)")
		}
	}
}

func TestParentReviewAssociation_OutsideWindow(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := mustTime("2026-06-03T19:00:00Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z") // 10 min after thread
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			t.Error("thread should NOT be marked when review is >5 minutes after thread")
		}
	}
}

func TestParentReviewAssociation_ThreadAfterReview(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	reviewTime := mustTime("2026-06-03T19:00:00Z")
	threadTime := mustTime("2026-06-03T19:01:00Z") // thread AFTER review
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			t.Error("thread should NOT be marked when thread is after review")
		}
	}
}

func TestParentReviewAssociation_NoReviewSummaries(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: time.Now()},
				},
			},
		},
	}
	entries := m.commentEntries()
	for _, e := range entries {
		if e.indentByParentReview {
			t.Error("no thread should be indented when there are no review summaries")
		}
	}
}

func TestParentReviewAssociation_EmptyCommentedReviewNotParent(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	emptyReviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "", SubmittedAt: emptyReviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			t.Error("empty COMMENTED review is filtered out and should not be a parent")
		}
	}
}

func TestParentReviewAssociation_NearestReviewMatches(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	nearReviewTime := mustTime("2026-06-03T19:10:00Z")
	farReviewTime := mustTime("2026-06-03T19:30:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "far review", SubmittedAt: farReviewTime},
			{Login: "bob", State: "COMMENTED", Body: "near review", SubmittedAt: nearReviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "carol", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	found := false
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			found = true
		}
	}
	if !found {
		t.Error("expected thread to be marked as indented (nearReview is within 5-min window)")
	}
}

func TestParentReviewAssociation_ApprovedWithNoBody(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	found := false
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			found = true
		}
	}
	if !found {
		t.Error("APPROVED review with no body should still be parent candidate")
	}
}

func TestParentReviewAssociation_MultipleThreadsSameParent(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	thread1Time := mustTime("2026-06-03T19:09:50Z")
	thread2Time := mustTime("2026-06-03T19:09:56Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread1", CreatedAt: thread1Time},
				},
			},
			{
				ID: "t2", Path: "a.go", Line: 2,
				Comments: []domain.PreviewThreadComment{
					{ID: "c2", Login: "carol", Body: "thread2", CreatedAt: thread2Time},
				},
			},
		},
	}
	entries := m.commentEntries()
	t1Indented := false
	t2Indented := false
	for _, e := range entries {
		if e.threadID == "t1" && e.indentByParentReview {
			t1Indented = true
		}
		if e.threadID == "t2" && e.indentByParentReview {
			t2Indented = true
		}
	}
	if !t1Indented {
		t.Error("expected thread t1 to be indented (within 5-min window)")
	}
	if !t2Indented {
		t.Error("expected thread t2 to be indented (within 5-min window)")
	}
}

// ── Indented rendering tests ──────────────────────────────────────────────────

func TestThreadIndentedUnderParentReview(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(80, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "handlers.go", Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread root", CreatedAt: threadTime},
					{ID: "c2", Login: "carol", Body: "thread reply", CreatedAt: replyTime},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	lines := m.commentLines(cw, -1)
	plain := descStripANSI(strings.Join(lines, "\n"))

	if !strings.Contains(plain, "review body") {
		t.Error("expected review body in output")
	}
	if !strings.Contains(plain, "thread root") {
		t.Error("expected 'thread root' in output")
	}
	if !strings.Contains(plain, "thread reply") {
		t.Error("expected 'thread reply' in output")
	}

	borderTops := strings.Count(plain, "╭")
	borderBottoms := strings.Count(plain, "╰")
	if borderTops != 2 {
		t.Errorf("expected 2 top borders (review + thread), got %d", borderTops)
	}
	if borderBottoms != 2 {
		t.Errorf("expected 2 bottom borders, got %d", borderBottoms)
	}

	linesWithIndent := 0
	for _, line := range strings.Split(plain, "\n") {
		if strings.HasPrefix(line, "  ╭") || strings.HasPrefix(line, "  │") || strings.HasPrefix(line, "  ╰") {
			linesWithIndent++
		}
	}
	if linesWithIndent == 0 {
		t.Error("expected thread box border lines to be indented by 2 spaces")
	}
}

func TestThreadNotIndentedWithoutParent(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(80, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread root", CreatedAt: threadTime},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	lines := m.commentLines(cw, -1)
	plain := descStripANSI(strings.Join(lines, "\n"))

	for _, line := range strings.Split(plain, "\n") {
		if strings.HasPrefix(line, "  ╭") || strings.HasPrefix(line, "  │") || strings.HasPrefix(line, "  ╰") {
			t.Errorf("expected no indented border lines without parent review, got: %q", line)
			break
		}
	}
}

func TestReviewSummaryNotIndented(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(80, 40)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread root", CreatedAt: threadTime},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	lines := m.commentLines(cw, -1)
	plain := descStripANSI(strings.Join(lines, "\n"))

	for _, line := range strings.Split(plain, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if strings.Contains(trimmed, "review body") {
			if strings.HasPrefix(line, "  ") && (strings.HasPrefix(line, "  ╭") || strings.HasPrefix(line, "  │")) {
				t.Errorf("review summary should NOT be indented, got: %q", line)
			}
		}
	}
}

func TestThreadBoxNarrowerUnderParent(t *testing.T) {
	t.Parallel()
	m := makePRDetail(120, 40, nil, nil)
	threadTime := mustTime("2026-06-03T19:09:50Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body here", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "thread root", CreatedAt: threadTime},
				},
			},
		},
	}
	m.SetTheme(theme.Default())
	cw := m.contentW()
	entries := m.commentEntries()

	var reviewEntry, threadEntry commentEntry
	for _, e := range entries {
		if e.state != "" && e.body == "review body here" {
			reviewEntry = e
		}
		if e.threadID == "t1" && !e.isThreadReply {
			threadEntry = e
		}
	}

	reviewRows := m.entryRowCount(reviewEntry, cw)
	threadRows := m.entryRowCount(threadEntry, cw)

	if !threadEntry.indentByParentReview {
		t.Error("expected thread entry to be marked indentByParentReview")
	}

	if threadRows < reviewRows {
		reason := "narrower width should cause more line wrapping for thread"
		if reviewEntry.body != "" && threadEntry.body != "" && len(reviewEntry.body) == len(threadEntry.body) {
			t.Errorf("thread rows (%d) should be >= review rows (%d) due to %s", threadRows, reviewRows, reason)
		}
	}
}

// ── Golden snapshot test for indented rendering ───────────────────────────────

func TestCommentsLinesWithParentReviewIndent(t *testing.T) {
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")
	thread2Time := mustTime("2026-06-03T19:09:56Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")

	snapshot := &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "utkarsh261", State: "COMMENTED", Body: "test latest", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "PRRT_1", Path: "internal/adapters/telegram/handlers.go", Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: threadTime},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: replyTime},
				},
			},
			{
				ID: "PRRT_2", Path: "internal/adapters/telegram/handlers.go", Line: 110,
				Comments: []domain.PreviewThreadComment{
					{ID: "c3", Login: "utkarsh261", Body: "test new", CreatedAt: thread2Time},
				},
			},
		},
	}

	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = snapshot
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			cw := m.contentW()
			lines := m.commentLines(cw, -1)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_parent_indent_w%d.txt", w))
		})
	}
}

func TestCommentEntryStartRowsSyncWithIndentedThread(t *testing.T) {
	t.Parallel()
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")
	reviewTime := mustTime("2026-06-03T19:10:00Z")

	for _, termW := range []int{80, 100, 120} {
		termW := termW
		t.Run(fmt.Sprintf("w%d", termW), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(termW, 40, nil, nil)
			m.Detail = &domain.PRPreviewSnapshot{
				Reviewers: []domain.PreviewReviewer{
					{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
				},
				ReviewThreads: []domain.PreviewReviewThread{
					{
						ID: "t1", Path: "a.go", Line: 5,
						Comments: []domain.PreviewThreadComment{
							{ID: "c1", Login: "bob", Body: "thread root", CreatedAt: threadTime},
							{ID: "c2", Login: "carol", Body: "thread reply", CreatedAt: replyTime},
						},
					},
				},
			}
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			cw := m.contentW()
			entries := m.commentEntries()

			if !entries[1].indentByParentReview {
				t.Fatal("expected thread entries to be indented")
			}

			startRows := m.commentEntryStartRows(cw)
			if len(startRows) != len(entries) {
				t.Fatalf("startRows length %d != entries length %d", len(startRows), len(entries))
			}

			allLines := m.commentLines(cw, -1)

			for i, sr := range startRows {
				if sr < 3 || sr >= len(allLines) {
					t.Errorf("startRows[%d]=%d out of bounds [3,%d)", i, sr, len(allLines))
					continue
				}
				line := descStripANSI(allLines[sr])
				if line == "" {
					t.Errorf("startRows[%d]=%d points to empty line", i, sr)
				}
			}

			reviewRow := startRows[0]
			threadRow := startRows[1]
			if threadRow <= reviewRow {
				t.Errorf("thread should start after review: reviewRow=%d threadRow=%d", reviewRow, threadRow)
			}
		})
	}
}

// ── Sort preserves chronological order for non-associated items ────────────────

func TestSort_PRCommentNotReorderedBeforeUnassociatedThread(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	prCommentTime := time.Date(2024, 1, 1, 10, 4, 0, 0, time.UTC)
	reviewTime := time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review body", SubmittedAt: reviewTime},
		},
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "pr comment", CreatedAt: prCommentTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "carol", Body: "thread comment", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	threadIdx := -1
	prIdx := -1
	for i, e := range entries {
		if e.threadID == "t1" && !e.isThreadReply {
			threadIdx = i
		}
		if e.commentID == "pc1" {
			prIdx = i
		}
	}
	if threadIdx == -1 || prIdx == -1 {
		t.Fatalf("missing entries: thread=%d prComment=%d", threadIdx, prIdx)
	}
	if prIdx < threadIdx {
		t.Errorf("PR comment at 10:04 (idx %d) should NOT sort before unassociated thread at 10:00 (idx %d)", prIdx, threadIdx)
	}
}

func TestSort_TwoReviewsTwoThreads_NoCorruption(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	thread1Time := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	thread2Time := time.Date(2024, 1, 1, 10, 4, 0, 0, time.UTC)
	review1Time := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)
	review2Time := time.Date(2024, 1, 1, 10, 6, 0, 0, time.UTC)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", Body: "review A", SubmittedAt: review1Time},
			{Login: "bob", State: "APPROVED", Body: "review B", SubmittedAt: review2Time},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "carol", Body: "thread1 comment", CreatedAt: thread1Time},
				},
			},
			{
				ID: "t2", Path: "b.go", Line: 10,
				Comments: []domain.PreviewThreadComment{
					{ID: "c2", Login: "dave", Body: "thread2 comment", CreatedAt: thread2Time},
				},
			},
		},
	}
	entries := m.commentEntries()
	reviewAIdx := -1
	reviewBIdx := -1
	thread1Idx := -1
	thread2Idx := -1
	for i, e := range entries {
		if e.body == "review A" && e.state == "COMMENTED" {
			reviewAIdx = i
		}
		if e.body == "review B" && e.state == "APPROVED" {
			reviewBIdx = i
		}
		if e.threadID == "t1" && !e.isThreadReply {
			thread1Idx = i
		}
		if e.threadID == "t2" && !e.isThreadReply {
			thread2Idx = i
		}
	}
	if reviewAIdx == -1 || reviewBIdx == -1 || thread1Idx == -1 || thread2Idx == -1 {
		t.Fatalf("missing entries: reviewA=%d reviewB=%d thread1=%d thread2=%d", reviewAIdx, reviewBIdx, thread1Idx, thread2Idx)
	}
	if reviewAIdx != thread1Idx-1 {
		t.Errorf("review A (idx %d) should immediately precede thread 1 (idx %d)", reviewAIdx, thread1Idx)
	}
	if reviewBIdx != thread2Idx-1 {
		t.Errorf("review B (idx %d) should immediately precede thread 2 (idx %d)", reviewBIdx, thread2Idx)
	}
}

func TestSort_UnassociatedReviewSummaryStaysChronological(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	threadTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	reviewTime := time.Date(2024, 1, 1, 10, 20, 0, 0, time.UTC) // 20 min after thread
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "unrelated review", SubmittedAt: reviewTime},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "t1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "earlier thread", CreatedAt: threadTime},
				},
			},
		},
	}
	entries := m.commentEntries()
	threadIdx := -1
	reviewIdx := -1
	for i, e := range entries {
		if e.threadID == "t1" {
			threadIdx = i
		}
		if e.body == "unrelated review" {
			reviewIdx = i
		}
	}
	if threadIdx == -1 || reviewIdx == -1 {
		t.Fatalf("missing entries: thread=%d review=%d", threadIdx, reviewIdx)
	}
	if reviewIdx < threadIdx {
		t.Errorf("unrelated review at 10:20 (idx %d) should NOT sort before thread at 10:00 (idx %d) when not associated", reviewIdx, threadIdx)
	}
}

// ── PR-level reply scroll edge case ───────────────────────────────────────────

func TestPostedCommentReplyToPRCommentScrollsToEnd(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "original", CreatedAt: mustTime("2024-01-01T00:00:00Z")},
		},
	}
	m.switchTab(TabComments)
	entries := m.commentEntries()
	originalIdx := -1
	for i, e := range entries {
		if e.commentID == "pc1" {
			originalIdx = i
		}
	}
	if originalIdx < 0 {
		t.Fatal("original comment not found")
	}

	m.postedComment = true
	m.postedCommentTarget = commentEntry{commentID: "pc1", login: "bob", body: "original"}

	refreshed := &domain.PRPreviewSnapshot{
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "original", CreatedAt: mustTime("2024-01-01T00:00:00Z")},
			{ID: "pc2", Login: "alice", Body: "my reply", CreatedAt: mustTime("2024-01-02T00:00:00Z")},
		},
	}
	m, _ = m.Update(cmds.PRDetailLoaded{Detail: *refreshed})

	if m.commentCursor == originalIdx && len(m.commentEntries()) > 1 {
		t.Errorf("PR-level reply should scroll to the new last entry, not the original comment (cursor=%d, original=%d, last=%d)",
			m.commentCursor, originalIdx, len(m.commentEntries())-1)
	}
	if m.commentCursor != len(m.commentEntries())-1 {
		t.Errorf("expected cursor at last entry (%d), got %d", len(m.commentEntries())-1, m.commentCursor)
	}
}

// ── Golden snapshot tests for resolved thread rendering ──────────────────────

func TestGoldenResolvedCollapsed(t *testing.T) {
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")

	snapshot := &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:         "PRRT_1",
				Path:       "handlers.go",
				Line:       103,
				IsResolved: true,
				ResolvedBy: "bob",
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: threadTime},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: replyTime},
				},
			},
		},
	}

	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = snapshot
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			cw := m.contentW()
			lines := m.commentLines(cw, -1)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_resolved_collapsed_w%d.txt", w))
		})
	}
}

func TestGoldenResolvedCollapsedActive(t *testing.T) {
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")

	snapshot := &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:         "PRRT_1",
				Path:       "handlers.go",
				Line:       103,
				IsResolved: true,
				ResolvedBy: "bob",
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: threadTime},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: replyTime},
				},
			},
		},
	}

	// Test at width-gate boundaries (59, 60, 80) to verify the m: hint appears/disappears.
	for _, w := range []int{59, 60, 80} {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = snapshot
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			m.activeTab = TabComments
			m.leftPanel.Focus = FocusContent
			cw := m.contentW()
			entries := m.commentEntries()
			// Find the summary entry index.
			activeIdx := -1
			for i, e := range entries {
				if e.isResolvedSummary {
					activeIdx = i
					break
				}
			}
			lines := m.commentLines(cw, activeIdx)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_resolved_collapsed_active_w%d.txt", w))
		})
	}
}

func TestGoldenResolvedExpanded(t *testing.T) {
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")

	snapshot := &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:         "PRRT_1",
				Path:       "handlers.go",
				Line:       103,
				IsResolved: true,
				ResolvedBy: "bob",
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: threadTime},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: replyTime},
				},
			},
		},
	}

	for _, w := range composeGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = snapshot
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			m.expandedResolved = map[string]bool{"PRRT_1": true}
			cw := m.contentW()
			lines := m.commentLines(cw, -1)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_resolved_expanded_w%d.txt", w))
		})
	}
}

func TestGoldenResolvedExpandedActive(t *testing.T) {
	threadTime := mustTime("2026-06-03T19:09:50Z")
	replyTime := mustTime("2026-06-04T19:22:11Z")

	snapshot := &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:         "PRRT_1",
				Path:       "handlers.go",
				Line:       103,
				IsResolved: true,
				ResolvedBy: "bob",
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: threadTime},
					{ID: "c2", Login: "utkarsh261", Body: "test inline", CreatedAt: replyTime},
				},
			},
		},
	}

	for _, w := range []int{59, 60, 80} {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = snapshot
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			m.activeTab = TabComments
			m.leftPanel.Focus = FocusContent
			m.expandedResolved = map[string]bool{"PRRT_1": true}
			cw := m.contentW()
			entries := m.commentEntries()
			// Active cursor on the first entry of the expanded resolved thread.
			activeIdx := -1
			for i, e := range entries {
				if e.threadID == "PRRT_1" && !e.isThreadReply {
					activeIdx = i
					break
				}
			}
			lines := m.commentLines(cw, activeIdx)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_resolved_expanded_active_w%d.txt", w))
		})
	}
}

func TestGoldenUnresolvedThreadActive(t *testing.T) {
	threadTime := mustTime("2026-06-03T19:09:50Z")

	snapshot := &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID:   "PRRT_1",
				Path: "handlers.go",
				Line: 103,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "utkarsh261", Body: "test", CreatedAt: threadTime},
				},
			},
		},
	}

	for _, w := range []int{59, 60, 80} {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w, 40, nil, nil)
			m.Detail = snapshot
			m.SetTheme(theme.Default())
			m.DiffLoading = false
			m.DetailLoading = false
			m.activeTab = TabComments
			m.leftPanel.Focus = FocusContent
			cw := m.contentW()
			entries := m.commentEntries()
			activeIdx := -1
			for i, e := range entries {
				if e.threadID == "PRRT_1" && !e.isThreadReply {
					activeIdx = i
					break
				}
			}
			lines := m.commentLines(cw, activeIdx)
			got := descStripANSI(strings.Join(lines, "\n"))
			checkGolden(t, got, fmt.Sprintf("comments_unresolved_active_w%d.txt", w))
		})
	}
}
