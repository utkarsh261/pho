package graphql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/utkarsh261/pho/internal/github/model"
)

func benchFixture(b *testing.B, name string) []byte {
	b.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "graphql", name, "response.json"))
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func BenchmarkNormalizeDashboard(b *testing.B) {
	data := benchFixture(b, "dashboard")
	var resp model.DashboardResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		b.Fatal(err)
	}
	repo := testRepo()
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var r model.DashboardResponse
			if err := json.Unmarshal(data, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("normalize", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, _, _, err := normalizeDashboardResponse(repo, resp.Data); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkNormalizePreview(b *testing.B) {
	data := benchFixture(b, "preview")
	var resp model.PreviewResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		b.Fatal(err)
	}
	repo := testRepo()
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var r model.PreviewResponse
			if err := json.Unmarshal(data, &r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("normalize", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := normalizePreviewResponse(repo, 142, resp.Data); err != nil {
				b.Fatal(err)
			}
		}
	})
}
