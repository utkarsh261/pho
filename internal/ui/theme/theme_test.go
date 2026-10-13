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

func TestChangingColourPropagates(t *testing.T) {
	t.Parallel()

	th := Default()

	// Clone and modify.
	th2 := *th
	th2.Primary = lipgloss.Color("#FF0000")

	if th2.Primary == th.Primary {
		t.Error("modifying a copy should not affect the original")
	}
	if th2.Primary != lipgloss.Color("#FF0000") {
		t.Errorf("expected Primary to be #FF0000, got %v", th2.Primary)
	}
}

func TestBuildCopiesAvatarPalette(t *testing.T) {
	t.Parallel()

	p := terminalWorkshop
	p.AvatarPalette = []lipgloss.Color{"#111111", "#222222"}
	th := Build(p)
	p.AvatarPalette[0] = "#999999"

	if th.AvatarPalette[0] != "#111111" {
		t.Errorf("Build shares the caller's AvatarPalette slice: got %s", th.AvatarPalette[0])
	}
}

func TestDefaultMatchesBuild(t *testing.T) {
	t.Parallel()

	if !reflect.DeepEqual(Default(), Build(terminalWorkshop)) {
		t.Error("Default() differs from Build(terminalWorkshop)")
	}
}
