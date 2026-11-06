package markdown

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
)

func BenchmarkRender(b *testing.B) {
	src := bench.Markdown(10, 1)
	for _, w := range []int{60, 120} {
		b.Run(fmt.Sprintf("cached/w%d", w), func(b *testing.B) {
			r := New()
			r.Render(src, w)
			b.ReportAllocs()
			for b.Loop() {
				r.Render(src, w)
			}
		})
		b.Run(fmt.Sprintf("varying/w%d", w), func(b *testing.B) {
			r := New()
			docs := make([]string, 8)
			for i := range docs {
				docs[i] = bench.Markdown(10, uint64(i+1))
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				r.cache = nil
				r.Render(docs[i%len(docs)], w)
				i++
			}
		})
	}
}
