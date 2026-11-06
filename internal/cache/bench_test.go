package cache

import (
	"context"
	"testing"
	"time"

	memorycache "github.com/utkarsh261/pho/internal/cache/memory"
	"github.com/utkarsh261/pho/internal/domain"
)

func BenchmarkStaleWhileRevalidate(b *testing.B) {
	type val struct {
		Title string
		Items []int
	}
	ctx := context.Background()
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	meta := domain.CacheMeta{Kind: "preview", Host: "github.com", Repo: "acme/r", FetchedAt: now, ExpiresAt: now.Add(time.Hour)}
	v := val{"title", make([]int, 200)}

	newCoord := func() *Coordinator {
		c := NewCoordinator(memorycache.NewJSONStore(1<<24), memorycache.NewJSONStore(1<<24), nil)
		c.Now = func() time.Time { return now }
		return c
	}

	b.Run("l1-hit-fresh", func(b *testing.B) {
		c := newCoord()
		if err := c.Write(ctx, "k", v, meta); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			var dest val
			if _, _, ok, err := c.StaleWhileRevalidate(ctx, "k", &dest, nil); err != nil || !ok {
				b.Fatal(ok, err)
			}
		}
	})
	b.Run("miss", func(b *testing.B) {
		c := newCoord()
		b.ReportAllocs()
		for b.Loop() {
			var dest val
			if _, _, ok, err := c.StaleWhileRevalidate(ctx, "k", &dest, nil); err != nil || ok {
				b.Fatal(ok, err)
			}
		}
	})
	b.Run("l2-hit-promote", func(b *testing.B) {
		c := newCoord()
		if err := c.L2.Put(ctx, "k", v, meta); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = c.L1.Delete(ctx, "k")
			var dest val
			if _, _, ok, err := c.StaleWhileRevalidate(ctx, "k", &dest, nil); err != nil || !ok {
				b.Fatal(ok, err)
			}
		}
	})
}
