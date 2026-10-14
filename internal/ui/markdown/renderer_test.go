package markdown_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
	"images",
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

// ── Colour golden tests ───────────────────────────────────────────────────────

// colourFixtures leaves out fixtures with fenced code: chroma picks between
// equally close 256-colour entries in random order, so highlighted code is
// not stable across runs.
var colourFixtures = []string{"headings", "bold_italic", "lists", "blockquotes", "links"}

var colourProfiles = []struct {
	name    string
	profile termenv.Profile
}{
	{"TrueColor", termenv.TrueColor},
	{"ANSI256", termenv.ANSI256},
	{"ANSI", termenv.ANSI},
	{"Ascii", termenv.Ascii},
}

// TestColourGoldenRendering keeps the escape codes, pinning the colours the
// renderer emits under each lipgloss colour profile. It changes the global
// profile, so it must not run in parallel.
func TestColourGoldenRendering(t *testing.T) {
	for _, p := range colourProfiles {
		for _, fix := range colourFixtures {
			t.Run(p.name+"/"+fix, func(t *testing.T) {
				prev := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(p.profile)
				t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

				got := strings.Join(New().Render(readFixture(t, fix+".md"), 80), "\n")

				goldenPath := filepath.Join("testdata", "golden", "ansi", p.name, fix+"_w80.ansi")
				if *update {
					if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
						t.Fatalf("mkdir golden dir: %v", err)
					}
					if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
						t.Fatalf("write golden %s: %v", goldenPath, err)
					}
					return
				}
				if want := string(readGolden(t, goldenPath)); got != want {
					t.Errorf("colour golden mismatch for %s under %s\ngot:\n%q\nwant:\n%q", fix, p.name, got, want)
				}
			})
		}
	}
}

// TestRenderFollowsProfileChange guards the cache: a render cached under one
// colour profile must not be served after the profile changes.
func TestRenderFollowsProfileChange(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	r := New()
	src := "# Title\n\nSome **bold** text."

	lipgloss.SetColorProfile(termenv.TrueColor)
	if got := strings.Join(r.Render(src, 80), "\n"); !strings.Contains(got, "\x1b[38;2;") {
		t.Fatalf("expected truecolor escapes, got %q", got)
	}

	lipgloss.SetColorProfile(termenv.Ascii)
	if got := strings.Join(r.Render(src, 80), "\n"); strings.Contains(got, "\x1b[") {
		t.Errorf("expected no escapes after switching to Ascii, got %q", got)
	}
}

// TestSetStyleSwitchesOutput checks that a new style re-renders cached
// markdown in that style, and that switching back restores the original.
func TestSetStyleSwitchesOutput(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	src := "# Title\n\nSome **bold** text."
	r := New()
	tokyo := strings.Join(r.Render(src, 80), "\n")

	r.SetStyle(styles.DraculaStyleConfig)
	dracula := strings.Join(r.Render(src, 80), "\n")
	if dracula == tokyo {
		t.Fatal("expected a different render after switching style")
	}

	r.SetStyle(styles.TokyoNightStyleConfig)
	if got := strings.Join(r.Render(src, 80), "\n"); got != tokyo {
		t.Errorf("switching back did not restore the original render\ngot:  %q\nwant: %q", got, tokyo)
	}
}

// TestSetStyleComparesByValue checks the renderer follows the style itself,
// not a name: a style differing in one colour re-renders, and an equal style
// built separately (new pointers, same values) changes nothing.
func TestSetStyleComparesByValue(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	src := "# Title\n\nSome **bold** text."
	r := New()
	before := strings.Join(r.Render(src, 80), "\n")

	tweaked := styles.TokyoNightStyleConfig
	red := "#FF0000"
	tweaked.Document.Color = &red
	r.SetStyle(tweaked)
	after := strings.Join(r.Render(src, 80), "\n")
	if after == before || !strings.Contains(after, "38;2;255;0;0") {
		t.Fatalf("a one-colour style change was ignored:\n%q", after)
	}

	same := styles.TokyoNightStyleConfig
	red2 := "#FF0000"
	same.Document.Color = &red2
	r.SetStyle(same)
	if got := strings.Join(r.Render(src, 80), "\n"); got != after {
		t.Errorf("an equal style changed the output:\n%q\nwant:\n%q", got, after)
	}
}

// TestRenderConcurrentWithSetStyle runs renders alongside style changes; run
// with -race. Afterwards the cache must hold output in the final style only.
func TestRenderConcurrentWithSetStyle(t *testing.T) {
	src := "# Title\n\nSome **bold** text."
	r := New()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				if (i+j)%5 == 0 {
					if j%2 == 0 {
						r.SetStyle(styles.DraculaStyleConfig)
					} else {
						r.SetStyle(styles.TokyoNightStyleConfig)
					}
				}
				r.Render(src, 60+j%3)
			}
		}()
	}
	wg.Wait()

	r.SetStyle(styles.DraculaStyleConfig)
	fresh := New()
	fresh.SetStyle(styles.DraculaStyleConfig)
	for _, w := range []int{60, 61, 62} {
		got, want := strings.Join(r.Render(src, w), "\n"), strings.Join(fresh.Render(src, w), "\n")
		if got != want {
			t.Errorf("width %d: cached output is not in the final style", w)
		}
	}
}

// TestColourGoldenCodeTrueColor pins highlighted fenced code at truecolor,
// where chroma's formatter has no nearest-colour ties to break (the lower
// profiles do, so they stay out of the colour goldens).
func TestColourGoldenCodeTrueColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	got := strings.Join(New().Render(readFixture(t, "code_blocks.md"), 80), "\n")
	goldenPath := filepath.Join("testdata", "golden", "ansi", "TrueColor", "code_blocks_w80.ansi")
	if *update {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}
	if want := string(readGolden(t, goldenPath)); got != want {
		t.Errorf("colour golden mismatch for code blocks\ngot:\n%q\nwant:\n%q", got, want)
	}
}
