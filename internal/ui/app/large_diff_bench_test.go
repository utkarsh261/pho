package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/bench"
	sqlite "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
)

func heapMB() float64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc) / 1e6
}

// BenchmarkLargeDiffLoad runs every stage between the raw diff arriving and
// the first frame, reporting each stage's share so a regression points at
// its stage.
func BenchmarkLargeDiffLoad(b *testing.B) {
	type fixture struct {
		name string
		raw  string
	}
	var fxs []fixture
	fxs = append(fxs, fixture{"ts64457", ts64457()})
	for _, sz := range []int{20000, 100000, 200000} {
		fxs = append(fxs, fixture{fmt.Sprintf("big-%dk", sz/1000), bench.SynthDiff(sz/500, 500)})
	}
	for _, sz := range []int{10000, 50000, 100000} {
		fxs = append(fxs, fixture{fmt.Sprintf("small-%dk", sz/1000), bench.SynthSmall(sz/100, 100)})
	}
	for _, fx := range fxs {
		b.Run(fx.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(fx.raw)))
			dir := b.TempDir()
			c, err := sqlite.New(filepath.Join(dir, "c.db"), 1)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = c.Close() })
			var st [7]time.Duration
			var heap float64
			lap := func(i int, t0 *time.Time) {
				st[i] += time.Since(*t0)
				*t0 = time.Now()
			}
			for b.Loop() {
				t0 := time.Now()
				dm, err := parse.Parse(fx.raw)
				if err != nil {
					b.Fatal(err)
				}
				lap(0, &t0)
				anchor.Generate(dm, "0123456789012345678901234567890123456789")
				lap(1, &t0)
				payload, _ := json.Marshal(dm)
				lap(2, &t0)
				var back diffmodel.DiffModel
				_ = json.Unmarshal(payload, &back)
				lap(3, &t0)
				pn := 42
				_ = c.Put(context.Background(), "k", dm, domain.CacheMeta{Kind: "diff", PRNumber: &pn})
				lap(4, &t0)

				payload, back = nil, diffmodel.DiffModel{}
				b.StopTimer()
				base := heapMB()
				m, _ := benchPRDetail(b, diffmodel.DiffModel{Repo: "acme/alpha", PRNumber: 123}, nil, "2")
				dm.Repo, dm.PRNumber = "acme/alpha", 123
				b.StartTimer()

				t0 = time.Now()
				_, _ = m.Update(cmds.DiffLoaded{Diff: *dm})
				lap(5, &t0)
				if v := m.View(); !strings.Contains(v, dm.Files[0].NewPath) {
					b.Fatal("diff not rendered")
				}
				lap(6, &t0)

				b.StopTimer()
				heap += heapMB() - base
				runtime.KeepAlive(m)
				b.StartTimer()
			}
			for i, name := range []string{"parse", "anchor", "marshal", "unmarshal", "sqlite", "diffloaded", "firstview"} {
				b.ReportMetric(float64(st[i].Microseconds())/1e3/float64(b.N), name+"-ms")
			}
			b.ReportMetric(heap/float64(b.N), "heap-MB")
		})
	}
}
