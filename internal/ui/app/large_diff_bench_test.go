package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	sqlite "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

// synthDiff builds files with 60-line hunks (20 context, 20 del, 20 add).
func synthDiff(files, linesPerFile int) string {
	var b strings.Builder
	for f := 0; f < files; f++ {
		p := fmt.Sprintf("pkg/mod%d/file%d.go", f%50, f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1111111..2222222 100644\n--- a/%s\n+++ b/%s\n", p, p, p, p)
		n := 0
		h := 0
		for n < linesPerFile {
			start := h*100 + 1
			fmt.Fprintf(&b, "@@ -%d,40 +%d,40 @@ func something%d() {\n", start, start, h)
			for i := 0; i < 20 && n < linesPerFile; i++ {
				fmt.Fprintf(&b, " \tctx := doTheThing(%d, \"some context line here\", value)\n", i)
				n++
			}
			for i := 0; i < 20 && n < linesPerFile; i++ {
				fmt.Fprintf(&b, "-\tresult := computeOldValue(input%d, options.Legacy, 42)\n", i)
				n++
			}
			for i := 0; i < 20 && n < linesPerFile; i++ {
				fmt.Fprintf(&b, "+\tresult := computeNewValue(input%d, options.Modern, 43)\n", i)
				n++
			}
			h++
		}
	}
	return b.String()
}

type diffmodelAlias = diffmodel.DiffModel

// small hunks: 3 ctx, 3 del, 3 add, 3 ctx (12 lines), like real review diffs.
func synthSmall(files, linesPerFile int) string {
	var b strings.Builder
	for f := 0; f < files; f++ {
		p := fmt.Sprintf("pkg/mod%d/file%d.go", f%50, f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1111111..2222222 100644\n--- a/%s\n+++ b/%s\n", p, p, p, p)
		for h := 0; h*12 < linesPerFile; h++ {
			start := h*40 + 1
			fmt.Fprintf(&b, "@@ -%d,9 +%d,9 @@ func f%d() {\n", start, start, h)
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, " \tctx := doTheThing(%d, value)\n", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, "-\tresult := computeOldValue(input%d, options.Legacy)\n", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, "+\tresult := computeNewValue(input%d, options.Modern)\n", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, " \treturn x%d\n", i)
			}
		}
	}
	return b.String()
}

func heapMB() float64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc) / 1e6
}

func benchOne(t *testing.T, name, raw string) {
	base := heapMB()
	tot := time.Now()
	t0 := time.Now()
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	tParse := time.Since(t0)
	t0 = time.Now()
	anchor.Generate(dm, "0123456789012345678901234567890123456789")
	tAnchor := time.Since(t0)
	lines := 0
	for _, f := range dm.Files {
		for _, h := range f.Hunks {
			lines += len(h.Lines)
		}
	}
	t0 = time.Now()
	payload, _ := json.Marshal(dm)
	tMarshal := time.Since(t0)
	t0 = time.Now()
	var back diffmodelAlias
	_ = json.Unmarshal(payload, &back)
	tUnmarshal := time.Since(t0)
	t0 = time.Now()
	dir := t.TempDir()
	c, err := sqlite.New(filepath.Join(dir, "c.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	pn := 42
	_ = c.Put(context.Background(), "k", dm, domain.CacheMeta{Kind: "diff", PRNumber: &pn})
	tSqlite := time.Since(t0)
	_ = c.Close()
	heapAfterParse := heapMB() - base

	repo := testutil.Repo("acme/large")
	summary := pr(repo.FullName, 42, "Large")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{summary})
	m.focus = domain.FocusPRListPanel
	_, _ = m.Update(dashboard.SelectPRMsg{Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 42, Summary: summary})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	dm.Repo, dm.PRNumber = repo.FullName, 42

	t0 = time.Now()
	_, _ = m.Update(cmds.DiffLoaded{Diff: *dm})
	tUpdate := time.Since(t0)
	t0 = time.Now()
	_ = m.View()
	tView := time.Since(t0)
	tTotalLocal := time.Since(tot)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	t0 = time.Now()
	for i := 0; i < 50; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		_ = m.View()
	}
	tScroll := time.Since(t0) / 50
	t0 = time.Now()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	_ = m.View()
	tG := time.Since(t0)
	t0 = time.Now()
	for i := 0; i < 20; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		_ = m.View()
	}
	tScrollEnd := time.Since(t0) / 20
	heapUI := heapMB() - base

	t.Logf("%-14s files=%5d lines=%7d raw=%6.1fMB json=%6.1fMB | parse=%-8v anchor=%-8v marshal=%-8v unmarshal=%-8v sqlite=%-8v | DiffLoaded=%-8v firstView=%-8v | localTotal=%-8v | j+view=%-8v G=%-8v k@end=%-8v | heap(model)=%5.0fMB heap(ui)=%5.0fMB",
		name, len(dm.Files), lines, float64(len(raw))/1e6, float64(len(payload))/1e6,
		tParse.Round(time.Millisecond), tAnchor.Round(time.Millisecond), tMarshal.Round(time.Millisecond), tUnmarshal.Round(time.Millisecond), tSqlite.Round(time.Millisecond),
		tUpdate.Round(time.Millisecond), tView.Round(time.Millisecond), tTotalLocal.Round(time.Millisecond),
		tScroll.Round(time.Microsecond*100), tG.Round(time.Millisecond), tScrollEnd.Round(time.Microsecond*100), heapAfterParse, heapUI)
	runtime.KeepAlive(m)
}

// TestLargeDiffBenchmark times each stage of loading a large diff. It only
// runs with PHO_BENCH=1; PHO_BENCH_FILES adds comma-separated raw diff files.
func TestLargeDiffBenchmark(t *testing.T) {
	if os.Getenv("PHO_BENCH") == "" {
		t.Skip("set PHO_BENCH=1 to run")
	}
	for _, p := range strings.Split(os.Getenv("PHO_BENCH_FILES"), ",") {
		if p == "" {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		benchOne(t, filepath.Base(p), string(raw))
	}
	for _, sz := range []int{20000, 50000, 100000, 150000, 200000} {
		benchOne(t, fmt.Sprintf("big-%dk", sz/1000), synthDiff(sz/500, 500))
	}
	for _, sz := range []int{10000, 20000, 30000, 50000, 75000, 100000} {
		benchOne(t, fmt.Sprintf("small-%dk", sz/1000), synthSmall(sz/100, 100))
	}
}
