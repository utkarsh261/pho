package overlay

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/utkarsh261/pho/internal/ui/theme"
)

var updateThemePick = flag.Bool("update-theme-pick", false, "overwrite theme picker golden files")

func newThemePick(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil)
	m.SetTheme(theme.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.OpenThemePick(theme.Names(), theme.DefaultName)
	return m
}

func typeRunes(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestThemePickStartsOnCurrent(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	m.OpenThemePick(theme.Names(), "nord")
	if got := m.PreviewTheme(); got != "nord" {
		t.Errorf("PreviewTheme() = %q, want nord", got)
	}
	if !m.IsThemePick() {
		t.Error("IsThemePick() = false")
	}
}

func TestThemePickArrowsMove(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	m.OpenThemePick(theme.Names(), theme.DefaultName)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got, want := m.PreviewTheme(), theme.Names()[1]; got != want {
		t.Errorf("after ↓ PreviewTheme() = %q, want %q", got, want)
	}
}

// TestThemePickJKType checks j and k filter like any letter, as in the repo
// picker, so "tokyo" can be typed.
func TestThemePickJKType(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	m.OpenThemePick(theme.Names(), theme.DefaultName)
	m = typeRunes(m, "jk")
	if m.query != "jk" {
		t.Errorf("query = %q, want jk", m.query)
	}

	m = NewModel(nil)
	m.OpenThemePick(theme.Names(), theme.DefaultName)
	m = typeRunes(m, "tokyo")
	if got := m.PreviewTheme(); got != "tokyo-night" {
		t.Errorf("after typing tokyo PreviewTheme() = %q, want tokyo-night", got)
	}
}

func TestThemePickNoMatches(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	m.OpenThemePick(theme.Names(), theme.DefaultName)
	m = typeRunes(m, "zzz")
	if got := m.PreviewTheme(); got != "" {
		t.Errorf("PreviewTheme() = %q, want empty", got)
	}
}

func TestThemePickGolden(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	cases := map[string]func(Model) Model{
		"theme_pick":          func(m Model) Model { return m },
		"theme_pick_filtered": func(m Model) Model { return typeRunes(m, "cat") },
		"theme_pick_empty":    func(m Model) Model { return typeRunes(m, "zzz") },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			got := setup(newThemePick(t)).View()
			path := filepath.Join("testdata", "golden", name+".ansi")
			if *updateThemePick {
				os.MkdirAll(filepath.Dir(path), 0o755)
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update-theme-pick): %v", err)
			}
			if got != string(want) {
				t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, ansi.Strip(got), ansi.Strip(string(want)))
			}
		})
	}
}

// TestThemePickKeepsBackdropLit checks the theme picker leaves the screen
// behind it in colour, since that is the preview, while the other palette
// modes still dim it.
func TestThemePickKeepsBackdropLit(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("RED")
	bg := strings.Repeat(accent+strings.Repeat(" ", 97)+"\n", 30)
	bg = strings.TrimSuffix(bg, "\n")
	const red = "38;2;255;0;0"

	themed := newThemePick(t)
	if !strings.Contains(themed.ViewOver(bg), red) {
		t.Error("theme picker dimmed the backdrop")
	}

	search := NewModel(nil)
	search.SetTheme(theme.Default())
	search, _ = search.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(search.ViewOver(bg), red) {
		t.Error("search palette no longer dims the backdrop")
	}
}

// TestThemePickIgnoresRefresh checks a background search refresh (more PRs
// loaded) leaves the theme chooser's selection where the user put it.
func TestThemePickIgnoresRefresh(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	m.OpenThemePick(theme.Names(), theme.DefaultName)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	want := m.PreviewTheme()
	m.RefreshResults()
	if got := m.PreviewTheme(); got != want {
		t.Errorf("after RefreshResults PreviewTheme() = %q, want %q", got, want)
	}
}
