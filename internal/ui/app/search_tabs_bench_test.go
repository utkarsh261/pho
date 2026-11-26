package app

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/domain"
)

// benchSearchTabs is 40 repos × 50 PRs, plus n custom tabs that each find
// 100 PRs in the selected repo.
func benchSearchTabs(b *testing.B, n int) (*Model, *bench.Driver) {
	b.Helper()
	tabs := make([]domain.SearchTab, n)
	for i := range tabs {
		name := fmt.Sprintf("Tab %d", i)
		tabs[i] = domain.SearchTab{ID: domain.CustomTabID(name), Name: name, Query: "label:x"}
	}
	m, d := benchDashboardWith(b, 40, 50, nil, func(deps *Dependencies) { deps.SearchTabs = tabs })
	svc := m.deps.Dashboard.(*stubDashboardService)
	svc.searchTabs = map[string]map[domain.DashboardTab][]domain.PullRequestSummary{}
	for _, r := range m.state.Repos.Discovered {
		found := map[domain.DashboardTab][]domain.PullRequestSummary{}
		for i, t := range tabs {
			found[t.ID] = bench.PRs(r.FullName, 100, uint64(100+i))
		}
		svc.searchTabs[r.FullName] = found
	}
	d.Press("tab") // focus the PR list
	return m, d
}

// Cycling through every tab, loading each custom one on the way.
func BenchmarkFrameCustomTabsSwitch(b *testing.B) {
	for _, n := range []int{0, domain.MaxSearchTabs} {
		b.Run(fmt.Sprintf("custom-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			_, d := benchSearchTabs(b, n)
			d.Reset()
			for b.Loop() {
				d.Press(bench.Repeat("l", len(domain.BuiltinTabs)+n)...)
			}
			d.Report(b)
		})
	}
}

// Scrolling a custom tab's 100 PRs, against the All tab's 50.
func BenchmarkFrameCustomTabsScroll(b *testing.B) {
	for _, tab := range []string{"all", "custom"} {
		b.Run(tab, func(b *testing.B) {
			b.ReportAllocs()
			n := 0
			if tab == "custom" {
				n = 1
			}
			m, d := benchSearchTabs(b, n)
			d.Press(bench.Repeat("l", 3+2*n)...) // All is 4th; the custom tab is 6th
			want := domain.TabAll
			if n == 1 {
				want = domain.CustomTabID("Tab 0")
			}
			if m.prList.Active != want {
				b.Fatalf("on %q, want %q", m.prList.Active, want)
			}
			d.Reset()
			for b.Loop() {
				d.Press(bench.Repeat("j", 40)...)
				d.Press(bench.Repeat("k", 40)...)
			}
			d.Report(b)
		})
	}
}

// A dashboard rebuild also regroups every loaded custom tab.
func BenchmarkRebuildDashboardTabs(b *testing.B) {
	for _, n := range []int{0, domain.MaxSearchTabs} {
		b.Run(fmt.Sprintf("custom-%d", n), func(b *testing.B) {
			m, d := benchSearchTabs(b, n)
			d.Press(bench.Repeat("l", len(domain.BuiltinTabs)+n)...) // load every custom tab
			if len(m.searchResults) != n {
				b.Fatalf("loaded %d custom tabs, want %d", len(m.searchResults), n)
			}
			b.ReportAllocs()
			for b.Loop() {
				m.rebuildDashboardTabs()
			}
		})
	}
}
