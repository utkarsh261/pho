package prdetail

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/parse"
)

func TestClipForRender(t *testing.T) {
	t.Parallel()
	short := strings.Repeat("a", 100)
	if got := clipForRender(short, 80); got != short {
		t.Fatal("short line must be returned unchanged")
	}
	long := strings.Repeat("é", 10000) // 2 bytes per rune
	got := clipForRender(long, 80)
	if len(got) >= len(long) || len(got) < 80*4 {
		t.Fatalf("clipped length %d out of range", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatal("clip split a rune")
	}
}

func TestDiffRendersMegabyteLinesQuickly(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("abcdefgh", 128*1024) // 1MB
	var b strings.Builder
	b.WriteString("diff --git a/bundle.min.js b/bundle.min.js\n--- a/bundle.min.js\n+++ b/bundle.min.js\n@@ -1,30 +1,30 @@\n")
	for range 15 {
		b.WriteString("-" + long + "\n+" + long + "x\n")
	}
	dm, err := parse.Parse(b.String())
	if err != nil {
		t.Fatal(err)
	}
	m := makePRDetail(120, 40, nil, nil)
	m.Diff = dm
	m.buildNavigableIndex()
	m.renderDiffTab(0, 34, 100) // first frame also computes word diffs
	start := time.Now()
	rows := m.renderDiffTab(0, 34, 100)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("rendering a screen of 1MB lines took %v", d)
	}
	for i, r := range rows {
		if w := lipgloss.Width(r); w > 100 {
			t.Fatalf("row %d is %d cells wide, want <= 100", i, w)
		}
	}
	if !strings.Contains(rows[4], "abcdefgh") {
		t.Fatalf("expected long line content on row 4, got %q", rows[4])
	}
}

func TestLongLineFileShowsAllHunks(t *testing.T) {
	t.Parallel()
	dm, err := parse.Parse(difftest.LongLineDiff().Raw)
	if err != nil {
		t.Fatal(err)
	}
	m := makePRDetail(120, 40, nil, nil)
	m.Diff = dm
	m.buildNavigableIndex()
	out := strings.Join(m.renderDiffTab(0, 40, 100), "\n")
	for _, want := range []string{"@@ -10,2 +10,2 @@", "new", "after.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered diff missing %q", want)
		}
	}
}
