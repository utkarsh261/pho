package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/ui/theme"
)

var pickFiles = []FileItem{
	{Path: "internal/model/view.go", Status: "modified", Additions: 3, Deletions: 1},
	{Path: "internal/view/model.go", Status: "added", Additions: 40},
	{Path: "README.md", Status: "removed", Deletions: 12},
}

func newFilePick(t *testing.T, width int) Model {
	t.Helper()
	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m.OpenFilePick(pickFiles, "internal/view/model.go")
	return m
}

func TestFilePickStartsOnCurrent(t *testing.T) {
	t.Parallel()

	m := newFilePick(t, 100)
	if !m.IsFilePick() || m.IsThemePick() {
		t.Errorf("IsFilePick() = %v, IsThemePick() = %v", m.IsFilePick(), m.IsThemePick())
	}
	if got := m.SelectedFile(); got != "internal/view/model.go" {
		t.Errorf("SelectedFile() = %q, want the current file", got)
	}
	if got := len(m.results); got != len(pickFiles) {
		t.Errorf("listed %d files, want %d", got, len(pickFiles))
	}
}

// TestFilePickRanksFileNames checks a match in the file name ranks above
// one only in the directory, and the empty filter keeps diff order.
func TestFilePickRanksFileNames(t *testing.T) {
	t.Parallel()

	m := typeRunes(newFilePick(t, 100), "model")
	var got []string
	for _, r := range m.results {
		got = append(got, r.Title)
	}
	want := []string{"internal/view/model.go", "internal/model/view.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("results = %v, want %v", got, want)
	}

	m = typeRunes(newFilePick(t, 100), "zzz")
	if got := m.SelectedFile(); got != "" {
		t.Errorf("SelectedFile() with no matches = %q, want empty", got)
	}
}

func TestFilePickRows(t *testing.T) {
	t.Parallel()

	view := ansi.Strip(newFilePick(t, 100).View())
	for _, want := range []string{"Go to file", "Filter files", "M internal/model/view.go", "A internal/view/model.go", "D README.md", "+40 -0"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
}

// TestFilePickLongPathKeepsName checks a path too long for the row is cut
// from the left, so the file name stays readable.
func TestFilePickLongPathKeepsName(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	long := strings.Repeat("deeply/nested/", 6) + "handler.go"
	m.OpenFilePick([]FileItem{{Path: long, Additions: 1}}, long)
	view := m.View()
	if !strings.Contains(view, "…") || !strings.Contains(view, "/handler.go") {
		t.Errorf("long path row lost the file name:\n%s", view)
	}
}
