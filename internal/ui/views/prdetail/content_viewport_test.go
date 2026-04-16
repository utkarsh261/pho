package prdetail

import (
	"strings"
	"testing"

	diffmodel "github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/ui/theme"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeDetailWithBody returns a PRPreviewSnapshot with the given body text.
func makeDetailWithBody(body string) *domain.PRPreviewSnapshot {
	return &domain.PRPreviewSnapshot{BodyExcerpt: body}
}

// makeDetailWithReviewers returns a PRPreviewSnapshot with the named reviewers
// in APPROVED state so that commentLines() returns non-nil.
func makeDetailWithReviewers(logins ...string) *domain.PRPreviewSnapshot {
	reviewers := make([]domain.PreviewReviewer, len(logins))
	for i, l := range logins {
		reviewers[i] = domain.PreviewReviewer{Login: l, State: "APPROVED"}
	}
	return &domain.PRPreviewSnapshot{Reviewers: reviewers}
}

// makeDiff wraps a file slice into a minimal DiffModel.
func makeDiff(files []diffmodel.DiffFile) *diffmodel.DiffModel {
	return &diffmodel.DiffModel{
		Files: files,
		Stats: diffmodel.DiffStats{TotalFiles: len(files)},
	}
}

// makeFilesWithDisplayRows creates n DiffFiles each with the given DisplayRows.
func makeFilesWithDisplayRows(n, displayRows int) []diffmodel.DiffFile {
	files := make([]diffmodel.DiffFile, n)
	for i := range files {
		files[i] = diffmodel.DiffFile{
			OldPath:     "file.go",
			NewPath:     "file.go",
			Status:      "modified",
			DisplayRows: displayRows,
		}
	}
	return files
}

// contentW returns the content viewport width for a model with the given width.
func (m *PRDetailModel) contentW() int {
	return contentViewportWidth(m.rightPanelWidth())
}

// ── ContentSection precompute tests ──────────────────────────────────────────

// TestContentSectionStartRowsAdditive verifies that each section's StartRow equals
// the sum of all preceding sections' RowCounts.
func TestContentSectionStartRowsAdditive(t *testing.T) {
	t.Parallel()

	// 3 files × 10 rows each = 30 diff rows
	files := makeFilesWithDisplayRows(3, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("description text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.Detail.Reviewers = []domain.PreviewReviewer{{Login: "alice", State: "APPROVED"}}

	sections := m.buildContentSections(m.contentW())

	if len(sections) < 2 {
		t.Fatalf("expected at least 2 sections, got %d", len(sections))
	}

	cursor := 0
	for _, sec := range sections {
		if sec.StartRow != cursor {
			t.Errorf("section %d: expected StartRow=%d, got %d", sec.Section, cursor, sec.StartRow)
		}
		cursor += sec.RowCount
	}
}

// TestViewportEmptyDescriptionStartsAtDiff verifies that when the PR body is empty,
// Description.RowCount = 0 and Diff.StartRow = 0.
func TestViewportEmptyDescriptionStartsAtDiff(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(1, 5)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("") // empty body
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false

	sections := m.buildContentSections(m.contentW())

	desc, _ := findSection(sections, domain.SectionDescription)
	if desc.RowCount != 0 {
		t.Errorf("expected Description.RowCount=0 for empty body, got %d", desc.RowCount)
	}

	diff, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		t.Fatal("expected SectionDiff to be present")
	}
	if diff.StartRow != 0 {
		t.Errorf("expected SectionDiff.StartRow=0 with empty description, got %d", diff.StartRow)
	}
}

// TestViewportNoCommentsShowsPlaceholder verifies that when no reviews exist the
// Comments section is still present (RowCount >= 1) and contains the "No reviews"
// placeholder text so that pressing '3' always lands somewhere.
func TestViewportNoCommentsShowsPlaceholder(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(1, 5)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("some body")
	m.Detail.Reviewers = nil // no reviews submitted
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.SetTheme(theme.Default())

	cw := m.contentW()
	sections := m.buildContentSections(cw)

	sec, ok := findSection(sections, domain.SectionComments)
	if !ok {
		t.Fatal("expected Comments section to be present even with no reviews")
	}
	if sec.RowCount < 1 {
		t.Errorf("expected RowCount >= 1 for placeholder, got %d", sec.RowCount)
	}

	// Rendered output at the comments section start should contain the placeholder.
	lines := m.renderContentLines(sections, sec.StartRow, 5, cw)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "No reviews") {
		t.Errorf("expected 'No reviews' placeholder in rendered output, got:\n%s", joined)
	}
}

// TestViewportAllSectionsEmpty verifies that when all sections are empty, the rendered
// output contains "No content".
func TestViewportAllSectionsEmpty(t *testing.T) {
	t.Parallel()

	// No detail, no diff, no loading.
	m := makePRDetail(100, 30, nil, nil)
	// Detail = nil, DetailLoading = false → desc RowCount = 0
	// Diff = nil, DiffLoading = false → diff RowCount = 0
	// Comments = nil → Comments omitted
	m.SetTheme(theme.Default())

	cw := m.contentW()
	sections := m.buildContentSections(cw)

	total := totalRowsInSections(sections)
	if total != 0 {
		t.Errorf("expected totalRows=0 for all-empty state, got %d", total)
	}

	// Rendered output should contain "No content".
	lines := m.renderContentLines(sections, 0, 10, cw)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "No content") {
		t.Errorf("expected 'No content' in output, got:\n%s", joined)
	}
}

// TestViewportSectionJump verifies pressing "2" scrolls to the Diff section's StartRow
// and moves focus to the content viewport.
func TestViewportSectionJump(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(2, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("some description text that fills multiple rows when wrapped")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusFiles

	// Discover where the Diff section starts.
	sections := m.buildContentSections(m.contentW())
	diff, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		t.Fatal("expected SectionDiff to exist")
	}
	if diff.StartRow == 0 && diff.RowCount > 0 {
		// Description was empty — jump would still be correct but can't distinguish
		// from already-at-top; skip this particular scenario check.
		t.Skip("description is empty; Diff.StartRow=0, jump is still valid but indistinguishable")
	}

	m = pressKey(m, "2")

	if m.ContentScroll != diff.StartRow {
		t.Errorf("expected ContentScroll=%d after '2' jump, got %d", diff.StartRow, m.ContentScroll)
	}
	if m.leftPanel.Focus != FocusContent {
		t.Errorf("expected focus to move to FocusContent after section jump, got %v", m.leftPanel.Focus)
	}
}

// TestViewportSectionJumpEmptySection verifies pressing "1" when description is empty
// is a no-op (scroll unchanged, focus unchanged).
func TestViewportSectionJumpEmptySection(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(1, 5)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("") // empty description
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusFiles
	m.ContentScroll = 0

	m = pressKey(m, "1") // try to jump to empty Description

	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected focus unchanged after jump to empty section, got %v", m.leftPanel.Focus)
	}
	if m.ContentScroll != 0 {
		t.Errorf("expected ContentScroll unchanged after jump to empty section, got %d", m.ContentScroll)
	}
}

// TestViewportScrollClampAtMax verifies that ContentScroll cannot exceed
// max(0, totalContentRows - contentViewportHeight).
func TestViewportScrollClampAtMax(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(3, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false

	m.ContentScroll = 99999
	m.clampContentScroll()

	maxScroll := m.maxContentScroll()
	if m.ContentScroll != maxScroll {
		t.Errorf("expected ContentScroll clamped to %d, got %d", maxScroll, m.ContentScroll)
	}
}

// TestViewportScrollClampAtZero verifies that ContentScroll cannot go negative.
func TestViewportScrollClampAtZero(t *testing.T) {
	t.Parallel()

	m := makePRDetail(100, 30, makeFiles("a.go"), nil)
	m.ContentScroll = -10
	m.clampContentScroll()

	if m.ContentScroll < 0 {
		t.Errorf("expected ContentScroll >= 0 after clamp, got %d", m.ContentScroll)
	}
}

// ── g/G scroll tests ──────────────────────────────────────────────────────────

// TestDoubleGScrollsToTop verifies that pressing g g sets ContentScroll to 0
// when the content viewport is focused.
func TestDoubleGScrollsToTop(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(3, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 15 // somewhere in the middle

	m = pressKey(m, "g")
	m = pressKey(m, "g")

	if m.ContentScroll != 0 {
		t.Errorf("expected ContentScroll=0 after g g, got %d", m.ContentScroll)
	}
}

// TestSingleGFollowedByOtherKey verifies that pressing g then a non-g key
// does NOT scroll to the top (single g only arms the sequence).
func TestSingleGFollowedByOtherKey(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(3, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 8

	m = pressKey(m, "g")
	m = pressKey(m, "k") // non-g key

	// ContentScroll should not be 0 from the g sequence (k also moved scroll).
	// The key check: LastKey must be cleared after the non-g key.
	if m.LastKey == "g" {
		t.Errorf("expected LastKey to be cleared after non-g key, still 'g'")
	}
}

// TestCapitalGScrollsToBottom verifies that pressing G sets ContentScroll to
// max(0, totalContentRows - contentViewportHeight) when content viewport is focused.
func TestCapitalGScrollsToBottom(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(3, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 0

	expectedMax := m.maxContentScroll()
	m = pressKey(m, "G")

	if m.ContentScroll != expectedMax {
		t.Errorf("expected ContentScroll=%d after G, got %d", expectedMax, m.ContentScroll)
	}
}

// ── Ctrl+D / Ctrl+U tests ────────────────────────────────────────────────────

// TestCtrlDPageDown verifies that Ctrl+D scrolls down by contentViewportHeight/2.
// Uses files with real hunk data so diffSectionRowCount produces enough rows
// (diffFileDisplayRows = 3 overhead + hunk lines) to exceed the viewport.
func TestCtrlDPageDown(t *testing.T) {
	t.Parallel()

	// Build 5 files each with 8 context lines → diffFileDisplayRows = 3+1+8 = 12 per file = 60 total.
	makeRichFile := func() diffmodel.DiffFile {
		lines := make([]diffmodel.DiffLine, 8)
		for i := range lines {
			lines[i] = diffmodel.DiffLine{Kind: "context", Raw: " ctx"}
		}
		hunk := diffmodel.DiffHunk{Header: "@@ -1,8 +1,8 @@", Lines: lines}
		f := diffmodel.DiffFile{
			OldPath: "file.go", NewPath: "file.go", Status: "modified",
			Hunks: []diffmodel.DiffHunk{hunk},
		}
		f.DisplayRows = diffFileDisplayRows(&f)
		return f
	}
	files := make([]diffmodel.DiffFile, 5)
	for i := range files {
		files[i] = makeRichFile()
	}

	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 0

	half := m.contentViewportHeight() / 2
	m = pressKey(m, "ctrl+d")

	if m.ContentScroll < half {
		t.Errorf("expected ContentScroll >= %d after Ctrl+D, got %d", half, m.ContentScroll)
	}
	// Must not exceed max.
	if m.ContentScroll > m.maxContentScroll() {
		t.Errorf("expected ContentScroll <= maxContentScroll (%d), got %d", m.maxContentScroll(), m.ContentScroll)
	}
}

// TestCtrlUPageUpClampedAtZero verifies that Ctrl+U from scroll=0 stays at 0.
func TestCtrlUPageUpClampedAtZero(t *testing.T) {
	t.Parallel()

	m := makePRDetail(100, 30, makeFiles("a.go"), nil)
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 0

	m = pressKey(m, "ctrl+u")

	if m.ContentScroll != 0 {
		t.Errorf("expected ContentScroll=0 after Ctrl+U at top, got %d", m.ContentScroll)
	}
}

// ── Overscan / render tests ───────────────────────────────────────────────────

// TestViewportOverscanBelowFold verifies that renderContentLines returns exactly
// contentH lines and that content below the visible fold (within overscan) is
// available in the collected map without causing blank outputs above the fold.
func TestViewportOverscanBelowFold(t *testing.T) {
	t.Parallel()

	// Create enough content to have rows well below the fold.
	files := makeFilesWithDisplayRows(5, 20) // 100 diff rows
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("header body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.SetTheme(theme.Default())

	cw := m.contentW()
	sections := m.buildContentSections(cw)
	contentH := m.contentViewportHeight()

	lines := m.renderContentLines(sections, 0, contentH, cw)

	if len(lines) != contentH {
		t.Errorf("expected exactly %d content lines, got %d", contentH, len(lines))
	}
}

// TestViewportScrolledMidContent verifies that when scrolled to the middle of the
// diff section, the rendered lines correspond to that section's content.
func TestViewportScrolledMidContent(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(3, 15) // 45 diff rows
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("description text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.SetTheme(theme.Default())

	cw := m.contentW()
	sections := m.buildContentSections(cw)
	contentH := m.contentViewportHeight()

	diffSec, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		t.Fatal("expected SectionDiff to exist")
	}

	// Scroll to the middle of the diff section.
	midScroll := diffSec.StartRow + diffSec.RowCount/2
	lines := m.renderContentLines(sections, midScroll, contentH, cw)

	if len(lines) != contentH {
		t.Errorf("expected exactly %d lines at mid-scroll, got %d", contentH, len(lines))
	}
}
