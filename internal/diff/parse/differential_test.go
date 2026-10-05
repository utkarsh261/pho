package parse

import (
	"reflect"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/diff/difftest"
	"github.com/utkarsh261/pho/internal/diff/model"
)

// requireSameAsLegacy fails if Parse and the old scanner-based parser
// disagree on anything other than the fields only the new parser fills.
func requireSameAsLegacy(t *testing.T, raw string) {
	t.Helper()
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	want, err := parseLegacy(raw)
	if err != nil {
		t.Fatal(err)
	}
	for i := range got.Files {
		got.Files[i].MaxLineLen = 0
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse differs from legacy parser\nraw:\n%s\ngot:  %+v\nwant: %+v", raw, got, want)
	}
}

func TestParseMatchesLegacyOnFixtures(t *testing.T) {
	t.Parallel()
	for _, fx := range difftest.RawDiffs() {
		t.Run(fx.Name, func(t *testing.T) { requireSameAsLegacy(t, fx.Raw) })
	}
}

func TestParseMatchesLegacyOnOddInputs(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"preamble\ndiff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n",
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b",
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n\n\n",
		"diff --git a/x b/x\n@@ -1 +1 @@\n--- not a header\n+++ also not\n",
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n+Binary files in content\n ctx\r\n",
		"diff --git a/x b/x\r\n--- a/x\r\n+++ b/x\r\n@@ -1 +1 @@\r\n-a\r\n+b\r\n",
		"diff --git a/only b/only\n",
		"diff --git a/a b/a\ndiff --git a/b b/b\n",
		"\n\ndiff --git a/x b/x\n",
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n?weird\n-\n+\n",
	} {
		requireSameAsLegacy(t, raw)
	}
}

func FuzzParseMatchesLegacy(f *testing.F) {
	for _, fx := range difftest.RawDiffs() {
		if len(fx.Raw) < 4096 {
			f.Add(fx.Raw)
		}
	}
	f.Add("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,3 +1,3 @@\n a\n-b\n+c\n")
	f.Fuzz(func(t *testing.T, raw string) {
		if strings.Contains(raw, strings.Repeat("x", 60000)) || len(raw) > 60000 {
			t.Skip() // the legacy parser stops at 64KB lines
		}
		requireSameAsLegacy(t, raw)
	})
}

func TestParseKeepsHunksAfterLongLine(t *testing.T) {
	t.Parallel()
	dm, err := Parse(difftest.LongLineDiff().Raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(dm.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(dm.Files))
	}
	f := dm.Files[0]
	if len(f.Hunks) != 2 || f.Additions != 2 || f.Deletions != 2 {
		t.Fatalf("a.min.js: hunks=%d +%d -%d, want 2 hunks +2 -2", len(f.Hunks), f.Additions, f.Deletions)
	}
	if f.MaxLineLen != 70001 {
		t.Errorf("MaxLineLen = %d, want 70001", f.MaxLineLen)
	}
	h := f.Hunks[1]
	if got := lineKinds(h.Lines); got != "deletion addition context" {
		t.Errorf("second hunk kinds = %q", got)
	}
	if *h.Lines[0].OldLine != 10 || *h.Lines[1].NewLine != 10 {
		t.Errorf("second hunk numbering: old=%d new=%d", *h.Lines[0].OldLine, *h.Lines[1].NewLine)
	}
	if dm.Files[1].NewPath != "after.go" || dm.Files[1].Additions != 1 {
		t.Errorf("file after the long line parsed wrong: %+v", dm.Files[1])
	}
}

func lineKinds(lines []model.DiffLine) string {
	kinds := make([]string, len(lines))
	for i, l := range lines {
		kinds[i] = l.Kind
	}
	return strings.Join(kinds, " ")
}
