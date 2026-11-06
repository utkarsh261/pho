package dashboard

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
)

func BenchmarkClassify(b *testing.B) {
	for _, n := range []int{500, 2000} {
		prs := bench.PRs("acme/repo-00", n, 1)
		b.Run(fmt.Sprintf("%dprs", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				DefaultSummaryTabClassifier{}.Classify("alice", prs)
			}
		})
	}
}
