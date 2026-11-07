package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utkarsh261/pho/internal/diff/difftest"
)

func BenchmarkFetchPRFiles(b *testing.B) {
	for _, name := range []string{"k8s142410", "ts64457"} {
		body := difftest.FilesJSON(name)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body)
		}))
		b.Cleanup(srv.Close)
		c := NewClient(srv.URL, "tok", nil)
		ctx := context.Background()
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				if _, err := c.FetchPRFiles(ctx, "o", "r", 1); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
