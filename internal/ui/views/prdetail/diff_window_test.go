package prdetail

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	diffsearch "github.com/utkarsh261/pho/internal/diff/search"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// BenchmarkRenderLargeDiffViewport measures one frame of the Diff tab on a
// 5000-line file with alternating edit pairs, scrolled to the middle.
func BenchmarkRenderLargeDiffViewport(b *testing.B) {
	lines := make([]diffmodel.DiffLine, 0, 5000)
	for i := range 2500 {
		o, n := i+1, i+1
		lines = append(lines,
			diffmodel.DiffLine{Kind: "deletion", Raw: fmt.Sprintf("-\tvalue := compute(%d, alpha)", i), OldLine: &o},
			diffmodel.DiffLine{Kind: "addition", Raw: fmt.Sprintf("+\tvalue := compute(%d, beta)", i), NewLine: &n})
	}
	f := makeFileWithHunks("big.go", []diffmodel.DiffHunk{{Header: "@@ -1,2500 +1,2500 @@", Lines: lines}})
	m := makePRDetail(200, 50, []diffmodel.DiffFile{f}, nil)
	m.Diff = makeDiff([]diffmodel.DiffFile{f})
	m.SetTheme(theme.Default())
	b.ResetTimer()
	for range b.N {
		_ = m.renderDiffSectionLines(2500, 2545, 150)
	}
}

// TestWindowedDiffRenderMatchesFullRender guards the viewport windowing in
// renderDiffSectionLines: any window must equal the same slice of a full
// render, with a cursor, a visual selection, a draft and search matches active.
func TestWindowedDiffRenderMatchesFullRender(t *testing.T) {
	t.Parallel()
	mk := func(path string, n int) diffmodel.DiffFile {
		var hunks []diffmodel.DiffHunk
		for h := range 3 {
			var lines []diffmodel.DiffLine
			for i := range n {
				o, nn := h*100+i+1, h*100+i+1
				switch i % 3 {
				case 0:
					lines = append(lines, diffmodel.DiffLine{Kind: "deletion", Raw: fmt.Sprintf("-x := old(%d)", i), OldLine: &o})
				case 1:
					lines = append(lines, diffmodel.DiffLine{Kind: "addition", Raw: fmt.Sprintf("+x := new(%d)", i), NewLine: &nn})
				default:
					lines = append(lines, diffmodel.DiffLine{Kind: "context", Raw: fmt.Sprintf(" ctx %d", i), OldLine: &o, NewLine: &nn})
				}
			}
			hunks = append(hunks, diffmodel.DiffHunk{Header: fmt.Sprintf("@@ -%d +%d @@", h*100+1, h*100+1), Lines: lines})
		}
		return makeFileWithHunks(path, hunks)
	}
	files := []diffmodel.DiffFile{mk("a.go", 12), mk("b.go", 9)}
	m := makePRDetail(160, 50, files, nil)
	m.Diff = makeDiff(files)
	m.SetTheme(theme.Default())
	m.activeTab = TabDiff
	m.leftPanel.Focus = FocusContent
	m.diffCursor = diffCursorLine{FileIdx: 0, HunkIdx: 1, LineIdx: 4}
	m.visual = visualModeState{Active: true, FileIdx: 1, HunkIdx: 0, StartLine: 2, EndLine: 5}
	m.draftCovered = map[hunkLineKey]bool{{0, 2, 1}: true}
	m.searchActive = true
	m.searchMatches = []diffsearch.Match{{FileIndex: 0, LineIndex: 3, StartCol: 1, EndCol: 2}, {FileIndex: 1, LineIndex: 40, StartCol: 1, EndCol: 2}}

	total := m.diffSectionRowCount()
	full := m.renderDiffSectionLines(0, total, 120)
	for start := 0; start < total; start += 3 {
		for _, h := range []int{1, 7, 25} {
			end := min(start+h, total)
			got := m.renderDiffSectionLines(start, end, 120)
			for i := range got {
				if got[i] != full[start+i] {
					t.Fatalf("window [%d,%d) row %d differs from full render:\n got %q\nwant %q", start, end, start+i, got[i], full[start+i])
				}
			}
		}
	}
}

// TestLongDiffLinesStayWithinContentWidth checks every diff row kind is
// clipped to the content width, so long lines never touch the panel border.
func TestLongDiffLinesStayWithinContentWidth(t *testing.T) {
	t.Parallel()
	long := " " + strings.Repeat("context ", 40)
	one, two, three := 1, 2, 3
	f := makeFileWithHunks("a.go", []diffmodel.DiffHunk{{Header: "@@ -1,3 +1,3 @@", Lines: []diffmodel.DiffLine{
		{Kind: "context", Raw: long, OldLine: &one, NewLine: &one},
		{Kind: "addition", Raw: "+" + long, NewLine: &two},
		{Kind: "deletion", Raw: "-" + long, OldLine: &three},
	}}})
	for _, themed := range []bool{true, false} {
		m := makePRDetail(160, 40, []diffmodel.DiffFile{f}, nil)
		m.Diff = makeDiff([]diffmodel.DiffFile{f})
		if themed {
			m.SetTheme(theme.Default())
		}
		for i, row := range m.renderDiffSectionLines(0, f.DisplayRows, 90) {
			if w := lipgloss.Width(row); w > 90 {
				t.Errorf("themed=%v row %d width %d exceeds content width 90", themed, i, w)
			}
		}
	}
}

// TestSelectedCheckRowIsBold checks the selected CI row keeps its name and
// status bold even though the coloured icon in front of it resets styling.
func TestSelectedCheckRowIsBold(t *testing.T) {
	t.Parallel()
	panel := LeftPanelModel{Checks: []domain.PreviewCheckRow{{Name: "build", State: "SUCCESS"}}, Focus: FocusCI}
	panel.SetTheme(theme.Default())
	row := panel.renderCIRow(panel.Checks[0], 0)
	want := panel.theme.Bold.Render(truncatePathLeft("build", lpCINameMax) + " " + formatCIStatus("SUCCESS"))
	if !strings.Contains(row, want) {
		t.Errorf("expected name and status rendered bold, got %q", row)
	}
}
