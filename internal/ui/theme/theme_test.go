package theme

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDefaultConstructsWithoutPanic(t *testing.T) {
	t.Parallel()

	th := Default()
	if th == nil {
		t.Fatal("Default() returned nil")
	}
}

func TestAllColoursAreNonZero(t *testing.T) {
	t.Parallel()

	th := Default()

	colours := map[string]lipgloss.Color{
		"Primary":   th.Primary,
		"Secondary": th.Secondary,
		"Success":   th.Success,
		"Warning":   th.Warning,
		"Error":     th.Error,
		"Muted":     th.Muted,
		"Border":    th.Border,
		"Subtle":    th.Subtle,
	}

	for name, c := range colours {
		if c == "" {
			t.Errorf("colour %s is zero", name)
		}
	}
}

func TestAllStylesAreInitialised(t *testing.T) {
	t.Parallel()

	th := Default()

	v := reflect.ValueOf(th).Elem()
	tp := v.Type()

	styleCount := 0
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		// Only check lipgloss.Style fields.
		if f.Type != reflect.TypeOf(lipgloss.Style{}) {
			continue
		}
		styleCount++
		s := v.Field(i).Interface().(lipgloss.Style)
		// A properly initialised style will render something (even for empty input).
		// We check that Render doesn't panic by calling it.
		_ = s.Render("test")
	}

	if styleCount == 0 {
		t.Fatal("no lipgloss.Style fields found in Theme struct")
	}
}

func TestWithRebuildsStyles(t *testing.T) {
	t.Parallel()

	th := Default()
	th2 := th.With(func(p *Palette) { p.Primary = "#FF0000" })

	if th2.Primary != "#FF0000" {
		t.Errorf("Primary = %v, want #FF0000", th2.Primary)
	}
	if got := th2.PanelFocused.GetBorderLeftForeground(); got != lipgloss.Color("#FF0000") {
		t.Errorf("PanelFocused border = %v, want the new Primary", got)
	}
	if th.Primary == "#FF0000" || th.PanelFocused.GetBorderLeftForeground() == lipgloss.Color("#FF0000") {
		t.Error("With changed the original theme")
	}
}

func TestCopiesDoNotShareAvatars(t *testing.T) {
	t.Parallel()

	th := Default()
	th2 := *th
	th2.AvatarPalette[0] = "#999999"
	if th.AvatarPalette[0] == "#999999" {
		t.Error("a copied theme shares its avatar colours with the original")
	}
	if Default().AvatarPalette[0] == "#999999" {
		t.Error("editing a theme's avatar colours changed the default palette")
	}
}

func TestBuildFillsMissingColours(t *testing.T) {
	t.Parallel()

	th := Build(Palette{Primary: "#FF0000", AvatarPalette: [AvatarCount]lipgloss.Color{"#111111"}})
	def := Default()

	if th.Primary != "#FF0000" || th.AvatarPalette[0] != "#111111" {
		t.Errorf("Build overwrote colours that were set: %v, %v", th.Primary, th.AvatarPalette[0])
	}
	if th.Highlight != def.Highlight || th.DiffAddBg != def.DiffAddBg || th.AvatarPalette[1] != def.AvatarPalette[1] {
		t.Error("Build left missing colours empty instead of using the default palette")
	}
	v := reflect.ValueOf(th.Palette)
	for i := range v.NumField() {
		if c, ok := v.Field(i).Interface().(lipgloss.Color); ok && c == "" {
			t.Errorf("%s is empty", v.Type().Field(i).Name)
		}
	}
}

func TestDefaultMatchesBuild(t *testing.T) {
	t.Parallel()

	if !reflect.DeepEqual(Default(), Build(terminalWorkshop)) {
		t.Error("Default() differs from Build(terminalWorkshop)")
	}
}
