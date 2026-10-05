package prdetail

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func TestChangedRangesHighlightsEditedWord(t *testing.T) {
	t.Parallel()
	oldR, newR, ok := changedRanges("-\treturn fetchUser(id)", "+\treturn fetchAccount(id)")
	if !ok {
		t.Fatal("expected an in-line edit to be detected")
	}
	if got := "-\treturn fetchUser(id)"[oldR.start:oldR.end]; got != "fetchUser" {
		t.Errorf("old range = %q, want %q", got, "fetchUser")
	}
	if got := "+\treturn fetchAccount(id)"[newR.start:newR.end]; got != "fetchAccount" {
		t.Errorf("new range = %q, want %q", got, "fetchAccount")
	}
}

func TestChangedRangesSkipsRewrites(t *testing.T) {
	t.Parallel()
	if _, _, ok := changedRanges("-completely different", "+nothing alike here"); ok {
		t.Error("expected a whole-line rewrite not to be highlighted")
	}
	if _, _, ok := changedRanges("-same", "+same"); ok {
		t.Error("expected identical text not to be highlighted")
	}
}

func TestIntraLineChangesPairsDeletionsWithAdditions(t *testing.T) {
	t.Parallel()
	lines := []diffmodel.DiffLine{
		{Kind: "context", Raw: " a := 1"},
		{Kind: "deletion", Raw: "-b := compute(x)"},
		{Kind: "deletion", Raw: "-c := 2"},
		{Kind: "addition", Raw: "+b := compute(y)"},
		{Kind: "addition", Raw: "+c := 3"},
		{Kind: "addition", Raw: "+d := new()"}, // unpaired: no range
	}
	got := intraLineChanges(lines)
	for _, idx := range []int{1, 2, 3, 4} {
		if _, ok := got[idx]; !ok {
			t.Errorf("expected a changed range for line %d", idx)
		}
	}
	for _, idx := range []int{0, 5} {
		if _, ok := got[idx]; ok {
			t.Errorf("expected no changed range for line %d", idx)
		}
	}
	if r := got[3]; lines[3].Raw[r.start:r.end] != "y" {
		t.Errorf("line 3 range = %q, want %q", lines[3].Raw[r.start:r.end], "y")
	}
}

func TestDiffStatBlocks(t *testing.T) {
	t.Parallel()
	th := theme.Default()
	cases := []struct {
		add, del   int
		green, red int
	}{
		{10, 0, 5, 0},
		{0, 10, 0, 5},
		{5, 5, 3, 2},   // rounds half up
		{100, 1, 4, 1}, // any deletion keeps at least one red square
		{1, 100, 1, 4}, // any addition keeps at least one green square
	}
	for _, c := range cases {
		got := descStripANSI(diffStatBlocks(c.add, c.del, th))
		if n := len([]rune(got)); n != 5 {
			t.Errorf("+%d -%d: expected 5 squares, got %d", c.add, c.del, n)
		}
		want := th.Additions.Render(repeatSquares(c.green)) + th.Deletions.Render(repeatSquares(c.red))
		if diffStatBlocks(c.add, c.del, th) != want {
			t.Errorf("+%d -%d: expected %d green / %d red", c.add, c.del, c.green, c.red)
		}
	}
}

func repeatSquares(n int) string {
	s := ""
	for range n {
		s += "■"
	}
	return s
}

func TestStatusModeOnlyOnDiffTab(t *testing.T) {
	t.Parallel()
	m := makePRDetail(120, 40, nil, nil)
	m.activeTab = TabDiff
	if got := m.StatusMode(); got != "" {
		t.Errorf("normal browsing: mode = %q, want empty", got)
	}
	m.visual.Active = true
	if got := m.StatusMode(); got != "VISUAL" {
		t.Errorf("visual mode on Diff tab: mode = %q, want VISUAL", got)
	}
	m.visual.Active = false
	m.searchActive = true
	if got := m.StatusMode(); got != "SEARCH" {
		t.Errorf("search on Diff tab: mode = %q, want SEARCH", got)
	}
	m.searchActive = false
	m.compose.Open(composeModeNew, commentEntry{}, 0)
	if got := m.StatusMode(); got != "" {
		t.Errorf("composing: mode = %q, want empty (the compose box labels itself)", got)
	}
	m.compose.Close()
	for _, tab := range []ContentTab{TabDescription, TabComments, TabCommits} {
		m.activeTab = tab
		m.searchActive = true
		m.mergeStep = mergeStepConfirm
		if got := m.StatusMode(); got != "" {
			t.Errorf("tab %d: mode = %q, want empty off the Diff tab", tab, got)
		}
	}
}

func TestChangedRangesStaysOnRuneBoundaries(t *testing.T) {
	t.Parallel()
	oldRaw, newRaw := `-x := "café"`, `+x := "cafè"`
	oldR, newR, ok := changedRanges(oldRaw, newRaw)
	if !ok {
		t.Fatal("expected the accent change to be detected")
	}
	for _, c := range []struct {
		raw string
		r   byteRange
	}{{oldRaw, oldR}, {newRaw, newR}} {
		for _, part := range []string{c.raw[:c.r.start], c.raw[c.r.start:c.r.end], c.raw[c.r.end:]} {
			if !utf8.ValidString(part) {
				t.Errorf("range %v splits a rune in %q: part %q", c.r, c.raw, part)
			}
		}
	}
	if got := newRaw[newR.start:newR.end]; got != "cafè" {
		t.Errorf("new range = %q, want the whole word %q", got, "cafè")
	}
}

func TestDiffGutterIsAlwaysFixedWidth(t *testing.T) {
	t.Parallel()
	m := makePRDetail(120, 40, nil, nil)
	m.SetTheme(theme.Default())
	cases := map[string]string{
		"":       "     ",
		"7":      "   7 ",
		"9999":   "9999 ",
		"12345":  "12345",
		"123456": "…3456",
		" ⋯":     "   ⋯ ",
	}
	for num, wantField := range cases {
		for _, marker := range []string{"", "cursor", "draft"} {
			g := m.diffGutter(marker, num)
			if w := lipgloss.Width(g); w != diffGutterWidth {
				t.Errorf("diffGutter(%q, %q) width = %d, want %d", marker, num, w, diffGutterWidth)
			}
			if got := []rune(descStripANSI(g)); string(got[1:]) != wantField {
				t.Errorf("diffGutter(%q, %q) field = %q, want %q", marker, num, string(got[1:]), wantField)
			}
		}
	}
}

func TestDraftedAdditionTintsWholeRow(t *testing.T) {
	t.Parallel()
	one := 1
	f := makeFileWithHunks("a.go", []diffmodel.DiffHunk{{
		Header: "@@ -0,0 +1,1 @@",
		Lines:  []diffmodel.DiffLine{{Kind: "addition", Raw: "+added", NewLine: &one}},
	}})
	m := makePRDetail(120, 40, []diffmodel.DiffFile{f}, nil)
	m.Diff = makeDiff([]diffmodel.DiffFile{f})
	m.SetTheme(theme.Default())
	m.draftCovered = map[hunkLineKey]bool{{0, 0, 0}: true}

	row := m.renderDiffSectionLines(0, f.DisplayRows, 80)[4]
	bgSeq := func(c lipgloss.Color) string {
		p := lipgloss.NewStyle().Background(c).Render("x")
		return p[:strings.Index(p, "x")]
	}
	if seq := bgSeq(m.theme.Subtle); seq != "" && !strings.HasPrefix(row, seq) {
		t.Errorf("expected the draft tint from the first cell, got %q", row)
	}
	if seq := bgSeq(m.theme.DiffAddBg); seq != "" && strings.Contains(row, seq) {
		t.Errorf("drafted row should not keep the addition background, got %q", row)
	}
	if !strings.Contains(descStripANSI(row), "●") {
		t.Errorf("expected the draft marker in the gutter, got %q", descStripANSI(row))
	}
}
