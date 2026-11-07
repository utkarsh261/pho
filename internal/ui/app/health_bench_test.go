//go:build bench

package app

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/application/search"
	"github.com/utkarsh261/pho/internal/application/stack"
	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
)

// maxLinearSlope leaves room for cache effects before calling an op superlinear.
const maxLinearSlope = 1.3

// Lines starting with SCALING, HEAP or SOAK are picked up by
// scripts/bench/report.go.
func TestScaling(t *testing.T) {
	// setup builds the input for size n untimed; the returned func is timed.
	cases := []struct {
		name  string
		sizes []int
		setup func(n int) func()
	}{
		{"parse", []int{25_000, 50_000, 100_000, 200_000}, func(n int) func() {
			raw := bench.SynthDiff(n/500, 500)
			return func() { _, _ = parse.Parse(raw) }
		}},
		{"palette-match", []int{2_500, 5_000, 10_000, 20_000}, func(n int) func() {
			s := search.New()
			s.AppendJumpPRs("acme/repo-00", bench.PRs("acme/repo-00", n, 1))
			return func() { s.SearchPRsForRepo("cache parser", "acme/repo-00", 50) }
		}},
		{"stack-build", []int{250, 500, 1_000, 2_000}, func(n int) func() {
			prs := bench.PRs("acme/repo-00", n, 1)
			return func() { stack.Build(prs) }
		}},
		{"dashboard-load", []int{250, 500, 1_000, 2_000}, func(n int) func() {
			_, d := benchDashboard(t, 1, 1, nil)
			r := bench.Repos(1)[0]
			snap := dashboardSnapshot(r, bench.PRs(r.FullName, n, 1)...)
			return func() { d.Measure(cmdsDashboardLoaded(r.FullName, snap, false, nil)) }
		}},
		{"comments-render", []int{10, 20, 40, 80}, func(n int) func() {
			_, d := benchPRDetail(t, benchDiff(t, bench.SynthSmall(5, 100)), bench.Comments(n, 1), "1")
			return func() { d.Press("3", "1") }
		}},
	}
	for _, c := range cases {
		runs := make(map[int]func(), len(c.sizes))
		for _, n := range c.sizes {
			runs[n] = c.setup(n)
		}
		slope, times := bench.Slope(c.sizes, 3, func(n int) { runs[n]() })
		flag := ""
		if slope > maxLinearSlope {
			flag = " SUPERLINEAR"
		}
		t.Logf("SCALING %s slope=%.2f sizes=%v times=%v%s", c.name, slope, c.sizes, times, flag)
	}
}

func heapInuseMB() float64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapInuse) / 1e6
}

// TestHeapPlateau opens and closes PRs repeatedly; retained heap should stop
// growing once caches are warm.
func TestHeapPlateau(t *testing.T) {
	m, d := benchDashboard(t, 1, 60, nil)
	detail := deeplinkPreviewFixture()
	dm := benchDiff(t, difftest.Raw("ts64457"))
	m.deps.PR = &stubPRService{detailResult: detail, diffResult: dm}
	m.focus = domain.FocusPRListPanel
	var samples []float64
	for i := range 50 {
		d.Press("enter")
		if i == 0 && m.currentView() != domain.PrimaryViewPRDetail {
			t.Fatalf("enter did not open the PR, view %s", m.currentView())
		}
		d.Press("2", "j", "j")
		for range 3 {
			if m.currentView() == domain.PrimaryViewDashboard {
				break
			}
			d.Press("esc")
		}
		if i == 0 && m.currentView() != domain.PrimaryViewDashboard {
			t.Fatalf("esc did not close the PR, view %s", m.currentView())
		}
		d.Press("j")
		if (i+1)%10 == 0 {
			samples = append(samples, heapInuseMB())
		}
	}
	growth := samples[len(samples)-1] - samples[0]
	t.Logf("HEAP open-close-50 samples-MB=%.1f growth-MB=%.1f", samples, growth)
}

// TestMonkeySoak walks a seeded random key path through one PR; a later window much slower
// than the first means something accumulates per keypress.
func TestMonkeySoak(t *testing.T) {
	// Every key keeps the walk in PR detail, so windows compare like with like.
	keys := []string{"j", "j", "j", "k", "k", "h", "l", "tab", "gg", "G", "ctrl+d", "ctrl+u",
		"1", "2", "3", "4", "n", "N"}
	r := rand.New(rand.NewPCG(42, 42))
	m, d := benchPRDetail(t, benchDiff(t, difftest.Raw("ts64457")), bench.Comments(20, 1), "2")
	const total, window = 20_000, 1_000
	for i := range total {
		d.Press(keys[r.IntN(len(keys))])
		if m.currentView() != domain.PrimaryViewPRDetail {
			t.Fatalf("key %d left PR detail for %s", i, m.currentView())
		}
	}
	lat := d.Latencies()
	var p99s []time.Duration
	for i := 0; i+window <= len(lat); i += window {
		w := slices.Clone(lat[i : i+window])
		slices.Sort(w)
		p99s = append(p99s, w[window*99/100].Round(time.Microsecond))
	}
	drift := float64(p99s[len(p99s)-1]) / float64(p99s[0])
	t.Logf("SOAK keys=%d p99-per-1k=%v drift=%s", total, p99s, fmt.Sprintf("%.2fx", drift))
}
