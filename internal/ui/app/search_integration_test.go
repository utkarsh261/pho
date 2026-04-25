package app

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainLine(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func makeSearchContextFile(path string, lines ...string) diffmodel.DiffFile {
	diffLines := make([]diffmodel.DiffLine, 0, len(lines))
	for _, line := range lines {
		diffLines = append(diffLines, diffmodel.DiffLine{Kind: "context", Raw: line})
	}
	return diffmodel.DiffFile{
		OldPath: path,
		NewPath: path,
		Status:  "modified",
		Hunks: []diffmodel.DiffHunk{{
			Header: "@@ -1,1 +1,1 @@",
			Lines:  diffLines,
		}},
	}
}

func pressRuneKey(t *testing.T, m *Model, r rune) {
	t.Helper()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

func pressKey(t *testing.T, m *Model, msg tea.KeyMsg) {
	t.Helper()
	_, _ = m.Update(msg)
}

func pressSpaceKey(t *testing.T, m *Model) {
	t.Helper()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
}

func typeQueryText(t *testing.T, m *Model, q string) {
	t.Helper()
	for _, r := range q {
		pressRuneKey(t, m, r)
	}
}

func typeQueryWithRealSpace(t *testing.T, m *Model, q string) {
	t.Helper()
	for _, r := range q {
		if r == ' ' {
			pressSpaceKey(t, m)
			continue
		}
		pressRuneKey(t, m, r)
	}
}

func findLineWith(view, needle string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(plainLine(line), needle) {
			return line
		}
	}
	return ""
}

func statusLineSnapshot(view string) string {
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(plainLine(lines[len(lines)-1]))
}

func openPRDetailWithDiff(t *testing.T, files []diffmodel.DiffFile, body string) (*Model, domain.Repository, domain.PullRequestSummary) {
	t.Helper()

	repo := testutil.Repo("acme/alpha")
	summary := pr(repo.FullName, 77, "Search integration")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{summary})
	m.focus = domain.FocusPRListPanel

	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab:     domain.TabMyPRs,
		Index:   0,
		Repo:    repo.FullName,
		Number:  summary.Number,
		Summary: summary,
	})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	diff := diffmodel.DiffModel{
		Repo:     repo.FullName,
		PRNumber: summary.Number,
		Files:    files,
		Stats:    diffmodel.DiffStats{TotalFiles: len(files)},
	}
	_, _ = m.Update(cmds.DiffLoaded{Diff: diff})
	if body != "" {
		_, _ = m.Update(cmds.PRDetailLoaded{
			Repo:   repo.FullName,
			Number: summary.Number,
			Detail: domain.PRPreviewSnapshot{
				Repo:        repo.FullName,
				Number:      summary.Number,
				Title:       summary.Title,
				BodyExcerpt: body,
			},
		})
	}

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	if m.prDetail == nil {
		t.Fatal("expected prDetail to be initialized")
	}
	return m, repo, summary
}

func TestSearchVirtualizationScrollToOffscreenMatch(t *testing.T) {
	files := make([]diffmodel.DiffFile, 0, 30)
	for i := 0; i < 30; i++ {
		lines := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}
		path := fmt.Sprintf("pkg/f%02d.go", i)
		if i == 29 {
			lines[4] = "needle offscreen target"
			path = "pkg/offscreen_match.go"
		}
		files = append(files, makeSearchContextFile(path, lines...))
	}

	m, _, _ := openPRDetailWithDiff(t, files, strings.Repeat("desc ", 30))
	m.prDetail.ContentScroll = 0

	pressRuneKey(t, m, '2')
	pressRuneKey(t, m, '/')
	typeQueryText(t, m, "needle")
	pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.prDetail.ContentScroll <= 0 {
		t.Fatalf("expected search Enter to scroll to off-screen match, got scroll=%d", m.prDetail.ContentScroll)
	}

	view := m.View()
	if !strings.Contains(view, "offscreen_match.go") {
		t.Fatalf("expected viewport to include offscreen target file after search scroll, got:\n%s", view)
	}
	if !strings.Contains(view, "1/1 matches") {
		t.Fatalf("expected status to show 1/1 matches, got:\n%s", view)
	}
}

func TestSearchMatchHighlightInRender(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	oldRenderer := lipgloss.DefaultRenderer()
	lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(
		os.Stdout,
		termenv.WithProfile(termenv.ANSI),
		termenv.WithUnsafe(),
	))
	defer lipgloss.SetDefaultRenderer(oldRenderer)

	files := []diffmodel.DiffFile{
		makeSearchContextFile("pkg/highlight.go",
			"plain line",
			"needle context one",
			"middle",
			"needle context two",
		),
	}
	m, _, _ := openPRDetailWithDiff(t, files, "")
	pressRuneKey(t, m, '2') // switch to Diff tab before rendering

	before := m.View()
	beforeLine := findLineWith(before, "needle context one")
	if beforeLine == "" {
		t.Fatal("expected to find target diff line before search")
	}
	beforeEsc := strings.Count(beforeLine, "\x1b[")

	pressRuneKey(t, m, '2')
	pressRuneKey(t, m, '/')
	typeQueryText(t, m, "needle")
	pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	after := m.View()
	afterLine := findLineWith(after, "needle context one")
	if afterLine == "" {
		t.Fatal("expected to find target diff line after search")
	}
	afterEsc := strings.Count(afterLine, "\x1b[")
	if afterEsc <= beforeEsc {
		t.Fatalf("expected more ANSI styling after search highlight, before=%d after=%d\nbefore=%q\nafter=%q", beforeEsc, afterEsc, beforeLine, afterLine)
	}
}

func TestSearchClearedOnEscThenSectionJumpWorks(t *testing.T) {
	files := []diffmodel.DiffFile{
		makeSearchContextFile("pkg/a.go", "needle one", "x", "y"),
		makeSearchContextFile("pkg/b.go", "z", "needle two", "w"),
	}
	m, _, _ := openPRDetailWithDiff(t, files, strings.Repeat("long description ", 40))

	pressRuneKey(t, m, '2')
	pressRuneKey(t, m, '/')
	typeQueryText(t, m, "needle")
	pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m.View(), "matches") {
		t.Fatal("expected search status text before Esc")
	}

	pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected to remain in PR detail after Esc clears search, got %s", m.currentView())
	}
	if strings.Contains(m.View(), "matches") {
		t.Fatalf("expected search status cleared after Esc, got:\n%s", m.View())
	}

	beforeN := m.prDetail.ContentScroll
	pressRuneKey(t, m, 'n')
	if m.prDetail.ContentScroll != beforeN {
		t.Fatalf("expected n to no-op after search clear, scroll before=%d after=%d", beforeN, m.prDetail.ContentScroll)
	}

	// Switch to Description then back to Diff to verify tab switching still works.
	pressRuneKey(t, m, '1')
	if m.prDetail.IsDiffTabActive() {
		t.Fatalf("expected switch to Description tab")
	}
	pressRuneKey(t, m, '2')
	if !m.prDetail.IsDiffTabActive() {
		t.Fatalf("expected switch back to Diff tab")
	}
}

func TestSearchEnterThenNAndNNavigateStatusSnapshots(t *testing.T) {
	files := []diffmodel.DiffFile{
		makeSearchContextFile("pkg/a.go", "alpha needle", "x"),
		makeSearchContextFile("pkg/b.go", "y", "beta needle"),
	}
	m, _, _ := openPRDetailWithDiff(t, files, "")

	pressRuneKey(t, m, '2')
	pressRuneKey(t, m, '/')
	typeQueryText(t, m, "needle")
	pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	s1 := statusLineSnapshot(m.View())
	if !strings.Contains(s1, "/ needle  1/2 matches") {
		t.Fatalf("snapshot 1 mismatch, got %q", s1)
	}

	pressRuneKey(t, m, 'n')
	s2 := statusLineSnapshot(m.View())
	if !strings.Contains(s2, "/ needle  2/2 matches") {
		t.Fatalf("snapshot 2 mismatch, got %q", s2)
	}
	if strings.Contains(s2, "/ needlen") {
		t.Fatalf("expected n navigation after Enter, but query was mutated: %q", s2)
	}

	pressRuneKey(t, m, 'N')
	s3 := statusLineSnapshot(m.View())
	if !strings.Contains(s3, "/ needle  1/2 matches") {
		t.Fatalf("snapshot 3 mismatch, got %q", s3)
	}
}

func TestSearchQueryWithSpaceKeyWorksSnapshot(t *testing.T) {
	files := []diffmodel.DiffFile{
		makeSearchContextFile("pkg/spaced.go", "alpha beta", "other"),
	}
	m, _, _ := openPRDetailWithDiff(t, files, "")

	pressRuneKey(t, m, '2')
	pressRuneKey(t, m, '/')
	typeQueryWithRealSpace(t, m, "alpha beta")
	pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	s := statusLineSnapshot(m.View())
	if !strings.Contains(s, "/ alpha beta  1/1 matches") {
		t.Fatalf("space-query snapshot mismatch, got %q", s)
	}
}
