package stackmap

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func testStack() *domain.Stack {
	pr := func(n int) *domain.PullRequestSummary {
		return &domain.PullRequestSummary{Number: n, CIStatus: domain.CIStatusSuccess}
	}
	return &domain.Stack{Trunk: "main", Members: []domain.StackMember{
		{Number: 3, Title: "Wire picker", Parent: 2, Layer: 3, Summary: pr(3)},
		{Number: 5, Title: "Side branch", Parent: 1, Layer: 2, Indent: 1, Summary: pr(5)},
		{Number: 2, Title: "Draw rail", Parent: 1, Layer: 2, Children: []int{3}, Summary: pr(2)},
		{Number: 1, Title: "Detect", Layer: 1, Children: []int{2, 5}, Summary: pr(1)},
	}}
}

func TestRowsMarkCurrentAndTrunk(t *testing.T) {
	t.Parallel()
	rows := Rows(theme.Default(), testStack(), Options{Width: 50, Current: 2, Cursor: -1, Indent: true})
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = strings.TrimRight(ansi.Strip(r), " ")
		if w := lipgloss.Width(r); w > 50 {
			t.Errorf("row %d is %d wide", i, w)
		}
	}
	for i, want := range []string{"  ○ #3 Wire picker", "    ○ #5 Side branch", "  ● #2 Draw rail", "  ○ #1 Detect", "  ╰ main"} {
		if !strings.HasPrefix(got[i], want) {
			t.Errorf("row %d = %q, want prefix %q", i, got[i], want)
		}
	}
}
