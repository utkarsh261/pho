package theme

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// contrastPair is a foreground drawn on a background somewhere in the UI.
type contrastPair struct {
	where  string
	fg, bg lipgloss.Color
	min    float64
}

// contrastPairs lists every fg/bg combination the presets promise to keep
// readable. A new combination in rendering code belongs here too.
func contrastPairs(th *Theme) []contrastPair {
	var pairs []contrastPair
	add := func(where string, min float64, bg lipgloss.Color, fgs ...lipgloss.Color) {
		for _, fg := range fgs {
			pairs = append(pairs, contrastPair{where, fg, bg, min})
		}
	}
	add("body text", 4.5, th.Base, th.Text, th.TextBright, th.BoxText)
	add("secondary text", 3.0, th.Base, th.TextDim, th.Muted, th.HelpText, th.AccentText)
	add("cursor row", 4.5, th.Highlight, th.Text, th.TextBright)
	add("cursor row secondary", 3.0, th.Highlight, th.TextDim, th.Muted)
	add("range selection", 4.5, th.Selection, th.Text)
	add("added line", 4.5, th.DiffAddBg, th.DiffAddFg)
	add("removed line", 4.5, th.DiffDelBg, th.DiffDelFg)
	add("changed words added", 3.0, th.DiffAddEmphBg, th.DiffAddFg)
	add("changed words removed", 3.0, th.DiffDelEmphBg, th.DiffDelFg)
	add("hunk header", 3.0, th.Base, th.DiffHunkFg)
	add("search current match", 4.5, th.SearchCurrentBg, th.SearchMatchFg)
	add("search other match", 4.5, th.SearchOtherBg, th.SearchMatchFg)
	add("search line", 4.5, th.SearchLineBg, th.Text)
	add("section tab", 4.5, th.Primary, th.OnPrimary)
	add("create-PR chosen option", 4.5, th.Primary, th.OnPrimary)
	add("create-PR other option", 3.0, th.Highlight, th.TextDim)
	add("create-PR option, field unfocused", 4.5, th.Selection, th.Text)
	add("status and icons", 3.0, th.Base,
		th.Success, th.Warning, th.Error, th.Secondary, th.SearchActiveFg,
		th.StateOpen, th.StateMerged, th.StateClosed, th.StateDraft)

	md := th.Markdown
	mdColor := func(s *string) lipgloss.Color {
		if s == nil {
			return ""
		}
		return lipgloss.Color(*s)
	}
	add("markdown body", 4.5, th.Base, mdColor(md.Document.Color))
	add("markdown headings", 4.5, th.Base, mdColor(md.Heading.Color))
	add("markdown title", 4.5, mdColor(md.H1.BackgroundColor), mdColor(md.H1.Color))
	add("markdown links", 3.0, th.Base, mdColor(md.Link.Color), mdColor(md.LinkText.Color))
	codeBg := mdColor(md.Code.BackgroundColor)
	if codeBg == "" {
		codeBg = th.Base
	}
	add("markdown code", 4.5, codeBg, mdColor(md.Code.Color))
	return pairs
}

// distinctPairs lists backgrounds that must not merge, or the cursor, tints
// and highlights disappear.
func distinctPairs(th *Theme) [][2]lipgloss.Color {
	return [][2]lipgloss.Color{
		{th.Highlight, th.Base},
		{th.Selection, th.Highlight},
		{th.DiffAddBg, th.DiffDelBg},
		{th.DiffAddBg, th.Base},
		{th.DiffDelBg, th.Base},
		{th.DiffAddEmphBg, th.DiffAddBg},
		{th.DiffDelEmphBg, th.DiffDelBg},
		{th.SearchLineBg, th.Base},
		{th.StateOpen, th.StateMerged},
		{th.StateOpen, th.StateClosed},
		{th.StateOpen, th.StateDraft},
		{th.StateMerged, th.StateClosed},
		{th.StateMerged, th.StateDraft},
		{th.StateClosed, th.StateDraft},
	}
}

var testedProfiles = []struct {
	name    string
	profile termenv.Profile
}{
	{"TrueColor", termenv.TrueColor},
	{"ANSI256", termenv.ANSI256},
}

// themeFailures returns every contrast and distinctness failure of the named
// theme, built under each profile colour guarantees cover. It sets the
// global colour profile, so callers must not run in parallel.
func themeFailures(t *testing.T, name string) []string {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	var out []string
	for _, p := range testedProfiles {
		lipgloss.SetColorProfile(p.profile)
		th, ok := Get(name)
		if !ok {
			t.Fatalf("theme %q not registered", name)
		}
		for _, pair := range contrastPairs(th) {
			if pair.fg == "" || pair.bg == "" {
				out = append(out, fmt.Sprintf("%s %s: missing colour", p.name, pair.where))
				continue
			}
			fg, bg := asProfile(pair.fg, p.profile), asProfile(pair.bg, p.profile)
			if r := contrast(fg, bg); r < pair.min {
				out = append(out, fmt.Sprintf("%s %s: %s on %s is %.2f:1, want %.1f:1",
					p.name, pair.where, fg, bg, r, pair.min))
			}
		}
		for _, d := range distinctPairs(th) {
			a, b := asProfile(d[0], p.profile), asProfile(d[1], p.profile)
			if a == b {
				out = append(out, fmt.Sprintf("%s %s and %s both render as %s", p.name, d[0], d[1], a))
			}
		}
	}
	return out
}

func TestPresetContrastAndDistinctness(t *testing.T) {
	for _, name := range Names() {
		if name == DefaultName {
			continue // report-only, see TestDefaultThemeReport
		}
		for _, f := range themeFailures(t, name) {
			t.Errorf("%s: %s", name, f)
		}
	}
}

// TestDefaultThemeReport records the default theme's failures without
// failing: its look is frozen, so they are reported rather than fixed.
func TestDefaultThemeReport(t *testing.T) {
	got := strings.Join(themeFailures(t, DefaultName), "\n") + "\n"
	path := filepath.Join("testdata", "known_default_failures.txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want, err := os.ReadFile(path); err != nil || string(want) != got {
		t.Logf("default theme report changed; run with -update to refresh %s:\n%s", path, got)
	}
}

// TestPresetsComplete checks each registered palette sets every colour
// itself. It reads the palettes before Build, which would fill any gap from
// the default palette and hide a missing colour.
func TestPresetsComplete(t *testing.T) {
	t.Parallel()

	for name, p := range palettes {
		v := reflect.ValueOf(p)
		for i := range v.NumField() {
			f := v.Type().Field(i)
			switch val := v.Field(i).Interface().(type) {
			case lipgloss.Color:
				if val == "" {
					t.Errorf("%s: %s is not set", name, f.Name)
				}
			case [AvatarCount]lipgloss.Color:
				for j, c := range val {
					if c == "" {
						t.Errorf("%s: %s[%d] is not set", name, f.Name, j)
					}
				}
			}
		}
		if reflect.ValueOf(p.Markdown).IsZero() {
			t.Errorf("%s: Markdown is not set", name)
		}
	}
}

func TestBlend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b  string
		share float64
		want  lipgloss.Color
	}{
		{"#FFFFFF", "#000000", 0, "#000000"},
		{"#FFFFFF", "#000000", 1, "#FFFFFF"},
		{"#FFFFFF", "#000000", 0.5, "#808080"},
		{"#A6E3A1", "#1E1E2E", 0.25, "#404F4B"},
	}
	for _, tc := range tests {
		if got := blend(tc.a, tc.b, tc.share); got != tc.want {
			t.Errorf("blend(%s, %s, %.2f) = %s, want %s", tc.a, tc.b, tc.share, got, tc.want)
		}
	}
}

// asProfile returns the colour a terminal with profile p actually shows for c.
func asProfile(c lipgloss.Color, p termenv.Profile) string {
	return strings.ToUpper(termenv.ConvertToRGB(p.Color(string(c))).Hex())
}

// termenvCutoffs are the channel values whose 256-colour conversion
// depends on the CPU. termenv computes the cube index as (v*255-35)/40 and
// tests v*255 < 48, with v = channel/255; on arm64 the compiler fuses the
// multiply and subtract (FMA), on amd64 it rounds in between, and for these
// two channels that tips the result. A preset colour with one renders as a
// different xterm colour on CI's Linux runners than on Apple silicon.
var termenvCutoffs = map[uint8]bool{48: true, 155: true}

// TestPresetColoursAvoidTermenvCutoffs builds each preset for a 256-colour
// terminal, so tints that switch to fixed xterm indices there are skipped
// and only colours termenv actually converts are checked. It sets the
// global colour profile, so it must not run in parallel.
func TestPresetColoursAvoidTermenvCutoffs(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	for _, name := range Names() {
		if name == DefaultName {
			continue
		}
		th, _ := Get(name)
		for where, hex := range hexColours(reflect.ValueOf(th.Palette), "") {
			r, g, b := hexRGB(hex)
			for _, c := range []uint8{r, g, b} {
				if termenvCutoffs[c] {
					t.Errorf("%s %s = %s has a channel of %d, which 256-colour terminals round inconsistently", name, where, hex, c)
				}
			}
		}
	}
}

var hexColourRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// hexColours collects every "#RRGGBB" colour reachable from v, keyed by
// field path, following pointers, arrays and nested structs.
func hexColours(v reflect.Value, path string) map[string]string {
	out := map[string]string{}
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			maps.Copy(out, hexColours(v.Elem(), path))
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if f := v.Type().Field(i); f.IsExported() {
				maps.Copy(out, hexColours(v.Field(i), path+"."+f.Name))
			}
		}
	case reflect.Array, reflect.Slice:
		for i := range v.Len() {
			maps.Copy(out, hexColours(v.Index(i), fmt.Sprintf("%s[%d]", path, i)))
		}
	case reflect.String:
		if s := v.String(); hexColourRe.MatchString(s) {
			out[path] = s
		}
	}
	return out
}

// TestPresetCodeContrast checks fenced-code tokens against their code
// background in each preset's chroma style (comments get the lower bar,
// since every published palette dims them on purpose).
func TestPresetCodeContrast(t *testing.T) {
	t.Parallel()

	for _, name := range Names() {
		if name == DefaultName {
			continue
		}
		th, _ := Get(name)
		styleName := th.Markdown.CodeBlock.Theme
		style, ok := chromastyles.Registry[styleName]
		if !ok || th.Markdown.CodeBlock.Chroma != nil {
			t.Errorf("%s: fenced code should use a chroma style by name, got %q", name, styleName)
			continue
		}
		for _, tk := range codeContrast {
			e := style.Get(tk.token)
			fg, bg := e.Colour.String(), e.Background.String()
			if !e.Colour.IsSet() {
				continue // inherits the terminal's text colour on the painted base
			}
			if !e.Background.IsSet() {
				bg = string(th.Base)
			}
			if r := contrast(strings.ToUpper(fg), strings.ToUpper(bg)); r < tk.min {
				t.Errorf("%s code %s: %s on %s is %.2f:1, want %.1f:1", name, tk.token, fg, bg, r, tk.min)
			}
		}
	}
}
