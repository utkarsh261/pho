package markdown_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "github.com/utkarsh261/pho/internal/ui/markdown"
)

// Run with -update to regenerate golden files:
//
//	go test ./internal/ui/markdown/... -update
var update = flag.Bool("update", false, "overwrite golden files with current output")

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// ── Golden file tests ─────────────────────────────────────────────────────────

var fixtures = []string{
	"headings",
	"bold_italic",
	"code_blocks",
	"lists",
	"blockquotes",
	"links",
	"mixed",
}

var widths = []int{60, 80, 120}

func TestGoldenRendering(t *testing.T) {
	for _, fix := range fixtures {
		for _, w := range widths {
			fix, w := fix, w
			t.Run(fmt.Sprintf("%s_w%d", fix, w), func(t *testing.T) {
				t.Parallel()

				src := readFixture(t, fix+".md")
				r := New()
				lines := r.Render(src, w)
				got := stripANSI(strings.Join(lines, "\n"))

				goldenPath := filepath.Join("testdata", "golden", fmt.Sprintf("%s_w%d.txt", fix, w))
				if *update {
					if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
						t.Fatalf("write golden %s: %v", goldenPath, err)
					}
					return
				}

				want := string(readGolden(t, goldenPath))
				if got != want {
					t.Errorf("golden mismatch for %s at width %d\ngot:\n%s\nwant:\n%s", fix, w, got, want)
				}
			})
		}
	}
}

// ── Edge-case unit tests ──────────────────────────────────────────────────────

func TestRenderEmpty(t *testing.T) {
	r := New()
	got := r.Render("", 80)
	if len(got) != 1 || got[0] != "" {
		t.Errorf("expected []string{\"\"}, got %v", got)
	}
}

func TestRenderWhitespaceOnly(t *testing.T) {
	r := New()
	got := r.Render("   \n  \t  \n", 80)
	if len(got) != 1 || got[0] != "" {
		t.Errorf("expected []string{\"\"}, got %v", got)
	}
}

func TestRenderLongSingleWord(t *testing.T) {
	r := New()
	word := strings.Repeat("x", 200)
	got := r.Render(word, 60)
	if len(got) == 0 {
		t.Error("expected at least one line for a long single word")
	}
}

func TestRenderWidthAdapts(t *testing.T) {
	r := New()
	src := strings.Repeat("This is a sentence that should wrap differently at different terminal widths. ", 5)

	narrow := r.Render(src, 60)
	wide := r.Render(src, 120)

	maxNarrow := 0
	for _, l := range narrow {
		if len(stripANSI(l)) > maxNarrow {
			maxNarrow = len(stripANSI(l))
		}
	}
	maxWide := 0
	for _, l := range wide {
		if len(stripANSI(l)) > maxWide {
			maxWide = len(stripANSI(l))
		}
	}
	if maxWide <= maxNarrow {
		t.Errorf("expected wider lines at width 120 (max %d) vs width 60 (max %d)", maxWide, maxNarrow)
	}
}

func TestRenderNoTrailingNewline(t *testing.T) {
	r := New()
	lines := r.Render("# Hello\n\nSome paragraph.", 80)
	for i, l := range lines {
		if strings.HasSuffix(l, "\n") {
			t.Errorf("line %d ends with newline: %q", i, l)
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func readGolden(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to generate)", path, err)
	}
	return data
}
