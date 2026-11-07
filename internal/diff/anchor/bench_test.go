package anchor

import (
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
)

func BenchmarkGenerate(b *testing.B) {
	mustParse := func(raw string) *model.DiffModel {
		dm, err := parse.Parse(raw)
		if err != nil {
			b.Fatal(err)
		}
		return dm
	}
	cases := []struct {
		name string
		dm   *model.DiffModel
	}{
		{"ts64457", mustParse(difftest.Raw("ts64457"))},
		{"synth-100k", mustParse(bench.SynthDiff(200, 500))},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Generate(c.dm, "abc123")
			}
		})
	}
}
