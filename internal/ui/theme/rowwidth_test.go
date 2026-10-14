package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRowWidthMatchesStringWidth(t *testing.T) {
	t.Parallel()

	rows := []string{
		"", "plain ascii", "\x1b[38;2;1;2;3mcoloured\x1b[0m text",
		"│ box ─┬─ ╭╮ ▎ ● ✓ ✗ ⋯ … → ■",
		"日本語のタイトル", "emoji 🚀 title", "check ✅ done", "é combining", "नमस्ते", "مَرْحَبًا", "שָׁלוֹם", "Ελληνικά", "Привет", "c1\u0085control", "soft\u00adhyphen", "ไทย",
		"◽ wide shape", "tab\there", "\x1b]8;;https://x\x07link\x1b]8;;\x07",
	}
	// Every rendered row in the app's colour goldens.
	files, _ := filepath.Glob(filepath.Join("..", "app", "testdata", "golden", "ansi", "*", "*", "*.ansi"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, strings.Split(string(b), "\n")...)
	}
	if len(files) == 0 {
		t.Fatal("no app goldens found")
	}
	for _, row := range rows {
		if got, want := rowWidth(row), ansi.StringWidth(row); got != want {
			t.Errorf("rowWidth(%q) = %d, want %d", row, got, want)
		}
	}
}
