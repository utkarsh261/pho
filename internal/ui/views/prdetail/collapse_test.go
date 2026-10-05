package prdetail

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
)

// collapseFixture builds a diff with one file per collapse rule:
//
//	0 src/a.go           small, expanded
//	1 package-lock.json  generated → collapsed
//	2 big.go             1,200 changed lines → collapsed
//	3 huge.go            6,000 changed lines → too large
//	4 nopatch.go         GitHub sent no patch → too large
//	5 z.go               small, expanded
func collapseFixture() string {
	var b strings.Builder
	file := func(path string, pairs int) {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,%d +1,%d @@\n", path, path, path, path, pairs+1, pairs+1)
		b.WriteString(" ctx\n")
		for i := range pairs {
			fmt.Fprintf(&b, "-old %d\n+new %d\n", i, i)
		}
	}
	file("src/a.go", 5)
	file("package-lock.json", 3)
	file("big.go", 600)
	file("huge.go", 3000)
	fmt.Fprintf(&b, "diff --git a/nopatch.go b/nopatch.go\n%s+16000 -0\n", parse.PatchUnavailableMarker)
	file("z.go", 2)
	return b.String()
}

func loadCollapseModel(t *testing.T, detail *domain.PRPreviewSnapshot) *PRDetailModel {
	t.Helper()
	dm, err := parse.Parse(collapseFixture())
	if err != nil {
		t.Fatal(err)
	}
	anchor.Generate(dm, placementSHA)
	m := makePRDetail(120, 40, nil, nil)
	m.PRService = &prServiceStub{}
	m.Detail = detail
	m.DiffLoading = true
	m, _ = m.Update(cmds.DiffLoaded{Repo: "owner/repo", Number: 1, Diff: *dm})
	m.leftPanel.Focus = FocusContent
	m.activeTab = TabDiff
	return m
}

type viewState struct {
	collapsed, tooLarge bool
	reason              collapseReason
}

func states(m *PRDetailModel) []viewState {
	out := make([]viewState, len(m.Diff.Files))
	for i := range out {
		v := m.views()[i]
		out[i] = viewState{v.collapsed, v.tooLarge, v.reason}
	}
	return out
}

func TestAutoCollapseRules(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	want := []viewState{
		{},
		{collapsed: true, reason: reasonGenerated},
		{collapsed: true, reason: reasonLarge},
		{collapsed: true, tooLarge: true, reason: reasonLarge},
		{collapsed: true, tooLarge: true, reason: reasonNoPatch},
		{},
	}
	got := states(m)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d (%s): %+v, want %+v", i, m.Diff.Files[i].NewPath, got[i], want[i])
		}
	}
	for i, c := range m.leftPanel.Collapsed {
		if c != want[i].collapsed {
			t.Errorf("left panel file %d collapsed=%v, want %v", i, c, want[i].collapsed)
		}
	}
}

func TestIsGeneratedPath(t *testing.T) {
	t.Parallel()
	for p, want := range map[string]bool{
		"package-lock.json":         true,
		"web/yarn.lock":             true,
		"go.sum":                    true,
		"vendor/github.com/x/y.go":  true,
		"a/node_modules/b/index.js": true,
		"dist/app.js":               true,
		"static/app.min.js":         true,
		"api/v1/service.pb.go":      true,
		"zz_generated.deepcopy.go":  true,
		"types_generated.go":        true,
		"src/vendors.go":            false,
		"internal/distance.go":      false,
		"README.md":                 false,
	} {
		if got := isGeneratedPath(p); got != want {
			t.Errorf("isGeneratedPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestFilesWithThreadsStartExpanded(t *testing.T) {
	t.Parallel()
	detail := &domain.PRPreviewSnapshot{ReviewThreads: []domain.PreviewReviewThread{
		{Path: "big.go", Line: 3}, {Path: "huge.go", Line: 3},
	}}
	got := states(loadCollapseModel(t, detail))
	if got[2].collapsed {
		t.Error("big.go has a review thread and must start expanded")
	}
	if !got[3].tooLarge || !got[3].collapsed {
		t.Error("huge.go stays too large even with a thread")
	}
}

func TestThreadsArrivingLaterExpandFile(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	detail := domain.PRPreviewSnapshot{ReviewThreads: []domain.PreviewReviewThread{{Path: "big.go", Line: 3}}}
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 1, Detail: detail})
	if m.fileCollapsed(2) {
		t.Fatal("big.go must expand once its review thread is known")
	}

	// After the user toggles anything, auto-collapse leaves state alone.
	m = loadCollapseModel(t, nil)
	m.setFileCollapsed(0, true)
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 1, Detail: detail})
	if !m.fileCollapsed(2) || !m.fileCollapsed(0) {
		t.Fatal("user collapse state must survive a detail reload")
	}
}

func TestRowBudgetCollapsesLargestFiles(t *testing.T) {
	t.Parallel()
	dm, _ := parse.Parse(difftest.RawDiffs()[1].Raw) // ts64457: ~19k lines
	m := makePRDetail(120, 40, nil, nil)
	m.Limits = DiffLimits{CollapseLines: 1 << 30, CollapseLineWidth: 1 << 30, MaxLines: 1 << 30, MaxLineWidth: 1 << 30, RowBudget: 2000}
	m.DiffLoading = true
	m, _ = m.Update(cmds.DiffLoaded{Repo: "owner/repo", Number: 1, Diff: *dm})
	expanded, budget := 0, 0
	smallestCollapsed, largestExpanded := 1<<30, 0
	for i := range m.Diff.Files {
		rows := diffFileDisplayRows(&m.Diff.Files[i])
		switch v := m.views()[i]; {
		case !v.collapsed:
			expanded += rows
			largestExpanded = max(largestExpanded, rows)
		case v.reason == reasonBudget:
			budget++
			smallestCollapsed = min(smallestCollapsed, rows)
		}
	}
	if expanded > 2000 || budget == 0 {
		t.Fatalf("expanded rows %d with %d files collapsed for the budget", expanded, budget)
	}
	if smallestCollapsed < largestExpanded {
		t.Errorf("collapsed a %d-row file while a %d-row file stays expanded", smallestCollapsed, largestExpanded)
	}
}

func TestZTogglesFileUnderCursor(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	m.setDiffCursor(diffCursorLine{FileIdx: 0, HunkIdx: 0, LineIdx: 2})
	m = pressKey(m, "z")
	if !m.fileCollapsed(0) || !m.diffCursor.isPlaceholder() || m.diffCursor.FileIdx != 0 {
		t.Fatalf("z must collapse the file and park the cursor on its placeholder, cursor=%+v", m.diffCursor)
	}
	m = pressKey(m, "z")
	if m.fileCollapsed(0) || m.diffCursor != (diffCursorLine{FileIdx: 0}) {
		t.Fatalf("z must expand the file and put the cursor on its first line, cursor=%+v", m.diffCursor)
	}
}

func TestZOnTooLargeFileShowsHint(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	m.setDiffCursor(diffCursorLine{FileIdx: 3, HunkIdx: placeholderLine, LineIdx: placeholderLine})
	m = pressKey(m, "z")
	if !m.fileCollapsed(3) {
		t.Fatal("a too-large file must not expand")
	}
	if !strings.Contains(m.StatusHint(), "O: open on GitHub") {
		t.Fatalf("status hint = %q", m.StatusHint())
	}
	m = pressKey(m, "j")
	if strings.Contains(m.StatusHint(), "open on GitHub") {
		t.Fatal("the hint must clear on the next key")
	}
}

func TestShiftZExpandsThenCollapsesAll(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	m = pressKey(m, "Z")
	want := []bool{false, false, false, true, true, false}
	for i, w := range want {
		if m.fileCollapsed(i) != w {
			t.Errorf("after Z: file %d collapsed=%v, want %v", i, m.fileCollapsed(i), w)
		}
	}
	m = pressKey(m, "Z")
	for i := range want {
		if !m.fileCollapsed(i) {
			t.Errorf("after second Z: file %d must be collapsed", i)
		}
	}
}

func TestCursorStopsOnPlaceholders(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	var stops []int
	for _, c := range m.navigableLines {
		if c.isPlaceholder() {
			stops = append(stops, c.FileIdx)
		}
	}
	if fmt.Sprint(stops) != "[1 2 3 4]" {
		t.Fatalf("placeholder stops = %v, want [1 2 3 4]", stops)
	}

	// Space, c and y on a placeholder do nothing and don't panic.
	m.setDiffCursor(diffCursorLine{FileIdx: 2, HunkIdx: placeholderLine, LineIdx: placeholderLine})
	m = pressKey(m, " ")
	if m.visual.Active {
		t.Fatal("visual mode must not start on a placeholder")
	}
	m = pressKey(m, "c")
	if m.compose.active {
		t.Fatal("compose must not open on a placeholder")
	}
	if cmd := m.emitCopyCommitPermalink(); cmd != nil {
		t.Fatal("no permalink for a placeholder")
	}
}

func TestJumpsExpandCollapsedFiles(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	m.jumpToFile(2)
	if m.fileCollapsed(2) {
		t.Fatal("jumping to a collapsed file must expand it")
	}
	if m.diffCursor.FileIdx != 2 || m.diffCursor.isPlaceholder() {
		t.Fatalf("cursor = %+v, want the first line of file 2", m.diffCursor)
	}
	if m.ContentScroll != m.rows().fileStart[2] {
		t.Fatalf("scroll = %d, want %d", m.ContentScroll, m.rows().fileStart[2])
	}

	m.jumpToFile(3)
	if !m.fileCollapsed(3) || m.diffCursor != (diffCursorLine{FileIdx: 3, HunkIdx: placeholderLine, LineIdx: placeholderLine}) {
		t.Fatalf("jumping to a too-large file must land on its placeholder, cursor=%+v", m.diffCursor)
	}
}

func TestJumpToCommentCodeExpandsCollapsedFile(t *testing.T) {
	t.Parallel()
	detail := &domain.PRPreviewSnapshot{ReviewThreads: []domain.PreviewReviewThread{{
		ID: "t1", Path: "src/a.go", Line: 3,
		Comments: []domain.PreviewThreadComment{{ID: "c1", Login: "bob", Body: "hi"}},
	}}}
	m := loadCollapseModel(t, detail)
	m.setFileCollapsed(0, true)
	m.activeTab = TabComments
	m.commentEntriesDirty = true
	entries := m.commentEntries()
	m.commentCursor = -1
	for i, e := range entries {
		if e.path == "src/a.go" {
			m.commentCursor = i
		}
	}
	if m.commentCursor < 0 {
		t.Fatal("thread entry not found")
	}
	m.jumpToCommentCode()
	if m.fileCollapsed(0) || m.diffCursor.FileIdx != 0 || m.diffCursor.isPlaceholder() {
		t.Fatalf("jump must expand src/a.go and land on its line, cursor=%+v", m.diffCursor)
	}
	path, line, _, ok := anchorForLine(&m.Diff.Files[0], &m.Diff.Files[0].Hunks[m.diffCursor.HunkIdx].Lines[m.diffCursor.LineIdx])
	if !ok || path != "src/a.go" || line != 3 {
		t.Fatalf("cursor on %s:%d", path, line)
	}
}

func TestSearchExpandsCollapsedAndSkipsTooLarge(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	m.searchActive = true
	m.searchQuery = "new 1"
	m.refreshSearchMatches()
	for _, mt := range m.searchMatches {
		if m.fileTooLarge(mt.FileIndex) {
			t.Fatalf("match in too-large file %d", mt.FileIndex)
		}
	}
	idx := -1
	for i, mt := range m.searchMatches {
		if mt.FileIndex == 2 {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("expected a match in big.go")
	}
	m.searchCursor = idx
	m.scrollToSearchCursor()
	if m.fileCollapsed(2) || m.diffCursor.FileIdx != 2 {
		t.Fatalf("search jump must expand big.go, cursor=%+v", m.diffCursor)
	}
}

func TestOpenFileOnGitHub(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	m.setDiffCursor(diffCursorLine{FileIdx: 3, HunkIdx: placeholderLine, LineIdx: placeholderLine})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("O")})
	if cmd == nil {
		t.Fatal("O must emit a command")
	}
	msg, ok := cmd().(OpenBrowserFile)
	if !ok || msg.Path != "huge.go" || msg.Number != 1 || msg.CommitSHA != "" {
		t.Fatalf("got %#v", cmd())
	}
}

// Every navigable stop must render on the row the cursor index says it is
// on, whatever is collapsed: the line's number for lines, the placeholder
// text for collapsed files.
func TestLayoutMatchesRenderedRows(t *testing.T) {
	t.Parallel()
	m := loadCollapseModel(t, nil)
	rng := rand.New(rand.NewSource(1))
	for trial := range 20 {
		for i := range m.Diff.Files {
			m.setFileCollapsed(i, rng.Intn(2) == 0)
		}
		checkLayout(t, m, trial)
	}
}

func checkLayout(t *testing.T, m *PRDetailModel, trial int) {
	t.Helper()
	lay := m.rows()
	row := 0
	for i := range m.Diff.Files {
		if lay.fileStart[i] != row {
			t.Fatalf("trial %d: file %d starts at %d, want %d", trial, i, lay.fileStart[i], row)
		}
		row += lay.fileRows[i]
	}
	if lay.total != row || m.diffSectionRowCount() != row {
		t.Fatalf("trial %d: total %d / section %d, want %d", trial, lay.total, m.diffSectionRowCount(), row)
	}
	all := m.renderDiffSectionLines(0, lay.total, 110)
	for i, c := range m.navigableLines {
		text := plainText(all[m.navigableRows[i]])
		if c.isPlaceholder() {
			if !strings.Contains(text, "lines") {
				t.Fatalf("trial %d: row %d for file %d placeholder shows %q", trial, m.navigableRows[i], c.FileIdx, text)
			}
			continue
		}
		dl := m.Diff.Files[c.FileIdx].Hunks[c.HunkIdx].Lines[c.LineIdx]
		if !strings.Contains(text, dl.Raw) || !strings.Contains(text, diffLineNumber(dl)) {
			t.Fatalf("trial %d: row %d shows %q, want line %q (%s)", trial, m.navigableRows[i], text, dl.Raw, diffLineNumber(dl))
		}
	}
}

// Collapsing other files must never change where a comment lands.
func TestPlacementUnchangedByCollapse(t *testing.T) {
	t.Parallel()
	fx := difftest.RawDiffs()[1] // ts64457
	golden := readAnchorGolden(t, fx.Name)
	m := loadPlacementModel(t, fx.Raw)
	rng := rand.New(rand.NewSource(7))
	for trial := range 8 {
		for i := range m.Diff.Files {
			m.setFileCollapsed(i, rng.Intn(3) == 0)
		}
		checked := 0
		for i, c := range m.navigableLines {
			if c.isPlaceholder() || i%11 != 0 {
				continue
			}
			want := golden[[3]int{c.FileIdx, c.HunkIdx, c.LineIdx}]
			m.drafts = nil
			m.setDiffCursor(c)
			m = pressKey(m, " ")
			m = pressKey(m, "c")
			if m.compose.active {
				m, _ = m.Update(submitComposeMsg{body: "x"})
			}
			m.compose.Close()
			if !want.ok {
				continue
			}
			if len(m.drafts) != 1 || m.drafts[0].Path != want.path || m.drafts[0].Line != want.line || m.drafts[0].Side != want.side {
				t.Fatalf("trial %d: %+v: drafts %+v, want %s %s:%d", trial, c, m.drafts, want.path, want.side, want.line)
			}
			checked++
		}
		if checked == 0 {
			t.Fatalf("trial %d: nothing checked", trial)
		}
	}
}

func TestLateThreadsNeverHideTheCurrentFile(t *testing.T) {
	t.Parallel()
	dm, _ := parse.Parse(difftest.RawDiffs()[1].Raw)
	m := makePRDetail(120, 40, nil, nil)
	m.PRService = &prServiceStub{}
	m.Limits = DiffLimits{CollapseLines: 1 << 30, CollapseLineWidth: 1 << 30, MaxLines: 1 << 30, MaxLineWidth: 1 << 30, RowBudget: 4000}
	m.DiffLoading = true
	m, _ = m.Update(cmds.DiffLoaded{Repo: "owner/repo", Number: 1, Diff: *dm})
	m.leftPanel.Focus = FocusContent
	m.activeTab = TabDiff
	// Put the cursor on the smallest expanded file, then pin every collapsed
	// file with threads so the budget must collapse something else.
	cur := -1
	var threads []domain.PreviewReviewThread
	for i := range m.Diff.Files {
		if m.fileCollapsed(i) {
			threads = append(threads, domain.PreviewReviewThread{Path: m.Diff.Files[i].NewPath})
		} else if !m.Diff.Files[i].IsBinary && len(m.Diff.Files[i].Hunks) > 0 &&
			(cur < 0 || diffFileDisplayRows(&m.Diff.Files[i]) > diffFileDisplayRows(&m.Diff.Files[cur])) {
			cur = i
		}
	}
	m.setDiffCursor(diffCursorLine{FileIdx: cur})
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 1, Detail: domain.PRPreviewSnapshot{ReviewThreads: threads}})
	if m.fileCollapsed(cur) {
		t.Fatal("the file under the cursor was collapsed by a late reload")
	}
}
