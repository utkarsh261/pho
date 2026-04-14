package parse

import (
	"testing"
)

func TestParseEmptyDiff(t *testing.T) {
	t.Parallel()
	dm, err := Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dm == nil {
		t.Fatal("expected non-nil DiffModel for empty diff")
	}
	if len(dm.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(dm.Files))
	}
}

func TestParseNormalDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
index abc1234..def5678 100644
--- a/file.go
+++ b/file.go
@@ -1,5 +1,8 @@
 package main
 
 func hello() {
-	return "old"
+	return "new"
+}
+
+func world() {
+	return "world"
 }
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	f := dm.Files[0]
	if f.OldPath != "file.go" {
		t.Errorf("expected OldPath=file.go, got %q", f.OldPath)
	}
	if f.NewPath != "file.go" {
		t.Errorf("expected NewPath=file.go, got %q", f.NewPath)
	}
	if f.Status != "modified" {
		t.Errorf("expected status=modified, got %q", f.Status)
	}
	if f.IsBinary {
		t.Error("expected non-binary")
	}
	if len(dm.FileIndex) == 0 {
		t.Error("expected FileIndex to be populated")
	}
}

func TestParseMultiFileDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1 @@
-old2
+new2
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(dm.Files))
	}
	if dm.Files[0].NewPath != "a.go" {
		t.Errorf("expected first file=a.go, got %q", dm.Files[0].NewPath)
	}
	if dm.Files[1].NewPath != "b.go" {
		t.Errorf("expected second file=b.go, got %q", dm.Files[1].NewPath)
	}
}

func TestParseRenamedDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/old.go b/new.go
similarity index 90%
rename from old.go
rename to new.go
--- a/old.go
+++ b/new.go
@@ -1 +1 @@
 old
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	f := dm.Files[0]
	if f.Status != "renamed" {
		t.Errorf("expected status=renamed, got %q", f.Status)
	}
	if f.OldPath != "old.go" {
		t.Errorf("expected OldPath=old.go, got %q", f.OldPath)
	}
	if f.NewPath != "new.go" {
		t.Errorf("expected NewPath=new.go, got %q", f.NewPath)
	}
}

func TestParseBinaryDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/image.png b/image.png
index abc1234..def5678 100644
Binary files a/image.png and b/image.png differ
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	if !dm.Files[0].IsBinary {
		t.Error("expected IsBinary=true")
	}
}

func TestParseNewFileDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func main() {}
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	f := dm.Files[0]
	if f.Status != "added" {
		t.Errorf("expected status=added, got %q", f.Status)
	}
	if f.NewPath != "new.go" {
		t.Errorf("expected NewPath=new.go, got %q", f.NewPath)
	}
}

func TestParseDeletedFileDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func main() {}
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	f := dm.Files[0]
	if f.Status != "removed" {
		t.Errorf("expected status=removed, got %q", f.Status)
	}
	if f.OldPath != "old.go" {
		t.Errorf("expected OldPath=old.go, got %q", f.OldPath)
	}
}

func TestParseLineNumbers(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -2,3 +2,4 @@
 context
-deleted
+added
 context2
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	hunk := dm.Files[0].Hunks[0]

	// First line: context at old line 2, new line 2
	if hunk.Lines[0].Kind != "context" {
		t.Fatalf("expected first line to be context")
	}
	if hunk.Lines[0].OldLine == nil || *hunk.Lines[0].OldLine != 2 {
		t.Errorf("expected first line old=2, got %v", hunk.Lines[0].OldLine)
	}
	if hunk.Lines[0].NewLine == nil || *hunk.Lines[0].NewLine != 2 {
		t.Errorf("expected first line new=2, got %v", hunk.Lines[0].NewLine)
	}

	// Second line: deletion at old line 3
	if hunk.Lines[1].Kind != "deletion" {
		t.Fatalf("expected second line to be deletion")
	}
	if hunk.Lines[1].OldLine == nil || *hunk.Lines[1].OldLine != 3 {
		t.Errorf("expected deletion old=3, got %v", hunk.Lines[1].OldLine)
	}
	if hunk.Lines[1].NewLine != nil {
		t.Errorf("expected deletion new=nil, got %v", hunk.Lines[1].NewLine)
	}

	// Third line: addition at new line 3
	if hunk.Lines[2].Kind != "addition" {
		t.Fatalf("expected third line to be addition")
	}
	if hunk.Lines[2].OldLine != nil {
		t.Errorf("expected addition old=nil, got %v", hunk.Lines[2].OldLine)
	}
	if hunk.Lines[2].NewLine == nil || *hunk.Lines[2].NewLine != 3 {
		t.Errorf("expected addition new=3, got %v", hunk.Lines[2].NewLine)
	}
}

func TestParseStats(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-old
+new
+extra
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1 @@
-old2
+new2
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dm.Stats.TotalFiles != 2 {
		t.Errorf("expected TotalFiles=2, got %d", dm.Stats.TotalFiles)
	}
	if dm.Stats.TotalAdditions != 3 {
		t.Errorf("expected TotalAdditions=3, got %d", dm.Stats.TotalAdditions)
	}
	if dm.Stats.TotalDeletions != 2 {
		t.Errorf("expected TotalDeletions=2, got %d", dm.Stats.TotalDeletions)
	}
	if dm.Stats.TotalHunks != 2 {
		t.Errorf("expected TotalHunks=2, got %d", dm.Stats.TotalHunks)
	}
}

func TestParseDisplayRows(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 line1
-line2
+line2modified
+extra
 line3`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := dm.Files[0]
	// 1 file header + 1 hunk header + 5 content lines = 7
	if f.DisplayRows != 7 {
		t.Errorf("expected DisplayRows=7, got %d", f.DisplayRows)
	}
}

func TestParseNoNewlineAtEnd(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1 +1 @@
-old
\ No newline at end of file
+new
\ No newline at end of file`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dm.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(dm.Files))
	}
	// "\ No newline..." lines should be skipped.
	// So we have: 1 deletion + 1 addition = 2 lines in the hunk.
	hunk := dm.Files[0].Hunks[0]
	if len(hunk.Lines) != 2 {
		t.Errorf("expected 2 lines (skip no-newline markers), got %d", len(hunk.Lines))
	}
}

func TestParseFileIndex(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/pkg/sub/file.go b/pkg/sub/file.go
--- a/pkg/sub/file.go
+++ b/pkg/sub/file.go
@@ -1 +1 @@
-old
+new
`
	dm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	idx, ok := dm.FileIndex["pkg/sub/file.go"]
	if !ok {
		t.Fatal("expected FileIndex to contain pkg/sub/file.go")
	}
	if idx != 0 {
		t.Errorf("expected index=0, got %d", idx)
	}
}
