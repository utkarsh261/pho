package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/utkarsh261/pho/internal/application/stack"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func stackedPRs() []domain.PullRequestSummary {
	mk := func(n int, title, head, base string) domain.PullRequestSummary {
		pr := makePR(n, title, head)
		pr.BaseRefName = base
		pr.State = domain.PRStateOpen
		return pr
	}
	return []domain.PullRequestSummary{
		mk(3, "Wire picker", "c", "b"),
		mk(2, "Draw rail", "b", "a"),
		mk(1, "Detect stacks", "a", "main"),
		mk(9, "Unrelated fix", "other", "main"),
	}
}

func listLines(t *testing.T, prs []domain.PullRequestSummary, idx domain.StackIndex, cursor int) []string {
	t.Helper()
	m := NewPRListPanelModel()
	m.SetTheme(theme.Default())
	m.SetTabSnapshot(domain.TabMyPRs, prs, len(prs), false)
	m.SetActiveTab(domain.TabMyPRs)
	m.SetStacks(idx)
	m.SetRect(70, 20)
	m.Cursor = cursor
	var out []string
	for _, l := range strings.Split(m.View(), "\n") {
		out = append(out, strings.TrimRight(ansi.Strip(l), " "))
	}
	return out
}

func TestPRListDrawsStackRail(t *testing.T) {
	t.Parallel()
	prs := stackedPRs()
	lines := listLines(t, prs, stack.Build(prs), 3)
	// Header, blank, tabs, blank, then 3 lines per row.
	rows := lines[4:]
	want := []string{
		" ● #3 Wire picker",
		" │  c",
		" │",
		" ● #2 Draw rail",
		" │  b",
		" │",
		" ● #1 Detect stacks",
		" ╰─ a",
		"",
		"▎● #9 Unrelated fix",
		"▎   other",
	}
	for i, w := range want {
		if !strings.HasPrefix(rows[i], w) {
			t.Fatalf("row line %d = %q, want prefix %q\n%s", i, rows[i], w, strings.Join(rows, "\n"))
		}
	}
	if !strings.HasSuffix(rows[7], "· onto main") {
		t.Errorf("bottom of stack = %q, want it to name the trunk", rows[7])
	}
}

func TestPRListLoneStackMemberShowsPosition(t *testing.T) {
	t.Parallel()
	prs := stackedPRs()
	idx := stack.Build(prs)
	lines := listLines(t, []domain.PullRequestSummary{prs[1], prs[3]}, idx, 0)
	if !strings.HasSuffix(lines[5], "· stack 2/3") {
		t.Errorf("lone member line = %q", lines[5])
	}
	if strings.Contains(strings.Join(lines, "\n"), "│") {
		t.Error("a lone member should not draw a rail")
	}
}

func TestPRListWithoutStacksUnchanged(t *testing.T) {
	t.Parallel()
	prs := []domain.PullRequestSummary{makePR(1, "One", "a"), makePR(2, "Two", "b")}
	plain := listLines(t, prs, nil, 0)
	indexed := listLines(t, prs, stack.Build(prs), 0)
	if strings.Join(plain, "\n") != strings.Join(indexed, "\n") {
		t.Errorf("unstacked list changed:\n%s\nvs\n%s", strings.Join(plain, "\n"), strings.Join(indexed, "\n"))
	}
}

func TestPreviewMapsTheStack(t *testing.T) {
	t.Parallel()
	prs := stackedPRs()
	p := NewPreviewPanelModel()
	p.SetTheme(theme.Default())
	p.SetStacks(stack.Build(prs))
	p.SetRect(60, 60)
	p.Update(SelectPRMsg{Repo: prs[1].Repo, Number: 2, Summary: prs[1]})
	view := ansi.Strip(p.View())
	for _, want := range []string{"Stack · 3 PRs onto main", "○ #3 Wire picker", "● #2 Draw rail", "○ #1 Detect stacks", "╰ main"} {
		if !strings.Contains(view, want) {
			t.Errorf("preview missing %q:\n%s", want, view)
		}
	}
}
