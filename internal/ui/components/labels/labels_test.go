package labels

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func ls(names ...string) []domain.Label {
	out := make([]domain.Label, len(names))
	for i, n := range names {
		out[i] = domain.Label{Name: n, Color: "d73a4a"}
	}
	return out
}

func TestStrip(t *testing.T) {
	t.Parallel()
	three := ls("bug", "area/diff", "needs-review")
	tests := []struct {
		name   string
		labels []domain.Label
		total  int
		width  int
		want   string
	}{
		{"none", nil, 0, 80, ""},
		{"all fit", three, 3, 80, "● bug  ● area/diff  ● needs-review"},
		{"exact fit", three, 3, 34, "● bug  ● area/diff  ● needs-review"},
		{"overflow folds into +N", three, 3, 33, "● bug  ● area/diff  +1"},
		{"total counts unfetched labels", three, 25, 80, "● bug  ● area/diff  ● needs-review  +22"},
		{"stale zero total falls back to len", three, 0, 80, "● bug  ● area/diff  ● needs-review"},
		{"nothing fits shows count", three, 3, 8, "3 labels"},
		{"singular count", ls("a-very-long-label"), 1, 10, "1 label"},
		{"count doesn't fit either", three, 3, 4, ""},
		{"zero width", three, 3, 0, ""},
		{"negative width", three, 3, -5, ""},
		{"only unfetched labels", nil, 4, 80, "4 labels"},
		{"long name truncated", ls("needs-backport-to-release-branch-1.x", "bug"), 2, 80, "● needs-backport-to-relea…  ● bug"},
		{"blank names skipped but counted", ls("bug", "  ", ""), 3, 80, "● bug  +2"},
		{"bidi override dropped", ls("\u202ereversed"), 1, 80, "● reversed"},
		{"zero-width-only name skipped", ls("bug", "\u200b\u200b", "\u200d"), 3, 80, "● bug  +2"},
		{"emoji joiner kept", ls("👩\u200d💻 dev"), 1, 80, "● 👩\u200d💻 dev"},
		{"control chars stripped", ls("evil\x1b[2Jname\n\tx"), 1, 80, "● evil [2Jname x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Strip(nil, tt.labels, tt.total, tt.width)
			if got != tt.want {
				t.Errorf("Strip() = %q, want %q", got, tt.want)
			}
			if w := lipgloss.Width(got); w > max(tt.width, 0) {
				t.Errorf("width %d exceeds budget %d", w, tt.width)
			}
		})
	}
}

func TestStripThemedMatchesPlainAndFitsWidth(t *testing.T) {
	t.Parallel()
	th := theme.Default()
	labels := []domain.Label{
		{Name: "bug", Color: "d73a4a"},
		{Name: "bad colour", Color: "zzz"},
		{Name: "no colour"},
		{Name: "🚀 release", Color: "#0e8a16"},
	}
	for w := 0; w <= 60; w++ {
		got := Strip(th, labels, 4, w)
		if lw := lipgloss.Width(got); lw > w {
			t.Fatalf("width %d: rendered %d columns: %q", w, lw, got)
		}
		if plain := Strip(nil, labels, 4, w); ansi.Strip(got) != plain {
			t.Fatalf("width %d: themed %q != plain %q", w, ansi.Strip(got), plain)
		}
	}
	if got := ansi.Strip(Strip(th, labels, 4, 80)); !strings.Contains(got, "● 🚀 release") {
		t.Errorf("wide-rune label missing: %q", got)
	}
}

func TestHexColor(t *testing.T) {
	t.Parallel()
	for in, ok := range map[string]bool{
		"d73a4a": true, "#D73A4A": true, "": false, "fff": false, "gggggg": false, "d73a4a0": false,
	} {
		if _, got := hexColor(in); got != ok {
			t.Errorf("hexColor(%q) ok = %v, want %v", in, got, ok)
		}
	}
}

func TestDotColorMutesColoursThatVanishIntoBase(t *testing.T) {
	t.Parallel()
	light := *theme.Default()
	light.Base = "#FFFFFF"
	ansiBase := *theme.Default()
	ansiBase.Base = "0"
	tests := []struct {
		name  string
		th    *theme.Theme
		color string
		want  bool
	}{
		{"near-white on white", &light, "fefefe", false},
		{"github default grey on white", &light, "ededed", false},
		{"red on white", &light, "d73a4a", true},
		{"black on dark default", theme.Default(), "000000", false},
		{"red on dark default", theme.Default(), "d73a4a", true},
		{"non-hex base skips the check", &ansiBase, "000000", true},
		{"invalid colour", theme.Default(), "zzz", false},
	}
	for _, tt := range tests {
		if _, got := dotColor(tt.th, tt.color); got != tt.want {
			t.Errorf("%s: dotColor ok = %v, want %v", tt.name, got, tt.want)
		}
	}
}
