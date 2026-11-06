package prdetail

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func benchDiff(b *testing.B, raw string) *diffmodel.DiffModel {
	b.Helper()
	dm, err := parse.Parse(raw)
	if err != nil {
		b.Fatal(err)
	}
	return dm
}

func benchTS(b *testing.B) string {
	b.Helper()
	for _, f := range difftest.RawDiffs() {
		if f.Name == "ts64457" {
			return f.Raw
		}
	}
	b.Fatal("ts64457 fixture missing")
	return ""
}

func BenchmarkIntraLineChanges(b *testing.B) {
	for _, c := range []struct {
		name string
		dm   *diffmodel.DiffModel
	}{
		{"ts64457", benchDiff(b, benchTS(b))},
		{"synth-100k", benchDiff(b, bench.SynthDiff(200, 500))},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for fi := range c.dm.Files {
					for hi := range c.dm.Files[fi].Hunks {
						intraLineChanges(c.dm.Files[fi].Hunks[hi].Lines)
					}
				}
			}
		})
	}
}

func BenchmarkCommentLines(b *testing.B) {
	for _, n := range []int{30, 100} {
		m := makePRDetail(200, 50, nil, nil)
		m.Detail = &domain.PRPreviewSnapshot{Comments: bench.Comments(n, 1)}
		m.SetTheme(theme.Default())
		cw := m.contentW()
		b.Run(fmt.Sprintf("%dcomments", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m.commentLines(cw, -1)
			}
		})
	}
}
