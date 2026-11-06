package parse

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/difftest"
)

func BenchmarkParse(b *testing.B) {
	type tc struct{ name, raw string }
	var cases []tc
	for _, f := range difftest.RawDiffs() {
		cases = append(cases, tc{f.Name, f.Raw})
	}
	for _, n := range []int{20_000, 100_000, 200_000} {
		cases = append(cases, tc{fmt.Sprintf("synth-%dk", n/1000), bench.SynthDiff(n/500, 500)})
	}
	cases = append(cases, tc{"small-50k", bench.SynthSmall(100, 500)})
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.raw)))
			for b.Loop() {
				if _, err := Parse(c.raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
