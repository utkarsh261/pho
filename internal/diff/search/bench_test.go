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
	for name, raw := range map[string]string{"ts64457": difftest.Raw("ts64457"), "synth-100k": bench.SynthDiff(200, 500)} {
		dm, err := parse.Parse(raw)
		if err != nil {
			b.Fatal(err)
		}
		out[name] = dm
	}
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
