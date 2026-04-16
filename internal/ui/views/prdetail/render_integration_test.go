package prdetail

import (
	"regexp"
	"strings"
	"testing"

	diffmodel "github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/application/cmds"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/ui/theme"
)

// ansiRe strips ANSI escape sequences so we can assert on plain text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainText(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// makeFileWithHunks builds a DiffFile with real hunk data and computes DisplayRows.
func makeFileWithHunks(path string, hunks []diffmodel.DiffHunk) diffmodel.DiffFile {
	rows := 1 // file header row
	for _, h := range hunks {
		rows++ // hunk header row
		rows += len(h.Lines)
	}
	return diffmodel.DiffFile{
		OldPath:     path,
		NewPath:     path,
		Status:      "modified",
		Hunks:       hunks,
		DisplayRows: rows,
	}
}

// ── Diff content visible in rendered output ──────────────────────────────────

// TestRenderDiffHeaderAppearsInView verifies that a file's "─── path" header
// is present in the rendered View() output when the diff is loaded and focus
// is on the content viewport.
func TestRenderDiffHeaderAppearsInView(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{
			Header: "@@ -1,3 +1,3 @@",
			Lines: []diffmodel.DiffLine{
				{Kind: "context", Raw: " unchanged"},
				{Kind: "deletion", Raw: "-old line"},
				{Kind: "addition", Raw: "+new line"},
			},
		},
	}
	f := makeFileWithHunks("pkg/foo/bar.go", hunks)
	files := []diffmodel.DiffFile{f}

	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("") // empty description → Diff.StartRow = 0
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.SetTheme(theme.Default())

	out := plainText(m.View())

	if !strings.Contains(out, "pkg/foo/bar.go") {
		t.Errorf("expected file path 'pkg/foo/bar.go' in rendered output; got:\n%s", out)
	}
}

// TestRenderDiffHunkHeaderAppearsInView verifies that the hunk header line
// (e.g. "@@ -1,3 +1,3 @@") appears in the rendered View() output.
func TestRenderDiffHunkHeaderAppearsInView(t *testing.T) {
	t.Parallel()

	hunkHeader := "@@ -1,3 +1,3 @@"
	hunks := []diffmodel.DiffHunk{
		{
			Header: hunkHeader,
			Lines: []diffmodel.DiffLine{
				{Kind: "addition", Raw: "+hello"},
			},
		},
	}
	f := makeFileWithHunks("main.go", hunks)
	files := []diffmodel.DiffFile{f}

	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.SetTheme(theme.Default())

	out := plainText(m.View())

	if !strings.Contains(out, hunkHeader) {
		t.Errorf("expected hunk header %q in rendered output; got:\n%s", hunkHeader, out)
	}
}

// TestRenderDiffLinesAppearInView verifies that actual diff lines (Raw field)
// appear in the rendered View() output.
func TestRenderDiffLinesAppearInView(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{
			Header: "@@ -1,2 +1,2 @@",
			Lines: []diffmodel.DiffLine{
				{Kind: "deletion", Raw: "-removed line"},
				{Kind: "addition", Raw: "+added line"},
			},
		},
	}
	f := makeFileWithHunks("service.go", hunks)
	files := []diffmodel.DiffFile{f}

	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.SetTheme(theme.Default())

	out := plainText(m.View())

	if !strings.Contains(out, "-removed line") {
		t.Errorf("expected deletion line '-removed line' in rendered output; got:\n%s", out)
	}
	if !strings.Contains(out, "+added line") {
		t.Errorf("expected addition line '+added line' in rendered output; got:\n%s", out)
	}
}

// TestRenderDescriptionDoesNotObscureDiff verifies that when a description is
// present, pressing '2' scrolls such that diff content still appears in the
// rendered output (not just blank rows).
func TestRenderDescriptionDoesNotObscureDiff(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{
			Header: "@@ -1,1 +1,1 @@",
			Lines: []diffmodel.DiffLine{
				{Kind: "addition", Raw: "+brand new"},
			},
		},
	}
	// Use enough files to push total content well past the viewport height.
	var files []diffmodel.DiffFile
	for i := 0; i < 5; i++ {
		files = append(files, makeFileWithHunks("file.go", hunks))
	}

	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("some description that takes a few rows")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.SetTheme(theme.Default())

	// Jump to diff section.
	m = pressKey(m, "2")

	out := plainText(m.View())

	if !strings.Contains(out, "file.go") {
		t.Errorf("expected diff file header 'file.go' in rendered output after pressing '2'; got:\n%s", out)
	}
}

// ── Tab indicator reflects active section ────────────────────────────────────

// TestTabIndicatorShowsDiffActiveAfterPress2 verifies that pressing '2'
// causes the "Diff" label in the rendered output to appear with the active
// marker ("●") when there is enough scrollable content.
func TestTabIndicatorShowsDiffActiveAfterPress2(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{
			Header: "@@ -1,5 +1,5 @@",
			Lines: func() []diffmodel.DiffLine {
				lines := make([]diffmodel.DiffLine, 10)
				for i := range lines {
					lines[i] = diffmodel.DiffLine{Kind: "context", Raw: " ctx line"}
				}
				return lines
			}(),
		},
	}
	var files []diffmodel.DiffFile
	for i := 0; i < 4; i++ {
		files = append(files, makeFileWithHunks("mod.go", hunks))
	}

	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("description text that adds rows above the diff")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusFiles
	m.SetTheme(theme.Default())

	m = pressKey(m, "2")

	out := plainText(m.View())

	// The active marker "●" must appear adjacent to "Diff".
	if !strings.Contains(out, "● Diff") {
		t.Errorf("expected '● Diff' in rendered output after pressing '2'; got tabs section:\n%s", out)
	}
}

// TestTabIndicatorShowsDescActiveAtTop verifies that at scroll=0 with a
// non-empty description, the "Desc" tab carries the active marker.
func TestTabIndicatorShowsDescActiveAtTop(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(2, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("some body text")
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 0
	m.SetTheme(theme.Default())

	out := plainText(m.View())

	if !strings.Contains(out, "● Desc") {
		t.Errorf("expected '● Desc' at scroll=0 with non-empty description; got:\n%s", out)
	}
}

// TestTabIndicatorDiffActiveWhenDescriptionEmpty verifies that when description
// is empty (RowCount=0), Diff is the active section at scroll=0.
func TestTabIndicatorDiffActiveWhenDescriptionEmpty(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(2, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("") // empty → Description omitted
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.ContentScroll = 0
	m.SetTheme(theme.Default())

	out := plainText(m.View())

	if !strings.Contains(out, "● Diff") {
		t.Errorf("expected '● Diff' when description is empty and scroll=0; got:\n%s", out)
	}
}

// ── renderDiffSectionLines unit-level checks ──────────────────────────────────

// TestRenderDiffSectionLinesFileHeader verifies row 0 is the file header.
func TestRenderDiffSectionLinesFileHeader(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{Header: "@@ -1,1 +1,1 @@", Lines: []diffmodel.DiffLine{{Kind: "addition", Raw: "+x"}}},
	}
	f := makeFileWithHunks("cmd/main.go", hunks)
	files := []diffmodel.DiffFile{f}

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false

	lines := m.renderDiffSectionLines(0, f.DisplayRows, 80)

	if len(lines) == 0 {
		t.Fatal("renderDiffSectionLines returned no lines")
	}
	if !strings.Contains(lines[0], "cmd/main.go") {
		t.Errorf("expected file header on row 0 to contain 'cmd/main.go', got %q", lines[0])
	}
}

// TestRenderDiffSectionLinesHunkHeader verifies row 1 is the hunk header.
func TestRenderDiffSectionLinesHunkHeader(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{Header: "@@ -5,3 +5,4 @@", Lines: []diffmodel.DiffLine{{Kind: "addition", Raw: "+y"}}},
	}
	f := makeFileWithHunks("lib.go", hunks)
	files := []diffmodel.DiffFile{f}

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false

	lines := m.renderDiffSectionLines(0, f.DisplayRows, 80)

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(lines))
	}
	if lines[1] != "@@ -5,3 +5,4 @@" {
		t.Errorf("expected hunk header on row 1, got %q", lines[1])
	}
}

// TestRenderDiffSectionLinesDiffLineRaw verifies that diff line Raw values
// appear in subsequent rows after the hunk header.
func TestRenderDiffSectionLinesDiffLineRaw(t *testing.T) {
	t.Parallel()

	hunks := []diffmodel.DiffHunk{
		{
			Header: "@@ -1,2 +1,2 @@",
			Lines: []diffmodel.DiffLine{
				{Kind: "deletion", Raw: "-gone"},
				{Kind: "addition", Raw: "+here"},
			},
		},
	}
	f := makeFileWithHunks("util.go", hunks)
	files := []diffmodel.DiffFile{f}

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false

	// rows: 0=file header, 1=hunk header, 2="-gone", 3="+here"
	lines := m.renderDiffSectionLines(0, f.DisplayRows, 80)

	if len(lines) < 4 {
		t.Fatalf("expected 4 rows (header+hunk+2 lines), got %d: %v", len(lines), lines)
	}
	if lines[2] != "-gone" {
		t.Errorf("expected '-gone' at row 2, got %q", lines[2])
	}
	if lines[3] != "+here" {
		t.Errorf("expected '+here' at row 3, got %q", lines[3])
	}
}

// TestRenderDiffSectionLinesMultipleFiles verifies virtualization across files:
// requesting only rows belonging to the second file returns that file's content.
func TestRenderDiffSectionLinesMultipleFiles(t *testing.T) {
	t.Parallel()

	hunk1 := []diffmodel.DiffHunk{{Header: "@@ -1 +1 @@", Lines: []diffmodel.DiffLine{{Raw: " ctx"}}}}
	hunk2 := []diffmodel.DiffHunk{{Header: "@@ -2 +2 @@", Lines: []diffmodel.DiffLine{{Raw: "+second"}}}}

	f1 := makeFileWithHunks("alpha.go", hunk1) // rows 0..f1.DisplayRows-1
	f2 := makeFileWithHunks("beta.go", hunk2)  // rows f1.DisplayRows..f1+f2.DisplayRows-1
	files := []diffmodel.DiffFile{f1, f2}

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false

	// Request exactly the rows belonging to f2.
	localStart := f1.DisplayRows
	localEnd := f1.DisplayRows + f2.DisplayRows
	lines := m.renderDiffSectionLines(localStart, localEnd, 80)

	if len(lines) == 0 {
		t.Fatal("expected rows for second file, got none")
	}
	if !strings.Contains(lines[0], "beta.go") {
		t.Errorf("expected 'beta.go' file header in first returned row, got %q", lines[0])
	}
}

// TestRenderDiffSectionLinesLoadingPlaceholder verifies that when Diff is nil
// and DiffLoading is true, row 0 contains the loading placeholder.
func TestRenderDiffSectionLinesLoadingPlaceholder(t *testing.T) {
	t.Parallel()

	m := makePRDetail(100, 30, nil, nil)
	m.Diff = nil
	m.DiffLoading = true

	lines := m.renderDiffSectionLines(0, 1, 80)

	if len(lines) == 0 || lines[0] != "Loading diff…" {
		t.Errorf("expected 'Loading diff…' placeholder, got %v", lines)
	}
}

// ── domain.PRPreviewSnapshot reviewers section ───────────────────────────────

// TestLegacyCacheDisplayRowsZero verifies that when a DiffModel arrives from a
// legacy cache entry with DisplayRows=0 on every file (written before the field
// was added), the model recomputes DisplayRows from hunks so that:
//   - the diff section has the correct row count (not the safety value of 1)
//   - pressing '2' actually scrolls to the diff section
//   - diff file headers and hunk content are visible in the rendered output
func TestLegacyCacheDisplayRowsZero(t *testing.T) {
	t.Parallel()

	// Build files with real hunk data but DisplayRows deliberately set to 0
	// to simulate a stale cache entry.
	hunks := []diffmodel.DiffHunk{
		{
			Header: "@@ -1,3 +1,4 @@",
			Lines: []diffmodel.DiffLine{
				{Kind: "context", Raw: " ctx"},
				{Kind: "deletion", Raw: "-old"},
				{Kind: "addition", Raw: "+new"},
			},
		},
	}
	makeStaleFile := func(path string) diffmodel.DiffFile {
		return diffmodel.DiffFile{
			OldPath:     path,
			NewPath:     path,
			Status:      "modified",
			Hunks:       hunks,
			DisplayRows: 0, // legacy cache: field missing
		}
	}

	files := []diffmodel.DiffFile{
		makeStaleFile("alpha.go"),
		makeStaleFile("beta.go"),
		makeStaleFile("gamma.go"),
	}

	m := makePRDetail(100, 30, files, nil)
	// Simulate long description so diff is below the fold at scroll=0.
	m.Detail = makeDetailWithBody(strings.Repeat("word ", 200))
	// Feed the diff via the DiffLoaded message path so recompute runs.
	dm := makeDiff(files)
	diffMsg := cmds.DiffLoaded{Diff: *dm}
	next, _ := m.Update(diffMsg)
	m = next

	// Verify DisplayRows was recomputed (should be 1+1+3 = 5 per file).
	for i, f := range m.Diff.Files {
		if f.DisplayRows == 0 {
			t.Errorf("file %d (%s): DisplayRows still 0 after recompute", i, f.NewPath)
		}
	}

	// Verify the diff section has the correct total rows (not just 1).
	cw := m.contentW()
	sections := m.buildContentSections(cw)
	diff, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		t.Fatal("expected SectionDiff to be present after recompute")
	}
	// 3 files × 5 rows = 15 diff rows; must be >1 (the stale safety value).
	if diff.RowCount <= 1 {
		t.Errorf("expected diff.RowCount > 1 after DisplayRows recompute, got %d", diff.RowCount)
	}

	// Pressing '2' must scroll to the diff start.
	m.leftPanel.Focus = FocusFiles
	m = pressKey(m, "2")
	if m.ContentScroll != diff.StartRow {
		t.Errorf("expected ContentScroll=%d after '2', got %d", diff.StartRow, m.ContentScroll)
	}

	// Rendered output must contain diff file headers.
	out := plainText(m.View())
	if !strings.Contains(out, "alpha.go") {
		t.Errorf("expected 'alpha.go' in rendered output after '2'; got:\n%s", out)
	}
}

// TestRenderCommentsAppearsInView verifies that reviewer login + state appear
// in the View() output when reviewers are present.
func TestRenderCommentsAppearsInView(t *testing.T) {
	t.Parallel()

	files := makeFilesWithDisplayRows(3, 10)
	m := makePRDetail(100, 30, files, nil)
	m.Detail = makeDetailWithBody("")
	m.Detail.Reviewers = []domain.PreviewReviewer{{Login: "bobreviewer", State: "APPROVED"}}
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.DetailLoading = false
	m.leftPanel.Focus = FocusContent
	m.SetTheme(theme.Default())

	// Scroll to comments section.
	m = pressKey(m, "3")

	out := plainText(m.View())

	if !strings.Contains(out, "bobreviewer") {
		t.Errorf("expected reviewer 'bobreviewer' in rendered output; got:\n%s", out)
	}
}
