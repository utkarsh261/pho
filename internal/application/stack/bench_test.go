package stack

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
)

func BenchmarkBuild(b *testing.B) {
	for _, n := range []int{100, 500, 2000} {
		prs := bench.PRs("acme/repo-00", n, 1)
		b.Run(fmt.Sprintf("%dprs", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Build(prs)
			}
		})
	}
}

func BenchmarkGroup(b *testing.B) {
	for _, n := range []int{100, 500, 2000} {
		prs := bench.PRs("acme/repo-00", n, 1)
		idx := Build(prs)
		b.Run(fmt.Sprintf("%dprs", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Group(prs, idx)
			}
		})
	}
}
