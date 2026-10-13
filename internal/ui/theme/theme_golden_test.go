package theme

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Run with -update to regenerate the golden file:
//
//	go test ./internal/ui/theme/... -update
var update = flag.Bool("update", false, "overwrite theme golden files with current output")

// TestDefaultThemeGolden pins every colour and style of the default theme.
// Existing entries must keep their values; new fields only need -update.
func TestDefaultThemeGolden(t *testing.T) {
	got := dumpTheme(Default())
	path := filepath.Join("testdata", "golden", "default_theme.txt")

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(formatDump(got)), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	want := parseDump(t, string(raw))

	for _, name := range sortedKeys(want) {
		g, ok := got[name]
		switch {
		case !ok:
			t.Errorf("%s: in golden but missing from theme", name)
		case g != want[name]:
			t.Errorf("%s changed:\n  golden: %s\n  now:    %s", name, want[name], g)
		}
	}
	for _, name := range sortedKeys(got) {
		if _, ok := want[name]; !ok {
			t.Errorf("%s: new theme entry not in golden; run with -update", name)
		}
	}
}

// dumpTheme flattens th into "Field" → value strings via reflection, so a
// field added to Theme is picked up without touching this test.
func dumpTheme(th *Theme) map[string]string {
	out := map[string]string{}
	v := reflect.ValueOf(th).Elem()
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		switch f := v.Field(i).Interface().(type) {
		case lipgloss.Color:
			out[name] = string(f)
		case []lipgloss.Color:
			parts := make([]string, len(f))
			for j, c := range f {
				parts[j] = string(c)
			}
			out[name] = strings.Join(parts, " ")
		case lipgloss.Style:
			out[name] = describeStyle(f)
		default:
			out[name] = fmt.Sprintf("unhandled %T", f)
		}
	}
	return out
}

func describeStyle(s lipgloss.Style) string {
	pt, pr, pb, pl := s.GetPadding()
	border, bt, br, bb, bl := s.GetBorder()
	return fmt.Sprintf(
		"fg=%v bg=%v bold=%t underline=%t faint=%t reverse=%t pad=%d,%d,%d,%d border=%q,%q,%q,%q sides=%t,%t,%t,%t borderfg=%v,%v,%v,%v",
		s.GetForeground(), s.GetBackground(),
		s.GetBold(), s.GetUnderline(), s.GetFaint(), s.GetReverse(),
		pt, pr, pb, pl,
		border.Top, border.Right, border.Bottom, border.Left,
		bt, br, bb, bl,
		s.GetBorderTopForeground(), s.GetBorderRightForeground(),
		s.GetBorderBottomForeground(), s.GetBorderLeftForeground(),
	)
}

func formatDump(m map[string]string) string {
	var b strings.Builder
	for _, k := range sortedKeys(m) {
		fmt.Fprintf(&b, "%s: %s\n", k, m[k])
	}
	return b.String()
}

func parseDump(t *testing.T, s string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("malformed golden line %q", line)
		}
		m[k] = v
	}
	return m
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
