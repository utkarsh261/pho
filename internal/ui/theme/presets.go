package theme

import (
	"fmt"
	"math"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)

// spec is a dark theme's published colours, mapped onto pho's roles by
// fromSpec. Hex values come from each theme's own palette.
type spec struct {
	name string

	base     string // editor background
	surface  string // slightly raised background (headers, keycaps)
	border   string // panel borders
	faint    string // gutters and separators
	muted    string // secondary text that must stay readable
	subtext  string // descriptions and metadata
	text     string // body text
	bright   string // titles and the selected row
	accent   string // focus, active tab, section tab background
	accent2  string // PR numbers, keycaps
	accentTx string // accent used as text
	green    string
	yellow   string
	red      string
	orange   string
	purple   string
	blue     string
	cyan     string
	pink     string

	// diffLift lightens diff and inline-code text toward bright so it stays
	// readable on the tinted backgrounds behind it.
	diffLift float64
	// base256 is the xterm index closest to base, for 256-colour terminals.
	base256 int
	// chroma names the chroma style for fenced code, matching the theme.
	chroma string

	// Tint strengths, as the share of the colour blended over base.
	highlight float64 // cursor row
	selection float64 // range selection
	diffLine  float64 // added/removed line
	diffEmph  float64 // changed words within a line
	searchRow float64 // line holding the current search match
}

func fromSpec(s spec) Palette {
	c := func(hex string) lipgloss.Color { return lipgloss.Color(hex) }
	tint := func(hex string, share float64) lipgloss.Color { return blend(hex, s.base, share) }
	return Palette{
		Name: s.name,

		Primary:   c(s.accent),
		Secondary: c(s.accent2),
		Success:   c(s.green),
		Warning:   c(s.yellow),
		Error:     c(s.red),
		Muted:     c(s.muted),
		Border:    c(s.border),
		Subtle:    c(s.surface),

		Text:       c(s.text),
		TextBright: c(s.bright),
		AccentText: c(s.accentTx),
		TextDim:    c(s.subtext),
		Faint:      c(s.faint),
		Highlight:  tint(s.accent, s.highlight),
		Selection:  tint(s.accent, s.selection),

		StateOpen:   c(s.green),
		StateMerged: c(s.purple),
		StateClosed: c(s.red),
		StateDraft:  c(s.muted),

		UnresolvedBorder: tint(s.yellow, 0.55),
		AvatarPalette: [AvatarCount]lipgloss.Color{
			c(s.pink), c(s.blue), c(s.green), c(s.yellow),
			c(s.purple), c(s.red), c(s.cyan), c(s.orange),
		},

		DiffAddBg:     tint(s.green, s.diffLine),
		DiffDelBg:     tint(s.red, s.diffLine),
		DiffAddEmphBg: tint(s.green, s.diffEmph),
		DiffDelEmphBg: tint(s.red, s.diffEmph),
		DiffAddFg:     blend(s.bright, s.green, s.diffLift),
		DiffDelFg:     blend(s.bright, s.red, s.diffLift),
		DiffHunkFg:    c(s.blue),

		SearchCurrentBg: c(s.yellow),
		SearchOtherBg:   c(s.cyan),
		SearchMatchFg:   c(s.base),
		SearchLineBg:    tint(s.blue, s.searchRow),
		SearchActiveFg:  c(s.yellow),

		OnPrimary: c(s.base),
		HelpText:  c(s.subtext),
		BoxText:   c(s.text),
		HintKeyFg: c(s.bright),

		Markdown: markdownStyle(s),

		Base:            c(s.base),
		PaintBackground: true,
		ANSIFallback:    true,
		Colors256:       colors256(s.base256),
	}
}

// colors256 picks xterm colours for the tints: greys stepped up from the
// base, dark green and red for diff lines, and light green and red text.
func colors256(base int) *Colors256 {
	grey := func(step int) lipgloss.Color { return lipgloss.Color(fmt.Sprint(base + step)) }
	return &Colors256{
		Base:          grey(0),
		Surface:       grey(2),
		Highlight:     grey(3),
		Selection:     grey(5),
		DiffAddBg:     "22",  // #005F00
		DiffDelBg:     "52",  // #5F0000
		DiffAddEmphBg: "28",  // #008700
		DiffDelEmphBg: "88",  // #870000
		DiffAddFg:     "157", // #AFFFAF
		DiffDelFg:     "217", // #FFAFAF
		SearchLineBg:  "24",  // #005F87
	}
}

// markdownStyle is glamour's dark style recoloured with the theme's text,
// heading, link and code colours.
func markdownStyle(s spec) ansi.StyleConfig {
	m := styles.DarkStyleConfig
	str := func(v string) *string { return &v }

	m.Document.Color = str(s.text)
	m.Heading.Color = str(s.accentTx)
	m.H1.Color = str(s.base)
	m.H1.BackgroundColor = str(s.accent)
	m.H6.Color = str(s.subtext)
	m.BlockQuote.Color = str(s.subtext)
	m.Link.Color = str(s.blue)
	m.LinkText.Color = str(s.cyan)
	m.Code.Color = str(string(blend(s.bright, s.orange, s.diffLift)))
	m.Code.BackgroundColor = str(s.surface)
	m.CodeBlock.Color = str(s.text)
	// Highlight fenced code with the theme's own chroma style. A custom
	// Chroma config would be registered by glamour under one global name,
	// once, so every later theme would reuse the first one's colours.
	m.CodeBlock.Chroma = nil
	m.CodeBlock.Theme = codeStyle(s)
	m.HorizontalRule.Color = str(s.faint)
	m.Item.Color = nil
	return m
}

// codeContrast is the contrast each fenced-code token must reach against
// the code background; comments, dimmed on purpose by every theme, get the
// lower bar.
var codeContrast = []struct {
	token chroma.TokenType
	min   float64
}{
	{chroma.Text, 4.5},
	{chroma.Keyword, 4.5},
	{chroma.LiteralString, 4.5},
	{chroma.LiteralNumber, 4.5},
	{chroma.NameFunction, 4.5},
	{chroma.Operator, 3.0},
	{chroma.Comment, 3.0},
}

// codeStyle registers "pho-<theme>": the theme's published chroma style
// with any token below codeContrast lifted toward the bright text colour,
// and returns its name.
func codeStyle(s spec) string {
	base := chromastyles.Get(s.chroma)
	b := chroma.NewStyleBuilder("pho-" + s.name)
	for _, tt := range base.Types() {
		b.AddEntry(tt, base.Get(tt))
	}
	bgOf := func(e chroma.StyleEntry) string {
		if e.Background.IsSet() {
			return strings.ToUpper(e.Background.String())
		}
		return s.base
	}
	for _, c := range codeContrast {
		e := base.Get(c.token)
		if !e.Colour.IsSet() {
			continue
		}
		fg, bg := strings.ToUpper(e.Colour.String()), bgOf(e)
		for share := 0.05; contrast(fg, bg) < c.min && share <= 1; share += 0.05 {
			fg = string(blend(s.bright, strings.ToUpper(e.Colour.String()), share))
		}
		e.Colour = chroma.ParseColour(fg)
		b.AddEntry(c.token, e)
	}
	style, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("theme: build code style for %s: %v", s.name, err))
	}
	chromastyles.Register(style)
	return style.Name
}

// contrast is the WCAG 2 contrast ratio of two "#RRGGBB" colours.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	r, g, b := hexRGB(hex)
	lin := func(v uint8) float64 {
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

var presets = []spec{
	{
		// https://catppuccin.com/palette (Mocha)
		name: "catppuccin-mocha", chroma: "catppuccin-mocha",
		base: "#1E1E2E", surface: "#313244", border: "#45475A", faint: "#585B70",
		muted: "#9399B2", subtext: "#A6ADC8", text: "#CDD6F4", bright: "#F5F5FA",
		accent: "#CBA6F7", accent2: "#89DCEB", accentTx: "#B4BEFE",
		green: "#A6E3A1", yellow: "#F9E2AF", red: "#F38BA8", orange: "#FAB387",
		purple: "#CBA6F7", blue: "#89B4FA", cyan: "#94E2D5", pink: "#F5C2E7",
		diffLift: 0.25, base256: 234,
		highlight: 0.16, selection: 0.30, diffLine: 0.14, diffEmph: 0.30, searchRow: 0.30,
	},
	{
		// https://github.com/folke/tokyonight.nvim (night)
		name: "tokyo-night", chroma: "tokyonight-night",
		base: "#1A1B26", surface: "#292E42", border: "#3B4261", faint: "#545C7E",
		muted: "#737AA2", subtext: "#A9B1D6", text: "#C0CAF5", bright: "#E4E9FF",
		accent: "#7AA2F7", accent2: "#7DCFFF", accentTx: "#BB9AF7",
		green: "#9ECE6A", yellow: "#E0AF68", red: "#F7768E", orange: "#FF9E64",
		purple: "#BB9AF7", blue: "#7AA2F7", cyan: "#7DCFFF", pink: "#FF007C",
		diffLift: 0.25, base256: 234,
		highlight: 0.16, selection: 0.30, diffLine: 0.14, diffEmph: 0.30, searchRow: 0.30,
	},
	{
		// https://github.com/morhetz/gruvbox (dark, medium contrast)
		name: "gruvbox-dark", chroma: "gruvbox",
		base: "#282828", surface: "#3C3836", border: "#504945", faint: "#665C54",
		muted: "#A89984", subtext: "#BDAE93", text: "#EBDBB2", bright: "#FBF1C7",
		accent: "#FE8019", accent2: "#8EC07C", accentTx: "#FABD2F",
		green: "#B8BB26", yellow: "#FABD2F", red: "#FB4934", orange: "#FE8019",
		// purple is #D3869B nudged by one: blue 155 converts to a
		// different xterm colour on arm64 than on amd64.
		purple: "#D3869A", blue: "#83A598", cyan: "#8EC07C", pink: "#D3869A",
		diffLift: 0.35, base256: 235,
		highlight: 0.16, selection: 0.30, diffLine: 0.14, diffEmph: 0.30, searchRow: 0.30,
	},
	{
		// https://www.nordtheme.com/docs/colors-and-palettes
		name: "nord", chroma: "nord",
		base: "#2E3440", surface: "#3B4252", border: "#434C5E", faint: "#4C566A",
		muted: "#9AA3B5", subtext: "#B6BECC", text: "#D8DEE9", bright: "#ECEFF4",
		accent: "#88C0D0", accent2: "#8FBCBB", accentTx: "#88C0D0",
		green: "#A3BE8C", yellow: "#EBCB8B", red: "#BF616A", orange: "#D08770",
		purple: "#B48EAD", blue: "#81A1C1", cyan: "#8FBCBB", pink: "#B48EAD",
		diffLift: 0.5, base256: 235,
		highlight: 0.18, selection: 0.32, diffLine: 0.16, diffEmph: 0.32, searchRow: 0.32,
	},
	{
		// https://draculatheme.com/contribute (spec)
		name: "dracula", chroma: "dracula",
		base: "#282A36", surface: "#44475A", border: "#44475A", faint: "#6272A4",
		muted: "#8D99C4", subtext: "#BDC3DC", text: "#F8F8F2", bright: "#FFFFFF",
		accent: "#BD93F9", accent2: "#8BE9FD", accentTx: "#FF79C6",
		green: "#50FA7B", yellow: "#F1FA8C", red: "#FF5555", orange: "#FFB86C",
		purple: "#BD93F9", blue: "#BD93F9", cyan: "#8BE9FD", pink: "#FF79C6",
		diffLift: 0.35, base256: 235,
		highlight: 0.16, selection: 0.30, diffLine: 0.13, diffEmph: 0.28, searchRow: 0.30,
	},
	{
		// https://rosepinetheme.com/palette (main)
		name: "rose-pine", chroma: "rose-pine",
		base: "#191724", surface: "#26233A", border: "#403D52", faint: "#524F67",
		muted: "#908CAA", subtext: "#ABA7C5", text: "#E0DEF4", bright: "#F4F2FF",
		accent: "#C4A7E7", accent2: "#9CCFD8", accentTx: "#EBBCBA",
		green: "#9CCFD8", yellow: "#F6C177", red: "#EB6F92", orange: "#F6C177",
		purple: "#C4A7E7", blue: "#31748F", cyan: "#9CCFD8", pink: "#EBBCBA",
		diffLift: 0.25, base256: 233,
		highlight: 0.16, selection: 0.30, diffLine: 0.15, diffEmph: 0.30, searchRow: 0.32,
	},
}

func init() {
	for _, s := range presets {
		Register(fromSpec(s))
	}
}

// blend mixes share of colour a over b, both "#RRGGBB".
func blend(a, b string, share float64) lipgloss.Color {
	ar, ag, ab := hexRGB(a)
	br, bg, bb := hexRGB(b)
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x)*share + float64(y)*(1-share) + 0.5)
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", mix(ar, br), mix(ag, bg), mix(ab, bb)))
}

// hexRGB parses "#RRGGBB"; it panics on anything else, since presets are
// fixed at compile time.
func hexRGB(hex string) (r, g, b uint8) {
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil || len(hex) != 7 {
		panic(fmt.Sprintf("theme: bad hex colour %q", hex))
	}
	return r, g, b
}
