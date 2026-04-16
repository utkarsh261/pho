package prdetail

import (
	"regexp"
	"strings"
	"testing"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// ansiRe strips ANSI escape sequences so we can assert on plain text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainText(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// makeFileWithHunks builds a DiffFile with real hunk data.
// DisplayRows is set via diffFileDisplayRows so it matches the UI row layout
// (3 overhead rows + hunk headers + diff lines).
func makeFileWithHunks(path string, hunks []diffmodel.DiffHunk) diffmodel.DiffFile {
	f := diffmodel.DiffFile{
		OldPath: path,
		NewPath: path,
		Status:  "modified",
		Hunks:   hunks,
	}
	f.DisplayRows = diffFileDisplayRows(&f)
	return f
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

// TestRenderDiffSectionLinesFileHeader verifies row 2 is the file header bar.
// Row layout per file: 0=blank, 1=dashed separator, 2=file header bar, 3+=hunks.
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

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(lines))
	}
	// row 0: blank padding
	if strings.TrimSpace(plainText(lines[0])) != "" {
		t.Errorf("expected blank row at row 0, got %q", lines[0])
	}
	// row 2: file header bar contains filename
	if !strings.Contains(plainText(lines[2]), "cmd/main.go") {
		t.Errorf("expected file header bar at row 2 to contain 'cmd/main.go', got %q", plainText(lines[2]))
	}
}

// TestRenderDiffSectionLinesHunkHeader verifies row 3 is the hunk header.
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

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 rows, got %d", len(lines))
	}
	// row 3: hunk header (after blank, separator, file bar)
	if !strings.Contains(plainText(lines[3]), "@@ -5,3 +5,4 @@") {
		t.Errorf("expected hunk header at row 3, got %q", plainText(lines[3]))
	}
}

// TestRenderDiffSectionLinesDiffLineRaw verifies that diff line Raw values
// appear at rows 4 and 5 (after blank, separator, file bar, hunk header).
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

	// rows: 0=blank, 1=separator, 2=file bar, 3=hunk header, 4="-gone", 5="+here"
	lines := m.renderDiffSectionLines(0, f.DisplayRows, 80)

	if len(lines) < 6 {
		t.Fatalf("expected 6 rows, got %d: %v", len(lines), lines)
	}
	if lines[4] != "-gone" {
		t.Errorf("expected '-gone' at row 4, got %q", lines[4])
	}
	if lines[5] != "+here" {
		t.Errorf("expected '+here' at row 5, got %q", lines[5])
	}
}

// TestRenderDiffSectionLinesMultipleFiles verifies virtualization across files:
// requesting only rows belonging to the second file returns that file's content.
func TestRenderDiffSectionLinesMultipleFiles(t *testing.T) {
	t.Parallel()

	hunk1 := []diffmodel.DiffHunk{{Header: "@@ -1 +1 @@", Lines: []diffmodel.DiffLine{{Raw: " ctx"}}}}
	hunk2 := []diffmodel.DiffHunk{{Header: "@@ -2 +2 @@", Lines: []diffmodel.DiffLine{{Raw: "+second"}}}}

	f1 := makeFileWithHunks("alpha.go", hunk1)
	f2 := makeFileWithHunks("beta.go", hunk2)
	files := []diffmodel.DiffFile{f1, f2}

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false

	// Request exactly the rows belonging to f2.
	// f1.DisplayRows == diffFileDisplayRows(&f1) since makeFileWithHunks uses it.
	localStart := f1.DisplayRows
	localEnd := f1.DisplayRows + f2.DisplayRows
	lines := m.renderDiffSectionLines(localStart, localEnd, 80)

	if len(lines) == 0 {
		t.Fatal("expected rows for second file, got none")
	}
	// f2 row layout: 0=blank, 1=separator, 2=file bar with "beta.go"
	if !strings.Contains(plainText(lines[2]), "beta.go") {
		t.Errorf("expected 'beta.go' in file header bar at row 2, got %q", plainText(lines[2]))
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

// TestLegacyCacheDisplayRowsZero verifies that files with DisplayRows==0 (legacy
// cache entries written before the field existed) still produce a correct diff
// section because diffSectionRowCount always derives row counts from hunks via
// diffFileDisplayRows — it never reads f.DisplayRows.
func TestLegacyCacheDisplayRowsZero(t *testing.T) {
	t.Parallel()

	// Build files with real hunk data but DisplayRows deliberately set to 0.
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
			DisplayRows: 0, // legacy: field missing in old cache entry
		}
	}

	files := []diffmodel.DiffFile{
		makeStaleFile("alpha.go"),
		makeStaleFile("beta.go"),
		makeStaleFile("gamma.go"),
	}

	m := makePRDetail(100, 30, files, nil)
	// Long description pushes diff below the fold at scroll=0.
	m.Detail = makeDetailWithBody(strings.Repeat("word ", 200))
	dm := makeDiff(files)
	diffMsg := cmds.DiffLoaded{Diff: *dm}
	next, _ := m.Update(diffMsg)
	m = next

	// Diff section must have the correct row count computed from hunks.
	// Each file: 3 overhead + 1 hunk header + 3 lines = 7 rows.  3 files = 21.
	cw := m.contentW()
	sections := m.buildContentSections(cw)
	diff, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		t.Fatal("expected SectionDiff to be present")
	}
	wantRows := 3 * diffFileDisplayRows(&m.Diff.Files[0])
	if diff.RowCount != wantRows {
		t.Errorf("expected diff.RowCount=%d (from hunks), got %d", wantRows, diff.RowCount)
	}

	// Pressing '2' must scroll to the diff start.
	m.leftPanel.Focus = FocusFiles
	m = pressKey(m, "2")
	if m.ContentScroll != diff.StartRow {
		t.Errorf("expected ContentScroll=%d after '2', got %d", diff.StartRow, m.ContentScroll)
	}

	// Rendered output must contain diff file header bars.
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
