package sqlitecache

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
)

func BenchmarkCache(b *testing.B) {
	dm, err := parse.Parse(bench.SynthDiff(100, 500))
	if err != nil {
		b.Fatal(err)
	}
	type small struct {
		A string
		N int
	}
	cases := []struct {
		name string
		val  any
		dest func() any
	}{
		{"diff-50k", dm, func() any { return new(model.DiffModel) }},
		{"small", small{"hello", 42}, func() any { return new(small) }},
	}
	ctx := context.Background()
	meta := domain.CacheMeta{Kind: "preview", Host: "github.com", Repo: "acme/repo-00", FetchedAt: bench.Epoch}
	for _, c := range cases {
		s := benchCache(b)
		b.Run("Put/"+c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := s.Put(ctx, "k", c.val, meta); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("Get/"+c.name, func(b *testing.B) {
			if err := s.Put(ctx, "k", c.val, meta); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, ok, err := s.Get(ctx, "k", c.dest()); err != nil || !ok {
					b.Fatal(ok, err)
				}
			}
		})
	}
}

func benchCache(b *testing.B) *Cache {
	c, err := New(filepath.Join(b.TempDir(), "cache.db"), 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close() })
	return c
}
