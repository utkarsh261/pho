package search

import (
	"testing"

	"github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/diff/parse"
)

func TestSearchEmptyQuery(t *testing.T) {
	t.Parallel()
	idx := &DiffSearchIndex{}
	matches := idx.Search("")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty query, got %d", len(matches))
	}
}

func TestSearchNoMatches(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+new
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("nonexistent")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestSearchSingleMatch(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+hello world
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("hello")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].StartCol != 0 || matches[0].EndCol != 5 {
		t.Errorf("expected col 0-5, got %d-%d", matches[0].StartCol, matches[0].EndCol)
	}
}

func TestSearchMultipleMatches(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 foo
-bar
+foo baz
 foo
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("foo")
	// "foo" appears in: line 0 (context), line 2 (addition "foo baz"), line 3 (context).
	// That's 3 matches.
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
+HELLO World
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("hello")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for case-insensitive search, got %d", len(matches))
	}
}

func TestSearchMultiFile(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+target
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1 @@
-old2
+target2
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("target")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	// First match should be in file 0.
	if matches[0].FileIndex != 0 {
		t.Errorf("expected first match in file 0, got %d", matches[0].FileIndex)
	}
	// Second match should be in file 1.
	if matches[1].FileIndex != 1 {
		t.Errorf("expected second match in file 1, got %d", matches[1].FileIndex)
	}
}

func TestSearchOnlyLineContent(t *testing.T) {
	t.Parallel()
	// The search index covers line content only, not file paths.
	// A query matching a file path but not any line content should return no matches.
	raw := `diff --git a/target.go b/target.go
--- a/target.go
+++ b/target.go
@@ -1 +1 @@
-old
+new
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	// "target.go" appears in the file path but NOT in any line content.
	matches := idx.Search("target.go")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (path-only search not supported), got %d", len(matches))
	}
}

func TestNextMatchWraps(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 foo
-bar
+foo baz
 foo
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("foo")
	if len(matches) < 2 {
		t.Fatalf("need at least 2 matches, got %d", len(matches))
	}

	// NextMatch at last index should wrap to 0.
	next, ok := NextMatch(matches, len(matches)-1)
	if !ok {
		t.Fatal("expected found=true")
	}
	if next != 0 {
		t.Errorf("expected wrap to 0, got %d", next)
	}

	// NextMatch at index 0 should go to 1.
	next, ok = NextMatch(matches, 0)
	if !ok {
		t.Fatal("expected found=true")
	}
	if next != 1 {
		t.Errorf("expected next=1, got %d", next)
	}
}

func TestPrevMatchWraps(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 foo
-bar
+foo baz
 foo
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)
	matches := idx.Search("foo")
	if len(matches) < 2 {
		t.Fatalf("need at least 2 matches, got %d", len(matches))
	}

	// PrevMatch at index 0 should wrap to last.
	prev, ok := PrevMatch(matches, 0)
	if !ok {
		t.Fatal("expected found=true")
	}
	if prev != len(matches)-1 {
		t.Errorf("expected wrap to %d, got %d", len(matches)-1, prev)
	}

	// PrevMatch at index 1 should go to 0.
	prev, ok = PrevMatch(matches, 1)
	if !ok {
		t.Fatal("expected found=true")
	}
	if prev != 0 {
		t.Errorf("expected prev=0, got %d", prev)
	}
}

func TestNextMatchNoMatches(t *testing.T) {
	t.Parallel()
	var matches []SearchMatch
	next, ok := NextMatch(matches, 0)
	if ok {
		t.Error("expected found=false for empty matches")
	}
	if next != 0 {
		t.Errorf("expected cursor=0, got %d", next)
	}
}

func TestPrevMatchNoMatches(t *testing.T) {
	t.Parallel()
	var matches []SearchMatch
	prev, ok := PrevMatch(matches, 0)
	if ok {
		t.Error("expected found=false for empty matches")
	}
	if prev != 0 {
		t.Errorf("expected cursor=0, got %d", prev)
	}
}

func TestBuildNilModel(t *testing.T) {
	t.Parallel()
	idx := Build(nil)
	if idx == nil {
		t.Fatal("expected non-nil index for nil model")
	}
	if idx.totalLines != 0 {
		t.Errorf("expected 0 lines for nil model, got %d", idx.totalLines)
	}
}

// TestBuildAndSearchIntegration verifies the full pipeline.
func TestBuildAndSearchIntegration(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,7 +1,7 @@
 package main

 func main() {
-	fmt.Println("hello")
+	fmt.Println("world")
 	return
 }

 func helper() {
-	return nil
+	return err
 }
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	idx := Build(dm)

	// Search for "return" — should match in both hunks.
	matches := idx.Search("return")
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches for 'return', got %d", len(matches))
	}

	// All matches should have correct file index.
	for _, m := range matches {
		if m.FileIndex != 0 {
			t.Errorf("expected all matches in file 0, got %d", m.FileIndex)
		}
	}
}

// Ensure model import is used.
var _ = model.DiffModel{}
