package app

import (
	"slices"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

func setupStack(t *testing.T) *Model {
	t.Helper()
	repo := testutil.Repo("acme/stack")
	mk := func(n int, head, base string) domain.PullRequestSummary {
		p := pr(repo.FullName, n, "layer "+head)
		p.HeadRefName, p.BaseRefName = head, base
		return p
	}
	// Recency order scatters the stack: #2, an unrelated #9, #1, #3.
	prs := []domain.PullRequestSummary{mk(2, "b", "a"), mk(9, "x", "main"), mk(1, "a", "main"), mk(3, "c", "b")}
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)
	m.focus = domain.FocusPRListPanel
	return m
}

func tabNumbers(m *Model, tab domain.DashboardTab) []int {
	var out []int
	for _, p := range m.state.Dashboard.PRsByTab[tab] {
		out = append(out, p.Number)
	}
	return out
}

func TestDashboardGroupsStacks(t *testing.T) {
	t.Parallel()
	m := setupStack(t)
	for _, tab := range []domain.DashboardTab{domain.TabMyPRs, domain.TabAll} {
		if got := tabNumbers(m, tab); !slices.Equal(got, []int{3, 2, 1, 9}) {
			t.Errorf("%s = %v, want the stack together top-first: [3 2 1 9]", tab, got)
		}
	}

	plain := setupStack(t)
	plain.deps.NoStackGrouping = true
	plain.rebuildDashboardTabs()
	if got := tabNumbers(plain, domain.TabAll); !slices.Equal(got, []int{2, 9, 1, 3}) {
		t.Errorf("ungrouped = %v", got)
	}
}
