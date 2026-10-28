package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/utkarsh261/pho/internal/fsutil"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fsutil.WriteAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("content = %q, %v; want new", got, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want only the target (temp file left behind?)", len(entries))
	}
}

func TestWriteAtomic_MissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "state.json")
	if err := fsutil.WriteAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("WriteAtomic into a missing directory succeeded, want error")
	}
}
