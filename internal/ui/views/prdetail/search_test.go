package prdetail

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	diffsearch "github.com/utkarsh261/pho/internal/diff/search"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

func makeSearchFile(path string, lines ...string) diffmodel.DiffFile {
	diffLines := make([]diffmodel.DiffLine, 0, len(lines))
	for _, line := range lines {
		diffLines = append(diffLines, diffmodel.DiffLine{
			Kind: "context",
			Raw:  line,
		})
	}
	f := diffmodel.DiffFile{
		OldPath: path,
		NewPath: path,
		Status:  "modified",
		Hunks: []diffmodel.DiffHunk{{
			Header: "@@ -1,1 +1,1 @@",
			Lines:  diffLines,
		}},
	}
	f.DisplayRows = diffFileDisplayRows(&f)
	return f
}

func expectedSearchScroll(m *PRDetailModel, match diffsearch.Match) int {
	m.normalizeDiffRows()

	flatLineIndexWithinFile := m.matchDisplayOffsetWithinFile(match)
	matchDisplayRow := m.Diff.Files[match.FileIndex].StartRow + flatLineIndexWithinFile
	contentHeight := m.contentViewportHeight()

	return clamp(matchDisplayRow-contentHeight/2, 0, m.maxContentScroll())
}

func typeQuery(m *PRDetailModel, query string) *PRDetailModel {
	for _, r := range query {
		m = pressKey(m, string(r))
	}
	return m
}

func TestSearchActivatesOnSlash(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "alpha", "beta"),
	}
	m := makePRDetail(120, 30, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")

	if !m.searchActive {
		t.Fatal("expected searchActive=true after '/'")
	}
	if m.searchQuery != "" {
		t.Fatalf("expected empty search query at activation, got %q", m.searchQuery)
	}
	if len(m.searchMatches) != 0 {
		t.Fatalf("expected no matches at activation, got %d", len(m.searchMatches))
	}
	if m.searchCursor != 0 {
		t.Fatalf("expected searchCursor=0 at activation, got %d", m.searchCursor)
	}
}

func TestSearchQueryUpdatesMatches(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "return err", "ok"),
		makeSearchFile("b.go", "RETURN value", "other"),
	}
	m := makePRDetail(120, 30, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "RETURN")

	if m.searchQuery != "RETURN" {
		t.Fatalf("expected searchQuery=RETURN, got %q", m.searchQuery)
	}
	if len(m.searchMatches) < 2 {
		t.Fatalf("expected at least 2 matches for case-insensitive query, got %d", len(m.searchMatches))
	}
	if m.searchCursor != 0 {
		t.Fatalf("expected cursor reset to first match, got %d", m.searchCursor)
	}
}

func TestSearchQueryAllowsSpace(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "multi word keyword", "other"),
	}
	m := makePRDetail(120, 30, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "multi word")

	if m.searchQuery != "multi word" {
		t.Fatalf("expected spaced query preserved, got %q", m.searchQuery)
	}
	if len(m.searchMatches) == 0 {
		t.Fatal("expected matches for spaced query")
	}
}

func TestSearchZeroMatchesStatusBar(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "return err", "ok"),
	}
	m := makePRDetail(120, 30, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "xyzquux")

	if len(m.searchMatches) != 0 {
		t.Fatalf("expected zero matches, got %d", len(m.searchMatches))
	}

	query, idx, count, active := m.SearchStatusState()
	if !active {
		t.Fatal("expected search state to be active")
	}

	sb := dashboard.NewStatusBarModel()
	sb.SetRect(120)
	sb.SetSearchState(query, idx, count)
	view := sb.View()
	if !strings.Contains(view, "0 matches") {
		t.Fatalf("expected 0 matches in status bar, got %q", view)
	}
}

func TestSearchEnterScrollsToFirstMatch(t *testing.T) {
	t.Parallel()

	var files []diffmodel.DiffFile
	for i := 0; i < 10; i++ {
		lines := []string{"a", "b", "c", "d", "e", "f"}
		if i == 9 {
			lines[4] = "needle target line"
		}
		files = append(files, makeSearchFile(fmt.Sprintf("f%d.go", i), lines...))
	}

	m := makePRDetail(120, 20, files, nil)
	m.Detail = makeDetailWithBody(strings.Repeat("description ", 20))
	m.Diff = makeDiff(files)
	m.ContentScroll = 0

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "needle")
	m = pressKey(m, "enter")

	if len(m.searchMatches) == 0 {
		t.Fatal("expected at least one search match")
	}
	want := expectedSearchScroll(m, m.searchMatches[0])
	if m.ContentScroll != want {
		t.Fatalf("expected ContentScroll=%d after Enter, got %d", want, m.ContentScroll)
	}
}

func TestSearchNNextWrapsAtEnd(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "hit one", "x"),
		makeSearchFile("b.go", "y", "hit two"),
	}
	m := makePRDetail(120, 20, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "hit")
	m = pressKey(m, "enter")
	if len(m.searchMatches) < 2 {
		t.Fatalf("need at least 2 matches, got %d", len(m.searchMatches))
	}
	m.searchCursor = len(m.searchMatches) - 1

	m = pressKey(m, "n")

	if m.searchCursor != 0 {
		t.Fatalf("expected cursor wrap to 0, got %d", m.searchCursor)
	}
	want := expectedSearchScroll(m, m.searchMatches[0])
	if m.ContentScroll != want {
		t.Fatalf("expected ContentScroll=%d after wrapped n, got %d", want, m.ContentScroll)
	}
}

func TestSearchNPrevWrapsAtStart(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "hit one", "x"),
		makeSearchFile("b.go", "y", "hit two"),
	}
	m := makePRDetail(120, 20, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "hit")
	m = pressKey(m, "enter")
	if len(m.searchMatches) < 2 {
		t.Fatalf("need at least 2 matches, got %d", len(m.searchMatches))
	}
	m.searchCursor = 0

	m = pressKey(m, "N")

	wantCursor := len(m.searchMatches) - 1
	if m.searchCursor != wantCursor {
		t.Fatalf("expected cursor wrap to %d, got %d", wantCursor, m.searchCursor)
	}
	want := expectedSearchScroll(m, m.searchMatches[wantCursor])
	if m.ContentScroll != want {
		t.Fatalf("expected ContentScroll=%d after wrapped N, got %d", want, m.ContentScroll)
	}
}

func TestSearchNNoopWhenNoMatches(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "alpha", "beta"),
	}
	m := makePRDetail(120, 20, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "xyzquux")
	m = pressKey(m, "enter")
	m.ContentScroll = 7
	m.searchCursor = 0

	m = pressKey(m, "n")
	if m.ContentScroll != 7 || m.searchCursor != 0 {
		t.Fatalf("expected no-op n with no matches, scroll=%d cursor=%d", m.ContentScroll, m.searchCursor)
	}

	m = pressKey(m, "N")
	if m.ContentScroll != 7 || m.searchCursor != 0 {
		t.Fatalf("expected no-op N with no matches, scroll=%d cursor=%d", m.ContentScroll, m.searchCursor)
	}

	// searchActive=false must also be a no-op for n/N.
	m.searchActive = false
	m.ContentScroll = 11
	m = pressKey(m, "n")
	if m.ContentScroll != 11 {
		t.Fatalf("expected no-op n when searchActive=false, scroll=%d", m.ContentScroll)
	}
}

func TestSearchEscClearsAllState(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "needle line", "other"),
	}
	m := makePRDetail(120, 20, files, nil)
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "needle")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next
	if cmd != nil {
		t.Fatal("expected Esc in active search to clear search, not emit BackToDashboard")
	}

	if m.searchActive {
		t.Fatal("expected searchActive=false after Esc")
	}
	if m.searchQuery != "" {
		t.Fatalf("expected empty searchQuery after Esc, got %q", m.searchQuery)
	}
	if m.searchMatches != nil {
		t.Fatalf("expected nil searchMatches after Esc, got %d matches", len(m.searchMatches))
	}
	if m.searchCursor != 0 {
		t.Fatalf("expected searchCursor=0 after Esc, got %d", m.searchCursor)
	}
}

func TestSearchPersistsAcrossTabFocus(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "hit one", "x"),
		makeSearchFile("b.go", "hit two", "y"),
	}
	checks := []domain.PreviewCheckRow{
		{Name: "ci/check", State: "SUCCESS"},
	}
	m := makePRDetail(120, 20, files, checks)
	m.Diff = makeDiff(files)
	m.leftPanel.Focus = FocusFiles

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "hit")
	m = pressKey(m, "enter")
	m = pressKey(m, "n")

	beforeQuery := m.searchQuery
	beforeCursor := m.searchCursor
	beforeCount := len(m.searchMatches)

	m = pressKey(m, "tab")

	if !m.searchActive {
		t.Fatal("expected search to remain active across Tab focus change")
	}
	if m.searchQuery != beforeQuery {
		t.Fatalf("expected query preserved across Tab, got %q", m.searchQuery)
	}
	if m.searchCursor != beforeCursor {
		t.Fatalf("expected cursor preserved across Tab, got %d", m.searchCursor)
	}
	if len(m.searchMatches) != beforeCount {
		t.Fatalf("expected matches preserved across Tab, got %d", len(m.searchMatches))
	}
}

func TestSearchScrollAfterEachN(t *testing.T) {
	t.Parallel()

	files := []diffmodel.DiffFile{
		makeSearchFile("a.go", "target 1", "x", "target 2"),
		makeSearchFile("b.go", "x", "target 3", "x"),
	}
	m := makePRDetail(120, 18, files, nil)
	m.Detail = makeDetailWithBody(strings.Repeat("desc ", 20))
	m.Diff = makeDiff(files)

	m.activeTab = TabDiff
	m = pressKey(m, "/")
	m = typeQuery(m, "target")
	if len(m.searchMatches) < 3 {
		t.Fatalf("need at least 3 matches, got %d", len(m.searchMatches))
	}

	m = pressKey(m, "enter")
	want := expectedSearchScroll(m, m.searchMatches[m.searchCursor])
	if m.ContentScroll != want {
		t.Fatalf("expected ContentScroll=%d after Enter, got %d", want, m.ContentScroll)
	}

	for i := 0; i < len(m.searchMatches)+1; i++ {
		m = pressKey(m, "n")
		want = expectedSearchScroll(m, m.searchMatches[m.searchCursor])
		if m.ContentScroll != want {
			t.Fatalf("step %d: expected ContentScroll=%d after n, got %d", i, want, m.ContentScroll)
		}
	}
}
