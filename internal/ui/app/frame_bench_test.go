package app

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/search"
	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func benchDiff(tb testing.TB, raw string) diffmodel.DiffModel {
	tb.Helper()
	dm, err := parse.Parse(raw)
	if err != nil {
		tb.Fatal(err)
	}
	anchor.Generate(dm, "0123456789012345678901234567890123456789")
	dm.Repo, dm.PRNumber = "acme/alpha", 123
	return *dm
}

func ts64457() string {
	for _, f := range difftest.RawDiffs() {
		if f.Name == "ts64457" {
			return f.Raw
		}
	}
	panic("ts64457 fixture missing")
}

// benchPRDetail opens PR #123 straight into its detail view at 200x50.
func benchPRDetail(tb testing.TB, dm diffmodel.DiffModel, comments []domain.PreviewComment, tab string) (*Model, *bench.Driver) {
	tb.Helper()
	detail := deeplinkPreviewFixture()
	detail.Comments = comments
	detail.FileCount = len(dm.Files)
	m := newDeepLinkTestModel(tb, 123, []domain.Repository{testutil.Repo("acme/alpha")},
		&stubPRService{detailResult: detail, diffResult: dm})
	m.SetTheme(theme.Default())
	d := bench.NewDriver(m)
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 50})
	for _, msg := range flattenCmd(m.Init()) {
		d.Send(msg)
	}
	if m.currentView() != domain.PrimaryViewPRDetail {
		tb.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	for _, msg := range bench.KeyMsgs(tab) {
		d.Send(msg)
	}
	return m, d
}

func BenchmarkFrameScroll(b *testing.B) {
	for _, fx := range []struct{ name, raw string }{
		{"ts64457", ts64457()},
		{"synth-200k", bench.SynthDiff(400, 500)},
	} {
		b.Run(fx.name, func(b *testing.B) {
			b.ReportAllocs()
			_, d := benchPRDetail(b, benchDiff(b, fx.raw), nil, "2")
			d.Send(tea.KeyMsg{Type: tea.KeyTab})
			script := append(bench.Repeat("j", 200), bench.Repeat("ctrl+d", 20)...)
			script = append(script, "G")
			script = append(script, bench.Repeat("k", 20)...)
			script = append(script, "gg")
			d.Reset()
			for b.Loop() {
				d.Press(script...)
			}
			d.Report(b)
		})
	}
}

func BenchmarkFrameSearchTyping(b *testing.B) {
	b.ReportAllocs()
	_, d := benchPRDetail(b, benchDiff(b, bench.SynthDiff(200, 500)), nil, "2")
	d.Send(tea.KeyMsg{Type: tea.KeyTab})
	d.Reset()
	for b.Loop() {
		d.Press("/", "computeNewValue", "enter")
		d.Press(bench.Repeat("n", 50)...)
		d.Press("esc", "gg")
	}
	d.Report(b)
}

func BenchmarkFrameTabHop(b *testing.B) {
	for _, n := range []int{30, 300} {
		b.Run(fmt.Sprintf("comments-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			_, d := benchPRDetail(b, benchDiff(b, ts64457()), bench.Comments(n, 1), "1")
			d.Reset()
			for b.Loop() {
				d.Press("2", "3", "4", "1")
			}
			d.Report(b)
		})
	}
}

// benchDashboard loads repos×perRepo PRs into the dashboard at 200x50.
func benchDashboard(tb testing.TB, repos, perRepo int, srch SearchService) (*Model, *bench.Driver) {
	tb.Helper()
	rs := bench.Repos(repos)
	dash := make(map[string]domain.DashboardSnapshot, len(rs))
	for i, r := range rs {
		prs := bench.PRs(r.FullName, perRepo, uint64(i))
		for j := range prs {
			prs[j].Author = "octocat"
		}
		dash[r.FullName] = dashboardSnapshot(r, prs...)
	}
	m := newTestModel(rs, dash)
	if srch != nil {
		m.deps.Search = srch
	}
	m.SetTheme(theme.Default())
	d := bench.NewDriver(m)
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 50})
	d.Send(cmdsReposDiscovered(rs))
	for _, r := range rs {
		d.Send(cmdsDashboardLoaded(r.FullName, dash[r.FullName], false, nil))
	}
	return m, d
}

func BenchmarkFrameDashboard(b *testing.B) {
	for _, sz := range []struct{ repos, per int }{{5, 20}, {40, 50}} {
		b.Run(fmt.Sprintf("repos-%d-prs-%d", sz.repos, sz.repos*sz.per), func(b *testing.B) {
			b.ReportAllocs()
			_, d := benchDashboard(b, sz.repos, sz.per, nil)
			d.Reset()
			for b.Loop() {
				d.Press(bench.Repeat("j", 30)...)
				d.Press(bench.Repeat("k", 30)...)
				d.Press("tab", "shift+tab")
			}
			d.Report(b)
		})
	}
}

func BenchmarkFramePalette(b *testing.B) {
	for _, n := range []int{1000, 20000} {
		b.Run(fmt.Sprintf("prs-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			srch := search.New()
			m, d := benchDashboard(b, 1, 20, srch)
			repo := bench.Repos(1)[0].FullName
			srch.AppendJumpPRs(repo, bench.PRs(repo, n, 7))
			srch.SetJumpIndexComplete(repo)
			_ = m
			d.Reset()
			for b.Loop() {
				d.Press("ctrl+p", "cache parser", "esc")
			}
			d.Report(b)
		})
	}
}

func BenchmarkFrameThemePreview(b *testing.B) {
	b.ReportAllocs()
	_, d := benchDashboard(b, 5, 20, nil)
	n := len(theme.Names())
	d.Reset()
	for b.Loop() {
		d.Press("T")
		d.Press(bench.Repeat("down", n)...)
		d.Press("esc")
	}
	d.Report(b)
}

func BenchmarkFrameResize(b *testing.B) {
	b.ReportAllocs()
	_, d := benchPRDetail(b, benchDiff(b, ts64457()), bench.Comments(30, 1), "2")
	d.Reset()
	for b.Loop() {
		for w := 60; w <= 240; w += 6 {
			d.Measure(tea.WindowSizeMsg{Width: w, Height: 30 + w%20})
		}
	}
	d.Report(b)
}
