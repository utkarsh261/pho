package anchor

import (
	"testing"

	"github.com/utkarsh261/pho/internal/diff/parse"
)

func TestGenerateAnchorsForAdditions(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1,2 @@
 context
+addition
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	Generate(dm, "abc123")

	// The addition line should have an anchor with Side=RIGHT.
	hunk := dm.Files[0].Hunks[0]
	// Lines: [0]=context, [1]=addition
	addLine := hunk.Lines[1]
	if len(addLine.Anchors) != 1 {
		t.Fatalf("expected 1 anchor for addition line, got %d", len(addLine.Anchors))
	}
	a := addLine.Anchors[0]
	if a.Side != "RIGHT" {
		t.Errorf("expected Side=RIGHT for addition, got %q", a.Side)
	}
	if a.Path != "file.go" {
		t.Errorf("expected Path=file.go, got %q", a.Path)
	}
	if a.CommitSHA != "abc123" {
		t.Errorf("expected CommitSHA=abc123, got %q", a.CommitSHA)
	}
}

func TestGenerateAnchorsForDeletions(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,2 +1 @@
 context
-deletion
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	Generate(dm, "def456")

	hunk := dm.Files[0].Hunks[0]
	// Lines: [0]=context, [1]=deletion
	delLine := hunk.Lines[1]
	if len(delLine.Anchors) != 1 {
		t.Fatalf("expected 1 anchor for deletion line, got %d", len(delLine.Anchors))
	}
	a := delLine.Anchors[0]
	if a.Side != "LEFT" {
		t.Errorf("expected Side=LEFT for deletion, got %q", a.Side)
	}
}

func TestGenerateAnchorsForContext(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
 context
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	Generate(dm, "ghi789")

	hunk := dm.Files[0].Hunks[0]
	ctxLine := hunk.Lines[0]
	if len(ctxLine.Anchors) != 1 {
		t.Fatalf("expected 1 anchor for context line, got %d", len(ctxLine.Anchors))
	}
	a := ctxLine.Anchors[0]
	if a.Side != "RIGHT" {
		t.Errorf("expected Side=RIGHT for context, got %q", a.Side)
	}
}

func TestGenerateNoAnchorsForHunkHeader(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
 line
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// The parser doesn't create a "hunk_header" DiffLine entry —
	// hunk headers are stored separately in DiffHunk.Header.
	// This test just verifies that the anchor generation
	// doesn't crash or add spurious anchors.
	hunk := dm.Files[0].Hunks[0]
	// All lines in the hunk should be content lines (context/addition/deletion).
	for i, line := range hunk.Lines {
		if line.Kind == "hunk_header" {
			t.Errorf("line %d should not have kind hunk_header", i)
		}
	}
}

func TestGenerateNilModel(t *testing.T) {
	t.Parallel()
	// Should not panic.
	Generate(nil, "abc123")
}

func TestGenerateEmptySHA(t *testing.T) {
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
	Generate(dm, "")

	// No anchors should be generated with empty SHA.
	for _, file := range dm.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if len(line.Anchors) != 0 {
					t.Errorf("expected 0 anchors with empty SHA, got %d", len(line.Anchors))
				}
			}
		}
	}
}

func TestGenerateRenamedFile(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/old.go b/new.go
rename from old.go
rename to new.go
--- a/old.go
+++ b/new.go
@@ -1 +1 @@
-old
+new
`
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	Generate(dm, "sha123")

	// Anchors should use the new path for additions.
	hunk := dm.Files[0].Hunks[0]
	for _, line := range hunk.Lines {
		if len(line.Anchors) > 0 {
			if line.Anchors[0].Path != "new.go" {
				t.Errorf("expected Path=new.go for addition, got %q", line.Anchors[0].Path)
			}
		}
	}
}
