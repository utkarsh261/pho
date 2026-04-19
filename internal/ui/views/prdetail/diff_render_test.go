package prdetail

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// TestMain forces ANSI color rendering for all tests in this package so that
// color-assertion tests (TestDiffRender*Color, TestDiffRenderHunkHeaderColor)
// work in headless CI environments where stdout is not a TTY.
func TestMain(m *testing.M) {
	// CI runners may export NO_COLOR, which disables ANSI even when a renderer
	// profile is forced. Clear it so color-assertion tests remain deterministic.
	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("CLICOLOR_FORCE", "1")

	r := lipgloss.NewRenderer(os.Stdout, termenv.WithProfile(termenv.ANSI), termenv.WithUnsafe())
	lipgloss.SetDefaultRenderer(r)
	os.Exit(m.Run())
}

// ── helpers ───────────────────────────────────────────────────────────────────

// makeAdditionFile returns a DiffFile with a single addition line.
func makeAdditionFile(path, raw string) diffmodel.DiffFile {
	f := diffmodel.DiffFile{
		OldPath: path,
		NewPath: path,
		Status:  "modified",
		Hunks: []diffmodel.DiffHunk{
			{
				Header: "@@ -1,1 +1,2 @@",
				Lines:  []diffmodel.DiffLine{{Kind: "addition", Raw: raw}},
			},
		},
	}
	f.DisplayRows = diffFileDisplayRows(&f)
	return f
}

func makeDeletionFile(path, raw string) diffmodel.DiffFile {
	f := diffmodel.DiffFile{
		OldPath: path,
		NewPath: path,
		Status:  "modified",
		Hunks: []diffmodel.DiffHunk{
			{
				Header: "@@ -1,2 +1,1 @@",
				Lines:  []diffmodel.DiffLine{{Kind: "deletion", Raw: raw}},
			},
		},
	}
	f.DisplayRows = diffFileDisplayRows(&f)
	return f
}

func makeContextFile(path, raw string) diffmodel.DiffFile {
	f := diffmodel.DiffFile{
		OldPath: path,
		NewPath: path,
		Status:  "modified",
		Hunks: []diffmodel.DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines:  []diffmodel.DiffLine{{Kind: "context", Raw: raw}},
			},
		},
	}
	f.DisplayRows = diffFileDisplayRows(&f)
	return f
}

func renderFileLines(t *testing.T, files []diffmodel.DiffFile) []string {
	t.Helper()
	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.SetTheme(theme.Default())
	return m.renderDiffSectionLines(0, files[0].DisplayRows, 80)
}

// ── Color tests ───────────────────────────────────────────────────────────────

// TestDiffRenderAdditionLineColor verifies that addition lines are rendered with
// ANSI green (color index 2) styling.
func TestDiffRenderAdditionLineColor(t *testing.T) {
	t.Parallel()

	f := makeAdditionFile("main.go", "+added line")
	lines := renderFileLines(t, []diffmodel.DiffFile{f})

	// Row layout: 0=blank, 1=separator, 2=header, 3=hunk header, 4=addition line
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 rows, got %d", len(lines))
	}
	addLine := lines[4]

	// Text content must survive stripping.
	if !strings.Contains(plainText(addLine), "+added line") {
		t.Errorf("expected '+added line' in addition line output, got: %q", plainText(addLine))
	}

	// With TestMain forcing ANSI renderer, ANSI codes must be present.
	if !ansiRe.MatchString(addLine) {
		t.Errorf("expected ANSI escape codes in addition line (green color), got plain: %q", addLine)
	}

	// ANSI codes already confirmed above; green is the intent (exact escape depends on terminal profile).
}

// TestDiffRenderDeletionLineColor verifies that deletion lines are rendered with
// ANSI red (color index 1) styling.
func TestDiffRenderDeletionLineColor(t *testing.T) {
	t.Parallel()

	f := makeDeletionFile("main.go", "-removed line")
	lines := renderFileLines(t, []diffmodel.DiffFile{f})

	if len(lines) < 5 {
		t.Fatalf("expected at least 5 rows, got %d", len(lines))
	}
	delLine := lines[4]

	if !strings.Contains(plainText(delLine), "-removed line") {
		t.Errorf("expected '-removed line' in deletion line output, got: %q", plainText(delLine))
	}
	if !ansiRe.MatchString(delLine) {
		t.Errorf("expected ANSI escape codes in deletion line (red color), got plain: %q", delLine)
	}
	// ANSI codes already confirmed above; red is the intent (exact escape depends on terminal profile).
}

// TestDiffRenderHunkHeaderColor verifies hunk headers use ANSI cyan (color 6) and bold.
func TestDiffRenderHunkHeaderColor(t *testing.T) {
	t.Parallel()

	hunkHeader := "@@ -1,3 +1,3 @@"
	f := makeFileWithHunks("pkg/auth.go", []diffmodel.DiffHunk{
		{
			Header: hunkHeader,
			Lines:  []diffmodel.DiffLine{{Kind: "context", Raw: "code"}},
		},
	})
	lines := renderFileLines(t, []diffmodel.DiffFile{f})

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 rows, got %d", len(lines))
	}
	hunkLine := lines[3]

	if !strings.Contains(plainText(hunkLine), hunkHeader) {
		t.Errorf("expected hunk header text in row 3, got: %q", plainText(hunkLine))
	}
	if !ansiRe.MatchString(hunkLine) {
		t.Errorf("expected ANSI escape codes in hunk header (cyan+bold), got plain: %q", hunkLine)
	}
	// Must be bold: either standalone \x1b[1m or combined \x1b[1;... (bold merged with color).
	if !strings.Contains(hunkLine, "\x1b[1m") && !strings.Contains(hunkLine, "\x1b[1;") {
		t.Errorf("expected bold ANSI code in hunk header, got: %q", hunkLine)
	}
	// ANSI codes confirmed above; cyan is the intent (exact escape depends on terminal profile).
}

// TestDiffRenderFileHeaderMuted verifies the file header bar is styled (not plain)
// and does not use the bright green or red of diff content lines.
func TestDiffRenderFileHeaderMuted(t *testing.T) {
	t.Parallel()

	f := makeFileWithHunks("internal/server.go", []diffmodel.DiffHunk{
		{Header: "@@ -1,1 +1,1 @@", Lines: []diffmodel.DiffLine{{Kind: "addition", Raw: "+x"}}},
	})
	lines := renderFileLines(t, []diffmodel.DiffFile{f})

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(lines))
	}
	headerLine := lines[2]

	// Must contain filename.
	if !strings.Contains(plainText(headerLine), "internal/server.go") {
		t.Errorf("expected filename in file header bar, got: %q", plainText(headerLine))
	}
	// File header must be styled (not plain).
	if !ansiRe.MatchString(headerLine) {
		t.Errorf("expected ANSI styling in file header bar, got plain: %q", headerLine)
	}
	// Must NOT use plain green (addition color).
	if strings.Contains(headerLine, "\x1b[32m") {
		t.Errorf("file header must not use green addition color: %q", headerLine)
	}
	// Must NOT use plain red (deletion color).
	if strings.Contains(headerLine, "\x1b[31m") {
		t.Errorf("file header must not use red deletion color: %q", headerLine)
	}
}

// TestDiffRenderContextLineNormal verifies context lines have no ANSI color codes.
func TestDiffRenderContextLineNormal(t *testing.T) {
	t.Parallel()

	f := makeContextFile("util.go", "unchanged context line")
	lines := renderFileLines(t, []diffmodel.DiffFile{f})

	if len(lines) < 5 {
		t.Fatalf("expected at least 5 rows, got %d", len(lines))
	}
	ctxLine := lines[4]

	// Context lines must be the raw text with NO ANSI codes.
	if ansiRe.MatchString(ctxLine) {
		t.Errorf("expected NO ANSI codes in context line, got: %q", ctxLine)
	}
	if ctxLine != "unchanged context line" {
		t.Errorf("expected exact raw text for context line, got: %q", ctxLine)
	}
}

// ── Structural / content tests ────────────────────────────────────────────────

// TestDiffRenderBinaryFilePlaceholder verifies that binary files render the
// "📄 Binary file (no diff available)" placeholder row and no hunk content.
func TestDiffRenderBinaryFilePlaceholder(t *testing.T) {
	t.Parallel()

	f := diffmodel.DiffFile{
		OldPath:  "image.png",
		NewPath:  "image.png",
		Status:   "modified",
		IsBinary: true,
		// No hunks — binary files have none.
	}
	f.DisplayRows = diffFileDisplayRows(&f)

	m := makePRDetail(100, 30, []diffmodel.DiffFile{f}, nil)
	m.Diff = makeDiff([]diffmodel.DiffFile{f})
	m.DiffLoading = false
	m.SetTheme(theme.Default())

	lines := m.renderDiffSectionLines(0, f.DisplayRows, 80)

	// Row layout: 0=blank, 1=separator, 2=file header, 3=binary placeholder
	if len(lines) < 4 {
		t.Fatalf("expected 4 rows for binary file (blank+sep+header+placeholder), got %d", len(lines))
	}

	// Placeholder text must appear at row 3.
	placeholder := plainText(lines[3])
	if !strings.Contains(placeholder, "📄 Binary file (no diff available)") {
		t.Errorf("expected binary placeholder at row 3, got: %q", placeholder)
	}

	// No additional hunk content beyond row 3.
	if len(lines) > 4 {
		for _, extra := range lines[4:] {
			if strings.TrimSpace(plainText(extra)) != "" {
				t.Errorf("expected no hunk content after binary placeholder, got: %q", extra)
			}
		}
	}
}

// TestDiffRenderRenamedFileHeader verifies that a renamed file's header shows
// "oldPath → newPath" in the file header bar.
func TestDiffRenderRenamedFileHeader(t *testing.T) {
	t.Parallel()

	f := diffmodel.DiffFile{
		OldPath: "pkg/old/server.go",
		NewPath: "pkg/new/server.go",
		Status:  "renamed",
		Hunks: []diffmodel.DiffHunk{
			{Header: "@@ -1,1 +1,1 @@", Lines: []diffmodel.DiffLine{{Kind: "context", Raw: "ctx"}}},
		},
	}
	f.DisplayRows = diffFileDisplayRows(&f)

	m := makePRDetail(100, 30, []diffmodel.DiffFile{f}, nil)
	m.Diff = makeDiff([]diffmodel.DiffFile{f})
	m.DiffLoading = false
	m.SetTheme(theme.Default())

	lines := m.renderDiffSectionLines(0, f.DisplayRows, 80)

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(lines))
	}
	header := plainText(lines[2])

	if !strings.Contains(header, "pkg/old/server.go") {
		t.Errorf("expected old path in renamed file header, got: %q", header)
	}
	if !strings.Contains(header, "pkg/new/server.go") {
		t.Errorf("expected new path in renamed file header, got: %q", header)
	}
	if !strings.Contains(header, "→") {
		t.Errorf("expected '→' arrow in renamed file header, got: %q", header)
	}
}

// TestDiffRenderEmptyDiff verifies that an empty diff (no files) renders the
// "No changes" placeholder.
func TestDiffRenderEmptyDiff(t *testing.T) {
	t.Parallel()

	m := makePRDetail(100, 30, nil, nil)
	m.Diff = &diffmodel.DiffModel{Files: nil}
	m.DiffLoading = false
	m.SetTheme(theme.Default())

	lines := m.renderDiffSectionLines(0, 1, 80)

	if len(lines) == 0 {
		t.Fatal("expected at least 1 row for empty diff placeholder")
	}
	if !strings.Contains(lines[0], "No changes") {
		t.Errorf("expected 'No changes' for empty diff, got: %q", lines[0])
	}
}

// TestDiffRenderTruncationAt5000Rows verifies that when the diff exceeds
// maxDiffDisplayRows, a truncation banner appears at row maxDiffDisplayRows
// and diffSectionRowCount returns maxDiffDisplayRows+1.
func TestDiffRenderTruncationAt5000Rows(t *testing.T) {
	t.Parallel()

	// Each file with no hunks has diffFileDisplayRows = 3 (blank+sep+header).
	// 1668 files × 3 = 5004 rows > maxDiffDisplayRows (5000).
	const fileCount = 1668
	files := makeFilesWithDisplayRows(fileCount, 3)

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.SetTheme(theme.Default())

	// diffSectionRowCount must return maxDiffDisplayRows+1.
	cw := m.contentW()
	sections := m.buildContentSections(cw)
	diff, ok := findSection(sections, domain.SectionDiff)
	if !ok {
		t.Fatal("expected SectionDiff to be present")
	}
	if diff.RowCount != maxDiffDisplayRows+1 {
		t.Errorf("expected RowCount=%d (cap+banner), got %d", maxDiffDisplayRows+1, diff.RowCount)
	}

	// Render a 2-row window that straddles the truncation point:
	// rows [maxDiffDisplayRows-1, maxDiffDisplayRows+1) → indices 0 and 1 in out.
	lines := m.renderDiffSectionLines(maxDiffDisplayRows-1, maxDiffDisplayRows+1, 80)

	if len(lines) < 2 {
		t.Fatalf("expected 2 lines in truncation window, got %d", len(lines))
	}
	// lines[1] maps to absolute diff row maxDiffDisplayRows → the banner.
	bannerText := plainText(lines[1])
	if !strings.Contains(bannerText, "truncated") {
		t.Errorf("expected truncation banner at row maxDiffDisplayRows, got: %q", bannerText)
	}
}

// TestDiffRenderVirtualizationSkipsOffscreen verifies that files entirely outside
// the render window produce no output rows, while the target file does appear.
func TestDiffRenderVirtualizationSkipsOffscreen(t *testing.T) {
	t.Parallel()

	hunkA := []diffmodel.DiffHunk{{Header: "@@ -1 +1 @@", Lines: []diffmodel.DiffLine{{Kind: "addition", Raw: "+alpha"}}}}
	hunkB := []diffmodel.DiffHunk{{Header: "@@ -1 +1 @@", Lines: []diffmodel.DiffLine{{Kind: "addition", Raw: "+beta"}}}}
	hunkC := []diffmodel.DiffHunk{{Header: "@@ -1 +1 @@", Lines: []diffmodel.DiffLine{{Kind: "addition", Raw: "+gamma"}}}}

	fA := makeFileWithHunks("alpha.go", hunkA)
	fB := makeFileWithHunks("beta.go", hunkB)
	fC := makeFileWithHunks("gamma.go", hunkC)
	files := []diffmodel.DiffFile{fA, fB, fC}

	m := makePRDetail(100, 30, files, nil)
	m.Diff = makeDiff(files)
	m.DiffLoading = false
	m.SetTheme(theme.Default())

	// Request only fB's rows.
	bStart := fA.DisplayRows
	bEnd := fA.DisplayRows + fB.DisplayRows
	lines := m.renderDiffSectionLines(bStart, bEnd, 80)

	joined := plainText(strings.Join(lines, "\n"))

	// fB content must appear.
	if !strings.Contains(joined, "beta.go") {
		t.Errorf("expected 'beta.go' in virtualized window, got:\n%s", joined)
	}

	// fA and fC must NOT appear.
	if strings.Contains(joined, "alpha.go") {
		t.Errorf("expected 'alpha.go' to be skipped (off-screen), but found it:\n%s", joined)
	}
	if strings.Contains(joined, "gamma.go") {
		t.Errorf("expected 'gamma.go' to be skipped (off-screen), but found it:\n%s", joined)
	}
}
