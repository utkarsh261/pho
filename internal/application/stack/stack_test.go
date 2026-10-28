package stack

import (
	"slices"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

var t0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func pr(n int, head, base string, age int) domain.PullRequestSummary {
	return domain.PullRequestSummary{
		Repo: "o/r", Number: n, Title: head, State: domain.PRStateOpen,
		HeadRefName: head, BaseRefName: base, UpdatedAt: t0.Add(-time.Duration(age) * time.Hour),
	}
}

func numbers(st *domain.Stack) []int {
	var out []int
	for _, m := range st.Members {
		out = append(out, m.Number)
	}
	return out
}

func prNumbers(prs []domain.PullRequestSummary) []int {
	var out []int
	for _, p := range prs {
		out = append(out, p.Number)
	}
	return out
}

func TestBuildLinearChain(t *testing.T) {
	prs := []domain.PullRequestSummary{
		pr(3, "c", "b", 1), pr(9, "other", "main", 2), pr(1, "a", "main", 3), pr(2, "b", "a", 4),
	}
	idx := Build(prs)
	st := idx.Lookup("O/R", 2)
	if st == nil {
		t.Fatal("expected #2 in a stack")
	}
	if got := numbers(st); !slices.Equal(got, []int{3, 2, 1}) {
		t.Fatalf("members = %v, want top-first [3 2 1]", got)
	}
	if st.Trunk != "main" || st.Native || st.Height() != 3 {
		t.Fatalf("trunk=%q native=%v height=%d", st.Trunk, st.Native, st.Height())
	}
	if m, _ := st.Member(2); m.Parent != 1 || m.Layer != 2 || !slices.Equal(m.Children, []int{3}) {
		t.Fatalf("member #2 = %+v", m)
	}
	if idx.Lookup("o/r", 9) != nil {
		t.Fatal("#9 is not stacked")
	}
}

func TestBuildFork(t *testing.T) {
	prs := []domain.PullRequestSummary{
		pr(1, "a", "main", 1), pr(2, "b", "a", 1), pr(4, "b2", "a", 1), pr(5, "c", "b2", 1),
	}
	st := Build(prs).Lookup("o/r", 1)
	if got := numbers(st); !slices.Equal(got, []int{5, 4, 2, 1}) {
		t.Fatalf("members = %v, want [5 4 2 1]", got)
	}
	if m2, _ := st.Member(2); m2.Indent != 1 || m2.Layer != 2 {
		t.Fatalf("side branch #2 = %+v", m2)
	}
	if m1, _ := st.Member(1); !slices.Equal(m1.Children, []int{4, 2}) {
		t.Fatalf("root children = %v, want newest first", m1.Children)
	}
}

func TestBuildIgnoresCyclesForksAndClosed(t *testing.T) {
	cross := pr(7, "x", "main", 1)
	cross.IsCrossRepository = true
	closed := pr(11, "y", "main", 1)
	closed.State = domain.PRStateClosed
	prs := []domain.PullRequestSummary{
		pr(1, "a", "b", 1), pr(2, "b", "a", 1), // cycle
		cross, pr(8, "z", "x", 1), // based on a fork's head
		closed, pr(12, "w", "y", 1), // based on a closed PR
		pr(13, "self", "self", 1),
	}
	if idx := Build(prs); len(idx) != 0 {
		t.Fatalf("expected no stacks, got %d entries", len(idx))
	}
}

func TestBuildDuplicateHeadPrefersNewest(t *testing.T) {
	prs := []domain.PullRequestSummary{pr(1, "a", "main", 5), pr(2, "a", "main", 1), pr(3, "c", "a", 1)}
	st := Build(prs).Lookup("o/r", 3)
	if m, _ := st.Member(3); m.Parent != 2 {
		t.Fatalf("parent = %d, want the newer #2", m.Parent)
	}
}

func TestBuildNative(t *testing.T) {
	ns := &domain.NativeStack{Number: 4, Trunk: "main", Layers: []domain.NativeStackLayer{
		{Position: 1, Number: 20, Title: "base", State: domain.PRStateMerged},
		{Position: 2, Number: 21, Title: "mid"},
		{Position: 3, Number: 22, Title: "top (unloaded)", State: domain.PRStateOpen},
	}}
	mid := pr(21, "mid", "main", 1) // retargeted to main after #20 merged
	mid.NativeStack = ns
	// #30 is based on mid's branch, but mid is in a native stack.
	idx := Build([]domain.PullRequestSummary{mid, pr(30, "x", "mid", 1)})
	st := idx.Lookup("o/r", 21)
	if st == nil || !st.Native {
		t.Fatal("expected a native stack")
	}
	if got := numbers(st); !slices.Equal(got, []int{22, 21, 20}) {
		t.Fatalf("members = %v", got)
	}
	if idx.Lookup("o/r", 22) != st || idx.Lookup("o/r", 30) != nil {
		t.Fatal("unloaded layer should be indexed; #30 should not")
	}
	top, _ := st.Member(22)
	base, _ := st.Member(20)
	if top.Summary != nil || base.State != domain.PRStateMerged {
		t.Fatalf("top=%+v base=%+v", top, base)
	}
	if m, _ := st.Member(21); m.Summary == nil || m.Parent != 20 {
		t.Fatalf("mid = %+v", m)
	}
}

func TestGroup(t *testing.T) {
	prs := []domain.PullRequestSummary{
		pr(2, "b", "a", 1), pr(9, "other", "main", 2), pr(1, "a", "main", 3), pr(3, "c", "b", 4),
	}
	idx := Build(prs)
	if got := prNumbers(Group(prs, idx)); !slices.Equal(got, []int{3, 2, 1, 9}) {
		t.Fatalf("grouped = %v, want [3 2 1 9]", got)
	}
	// A lone member stays where it was.
	lone := []domain.PullRequestSummary{prs[1], prs[0]}
	if got := prNumbers(Group(lone, idx)); !slices.Equal(got, []int{9, 2}) {
		t.Fatalf("lone = %v", got)
	}
}
