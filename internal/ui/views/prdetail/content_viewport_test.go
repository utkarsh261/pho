package prdetail

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

var updateGolden = flag.Bool("update", false, "overwrite golden files with current output")

var descAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func descStripANSI(s string) string {
	return descAnsiRe.ReplaceAllString(s, "")
}

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

// TestViewportNoCommentsShowsPlaceholder verifies that when no reviews exist the
// Comments section still shows the "No reviews" placeholder.
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
	lines := m.commentLines(cw, -1)
	if len(lines) == 0 {
		t.Fatal("expected commentLines to return non-nil")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "No reviews") {
		t.Errorf("expected 'No reviews' placeholder in rendered output, got:\n%s", joined)
	}
}

// TestViewportSectionJump verifies pressing "2" switches to the Diff tab
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

	m = pressKey(m, "2")

	if m.activeTab != TabDiff {
		t.Errorf("expected activeTab=TabDiff after '2', got %d", m.activeTab)
	}
	if m.ContentScroll != 0 {
		t.Errorf("expected ContentScroll=0 after '2' jump, got %d", m.ContentScroll)
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

	m = pressKey(m, "1") // already on Description tab → no-op

	if m.activeTab != TabDescription {
		t.Errorf("expected activeTab=TabDescription after '1', got %d", m.activeTab)
	}
	// Re-pressing current tab is a no-op; focus stays unchanged.
	if m.leftPanel.Focus != FocusFiles {
		t.Errorf("expected focus unchanged when re-pressing current tab, got %v", m.leftPanel.Focus)
	}
	if m.ContentScroll != 0 {
		t.Errorf("expected ContentScroll=0 after jump to empty section, got %d", m.ContentScroll)
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
	m.activeTab = TabDiff
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

// ── descriptionLines golden tests ─────────────────────────────────────────────

// descriptionBody is a realistic markdown PR description used for golden tests.
const descriptionBody = `## Summary

This PR integrates **glamour** as the markdown renderer for PR descriptions.

## Changes

- Added lazy-initializing ` + "`" + `Renderer` + "`" + ` wrapper in ` + "`" + `internal/ui/markdown` + "`" + `
- Replaced plain ` + "`" + `wrapParagraph` + "`" + ` calls in description rendering

## Notes

> Regenerate golden files after bumping the glamour version.
`

// descGoldenWidths are the terminal widths tested, chosen to straddle the
// MinWidthForSidebar=80 threshold that changes the right-panel content width.
var descGoldenWidths = []int{79, 80, 120}

func TestDescriptionLinesGolden(t *testing.T) {
	for _, termW := range descGoldenWidths {
		termW := termW
		t.Run(fmt.Sprintf("w%d", termW), func(t *testing.T) {
			t.Parallel()

			m := makePRDetail(termW, 40, nil, nil)
			m.Detail = makeDetailWithBody(descriptionBody)
			cw := m.contentW()

			lines := m.descriptionLines(cw)
			got := descStripANSI(strings.Join(lines, "\n"))

			goldenPath := filepath.Join("testdata", "golden", fmt.Sprintf("description_w%d.txt", termW))
			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
				return
			}

			data, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with -update to generate)", goldenPath, err)
			}
			if got != string(data) {
				t.Errorf("golden mismatch for description at width %d\ngot:\n%s\nwant:\n%s", termW, got, string(data))
			}
		})
	}
}
