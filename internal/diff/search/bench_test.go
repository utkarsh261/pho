package search

import (
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
)

func benchModels(b *testing.B) map[string]*model.DiffModel {
	b.Helper()
	out := map[string]*model.DiffModel{}
	for _, f := range difftest.RawDiffs() {
		if f.Name == "ts64457" {
			dm, err := parse.Parse(f.Raw)
			if err != nil {
				b.Fatal(err)
			}
			out["ts64457"] = dm
		}
	}
	dm, err := parse.Parse(bench.SynthDiff(200, 500))
	if err != nil {
		b.Fatal(err)
	}
	out["synth-100k"] = dm
	return out
}

func BenchmarkBuild(b *testing.B) {
	for name, dm := range benchModels(b) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Build(dm)
			}
		})
	}
}

func BenchmarkSearch(b *testing.B) {
	for name, dm := range benchModels(b) {
		idx := Build(dm)
		for _, q := range []string{"computeNewValue", "e"} {
			b.Run(name+"/"+q, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					idx.Search(q)
				}
			})
		}
	}
}
