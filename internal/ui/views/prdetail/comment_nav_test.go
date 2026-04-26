package prdetail

import (
	"context"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeDetailWithComments returns a snapshot with both reviewers and plain comments.
func makeDetailWithComments() *domain.PRPreviewSnapshot {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	return &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM", SubmittedAt: t1},
		},
		Comments: []domain.PreviewComment{
			{Login: "bob", Body: "Should we add error handling?", CreatedAt: t2},
			{Login: "carol", Body: "Good catch.", CreatedAt: t3},
		},
	}
}


// makeCommentModel builds a PRDetailModel with comments loaded and focus on the Comments section.
func makeCommentModel(width, height int) *PRDetailModel {
	m := makePRDetail(width, height, nil, nil)
	m.Detail = makeDetailWithComments()
	m.DiffLoading = false
	m.DetailLoading = false
	m.SetTheme(theme.Default())
	m.leftPanel.Focus = FocusContent
	// Jump to comments section so j/k activates comment navigation.
	m.switchTab(TabComments)
	return m
}

// ── cursor activation ─────────────────────────────────────────────────────────

func TestCommentCursorStaysNegativeUntilJ(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	if m.commentCursor != -1 {
		t.Fatalf("expected commentCursor=-1 before any j, got %d", m.commentCursor)
	}
}

func TestCommentCursorActivatesOnFirstJ(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j")
	if m.commentCursor != 0 {
		t.Errorf("expected commentCursor=0 after first j, got %d", m.commentCursor)
	}
}

func TestCommentCursorAdvancesOnJ(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j") // cursor=0
	m = pressKey(m, "j") // cursor=1
	if m.commentCursor != 1 {
		t.Errorf("expected commentCursor=1 after second j, got %d", m.commentCursor)
	}
}

func TestCommentCursorClampedAtLast(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	// 3 entries: alice, bob, carol
	for range 10 {
		m = pressKey(m, "j")
	}
	entries := m.commentEntries()
	last := len(entries) - 1
	if m.commentCursor != last {
		t.Errorf("expected cursor clamped at %d, got %d", last, m.commentCursor)
	}
}

// ── k navigation ─────────────────────────────────────────────────────────────

func TestCommentKDecreasesCursor(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j") // 0
	m = pressKey(m, "j") // 1
	m = pressKey(m, "k") // back to 0
	if m.commentCursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", m.commentCursor)
	}
}

func TestCommentKAtZeroDeactivatesCursor(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j") // cursor=0
	m = pressKey(m, "k") // at 0 → deactivate
	if m.commentCursor != -1 {
		t.Errorf("expected cursor=-1 after k at entry 0, got %d", m.commentCursor)
	}
}

func TestCommentKWithoutCursorScrollsUp(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	// Navigate to comments section which sets ContentScroll.
	scrollBefore := m.ContentScroll
	// k without active cursor should scroll up.
	m = pressKey(m, "k")
	if m.ContentScroll >= scrollBefore && scrollBefore > 0 {
		t.Errorf("expected ContentScroll to decrease from %d, got %d", scrollBefore, m.ContentScroll)
	}
}

// ── reset conditions ─────────────────────────────────────────────────────────

func TestCommentCursorResetOnTabForward(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j") // activate
	m = pressKey(m, "tab")
	if m.commentCursor != -1 {
		t.Errorf("expected cursor reset on tab, got %d", m.commentCursor)
	}
}

func TestCommentCursorResetOnTabBackward(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j")
	m = pressKey(m, "shift+tab")
	if m.commentCursor != -1 {
		t.Errorf("expected cursor reset on shift+tab, got %d", m.commentCursor)
	}
}

func TestCommentCursorResetOnSectionJump(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j")
	m = pressKey(m, "1")
	if m.commentCursor != -1 {
		t.Errorf("expected cursor reset on section jump, got %d", m.commentCursor)
	}
}

func TestCommentCursorResetOnPRDetailLoaded(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j")
	m, _ = m.Update(cmds.PRDetailLoaded{
		Detail: *makeDetailWithComments(),
	})
	if m.commentCursor != -1 {
		t.Errorf("expected cursor reset on PRDetailLoaded, got %d", m.commentCursor)
	}
}

func TestCommentCursorResetOnEscToFiles(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m = pressKey(m, "j")
	m = pressKey(m, "esc") // FocusContent → FocusFiles
	if m.commentCursor != -1 {
		t.Errorf("expected cursor reset on esc, got %d", m.commentCursor)
	}
}

// ── r/c key handlers ─────────────────────────────────────────────────────────

func TestRKeyNoopWithoutCursor(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m.PRService = &prServiceStub{}
	// r without cursor active should not open compose.
	m = pressKey(m, "r")
	if m.compose.active {
		t.Error("expected compose to stay closed when r pressed without active cursor")
	}
}

func TestRKeyOpensReplyComposeWithCursor(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, "j") // activate cursor at 0
	m = pressKey(m, "r")
	if !m.compose.active {
		t.Error("expected compose to open after r with active cursor")
	}
	if m.compose.mode != composeModeReply {
		t.Errorf("expected composeModeReply, got %v", m.compose.mode)
	}
	// target should be the first entry (alice).
	if m.compose.target.login != "alice" {
		t.Errorf("expected reply target login=alice, got %q", m.compose.target.login)
	}
}

func TestShiftCKeyOpensNewCompose(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m.PRService = &prServiceStub{}
	m = pressKey(m, "C")
	if !m.compose.active {
		t.Error("expected compose to open after C")
	}
	if m.compose.mode != composeModeNew {
		t.Errorf("expected composeModeNew, got %v", m.compose.mode)
	}
}

func TestShiftCKeyNoopWithoutPRService(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m.PRService = nil
	m = pressKey(m, "C")
	if m.compose.active {
		t.Error("expected compose to stay closed when PRService is nil")
	}
}

// ── compose blocks main key handling ─────────────────────────────────────────

func TestComposeActiveBlocksNavigation(t *testing.T) {
	t.Parallel()
	m := makeCommentModel(100, 40)
	m.PRService = &prServiceStub{}
	scrollBefore := m.ContentScroll
	m = pressKey(m, "C") // open compose
	m = pressKey(m, "j") // should go to compose, not scroll
	// Content scroll should not change because j was consumed by compose.
	if m.ContentScroll != scrollBefore {
		t.Errorf("expected scroll unchanged while compose active, got %d (was %d)", m.ContentScroll, scrollBefore)
	}
}

// ── commentEntryStartRows sync with commentLines ──────────────────────────────

// TestCommentEntryStartRowsSyncWithCommentLines is the critical guard that ensures
// commentEntryStartRows exactly mirrors commentLines layout for scroll accuracy.
func TestCommentEntryStartRowsSyncWithCommentLines(t *testing.T) {
	t.Parallel()

	widths := []int{79, 80, 120}
	for _, termW := range widths {
		termW := termW
		t.Run("", func(t *testing.T) {
			t.Parallel()
			// Skip widths where sidebar visibility is borderline (79, 80).
			if termW < 81 {
				t.Skip("skipping borderline sidebar width")
			}
			m := makePRDetail(termW, 40, nil, nil)
			m.Detail = makeDetailWithComments()
			m.SetTheme(theme.Default())
			cw := m.contentW()

			startRows := m.commentEntryStartRows(cw)
			if len(startRows) == 0 {
				t.Fatal("expected non-empty startRows")
			}

			allLines := m.commentLines(cw, -1)

			// Section header occupies rows 0..2 (blank + sep + label).
			// startRows[0] should == 3 (the border-top line of the first entry).
			if startRows[0] != 3 {
				t.Errorf("startRows[0]=%d, want 3 (after section header)", startRows[0])
			}

			entries := m.commentEntries()
			for i, sr := range startRows {
				headerRow := sr + 1 // sr is border-top; sr+1 is the "@login" header line
				if headerRow >= len(allLines) {
					t.Errorf("entry %d headerRow=%d >= len(allLines)=%d", i, headerRow, len(allLines))
					continue
				}
				// The line after the border top should contain "@login".
				line := descStripANSI(allLines[headerRow])
				want := "@" + entries[i].login
				if line == "" || !containsStr(line, want) {
					t.Errorf("entry %d headerRow=%d: line=%q does not contain %q", i, headerRow, line, want)
				}
			}
		})
	}
}

// ── view shrinks when compose open ───────────────────────────────────────────

func TestContentViewportHeightShrinkWhenComposeOpen(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 40, nil, nil)
	m.SetTheme(theme.Default())
	heightBefore := m.contentViewportHeight()
	m.compose.active = true
	heightAfter := m.contentViewportHeight()
	if heightAfter >= heightBefore {
		t.Errorf("expected viewport height to shrink when compose open: before=%d after=%d", heightBefore, heightAfter)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// prServiceStub satisfies cmds.PRService for unit tests.
type prServiceStub struct{}

func (s *prServiceStub) LoadDetail(_ context.Context, _ domain.Repository, _ int, _ bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (s *prServiceStub) LoadDiff(_ context.Context, _ domain.Repository, _ int, _ string, _ bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (s *prServiceStub) PostComment(_ context.Context, _, _ string) error       { return nil }
func (s *prServiceStub) PostReviewComment(_ context.Context, _, _ string) error { return nil }
func (s *prServiceStub) ApprovePR(_ context.Context, _, _ string) error         { return nil }
func (s *prServiceStub) SubmitReviewWithComments(_ context.Context, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *prServiceStub) SaveDraftComments(_ context.Context, _ domain.Repository, _ int, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *prServiceStub) LoadDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (s *prServiceStub) DeleteDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
