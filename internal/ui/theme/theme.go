// Package theme defines all lipgloss styles for the pho terminal UI.
// A single Theme struct is constructed once at startup and passed to every
// panel model — no globals, no side effects.
package theme

import (
	"reflect"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// AvatarCount is the number of colours used to tell comment authors apart.
const AvatarCount = 8

// Palette holds the colours a theme is built from. Build derives every style
// from it, so a new look only needs a new Palette.
//
// A built Theme's palette is read-only: its styles were derived from these
// colours, so changing one afterwards leaves the styles on the old value.
// Use Theme.With to get a theme with different colours.
type Palette struct {
	// Name identifies the palette, e.g. "terminal-workshop".
	Name string

	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Muted     lipgloss.Color
	Border    lipgloss.Color
	Subtle    lipgloss.Color

	Text       lipgloss.Color // primary foreground for content
	TextBright lipgloss.Color // emphasised foreground (selected rows, titles)
	AccentText lipgloss.Color // lighter accent for text on the dark background
	TextDim    lipgloss.Color // secondary foreground (descriptions, metadata)
	Faint      lipgloss.Color // tertiary foreground (gutters, separators in text)
	Highlight  lipgloss.Color // tinted background for the selected row / cursor
	Selection  lipgloss.Color // stronger tinted background for range selections

	// PR state pill backgrounds.
	StateOpen   lipgloss.Color
	StateMerged lipgloss.Color
	StateClosed lipgloss.Color
	StateDraft  lipgloss.Color

	// Review threads still awaiting resolution.
	UnresolvedBorder lipgloss.Color

	// Colours used to tell comment authors apart.
	// An array rather than a slice, so copying a Palette or Theme never
	// shares it.
	AvatarPalette [AvatarCount]lipgloss.Color

	// Diff line backgrounds.
	DiffAddBg lipgloss.Color
	DiffDelBg lipgloss.Color
	// Stronger backgrounds for the words that changed within a line.
	DiffAddEmphBg lipgloss.Color
	DiffDelEmphBg lipgloss.Color
	// Diff line text and the @@ hunk header.
	DiffAddFg  lipgloss.Color
	DiffDelFg  lipgloss.Color
	DiffHunkFg lipgloss.Color

	// Diff search: the current and other matched words, the text on them,
	// and the background of the line holding the current match.
	SearchCurrentBg lipgloss.Color
	SearchOtherBg   lipgloss.Color
	SearchMatchFg   lipgloss.Color
	SearchLineBg    lipgloss.Color
	// Status bar text while a search is being typed.
	SearchActiveFg lipgloss.Color

	OnPrimary lipgloss.Color // text on a Primary background
	HelpText  lipgloss.Color // non-key status bar text
	BoxText   lipgloss.Color // unselected overlay rows
	HintKeyFg lipgloss.Color // the key in a key hint

	// Markdown is the glamour style for PR descriptions and comments.
	Markdown ansi.StyleConfig

	// Base is the background the palette is designed against. Blends and
	// contrast checks use it; it is painted only when PaintBackground is set.
	Base lipgloss.Color
	// PaintBackground fills the whole UI with Base instead of leaving the
	// terminal's own background showing.
	PaintBackground bool
	// ANSIFallback marks the cursor and selection with reverse video on
	// 16-colour terminals, where background tints can collapse together.
	ANSIFallback bool
	// Colors256 replaces the subtle backgrounds on 256-colour terminals,
	// where downsampled hex tints collapse into the same few xterm colours.
	Colors256 *Colors256
}

// Colors256 holds xterm palette indices ("236") used in place of a
// palette's tints when the terminal has 256 colours.
type Colors256 struct {
	Base, Surface        lipgloss.Color
	Highlight, Selection lipgloss.Color
	DiffAddBg, DiffDelBg lipgloss.Color
	DiffAddEmphBg        lipgloss.Color
	DiffDelEmphBg        lipgloss.Color
	DiffAddFg, DiffDelFg lipgloss.Color
	SearchLineBg         lipgloss.Color
}

// apply256 swaps in c's colours, including the ones derived from Base.
func (p *Palette) apply256(c *Colors256) {
	p.Base, p.OnPrimary, p.SearchMatchFg = c.Base, c.Base, c.Base
	p.Subtle = c.Surface
	p.Highlight, p.Selection = c.Highlight, c.Selection
	p.DiffAddBg, p.DiffDelBg = c.DiffAddBg, c.DiffDelBg
	p.DiffAddEmphBg, p.DiffDelEmphBg = c.DiffAddEmphBg, c.DiffDelEmphBg
	p.DiffAddFg, p.DiffDelFg = c.DiffAddFg, c.DiffDelFg
	p.SearchLineBg = c.SearchLineBg

	md := p.Markdown
	base, surface := string(c.Base), string(c.Surface)
	md.H1.Color = &base
	md.Code.BackgroundColor = &surface
	p.Markdown = md
}

// Theme holds every reusable lipgloss style used across the application,
// along with the Palette it was built from. Call Default() for the standard
// look, or Build with your own Palette.
type Theme struct {
	Palette

	// ── panel borders ──────────────────────────────────────────────
	Panel        lipgloss.Style // normal panel left border (gray)
	PanelFocused lipgloss.Style // focused panel left border (violet)

	// ── section dividers ───────────────────────────────────────────
	Divider lipgloss.Style // horizontal rule in Border color

	// ── row styles ─────────────────────────────────────────────────
	NormalRow lipgloss.Style // no decoration

	// ── text styles ────────────────────────────────────────────────
	Title        lipgloss.Style // bold
	Header       lipgloss.Style // bold heading text for panels and the PR title
	Bold         lipgloss.Style // bold, normal fg
	MutedTxt     lipgloss.Style // muted fg
	FaintTxt     lipgloss.Style // faint fg for separators and gutters
	DimTxt       lipgloss.Style // secondary text (names, metadata)
	PrimaryTxt   lipgloss.Style // primary fg colour
	SecondaryTxt lipgloss.Style // secondary fg + bold
	Number       lipgloss.Style // secondary fg + bold (for #123 PR numbers)

	// ── CI / Review status icons ───────────────────────────────────
	CISuccess lipgloss.Style // emerald
	CIFailure lipgloss.Style // red
	CIPending lipgloss.Style // amber
	CIMuted   lipgloss.Style // muted

	ReviewApproved lipgloss.Style // emerald
	ReviewChanges  lipgloss.Style // red
	ReviewRequired lipgloss.Style // amber
	ReviewDraft    lipgloss.Style // muted
	ReviewMuted    lipgloss.Style // muted

	// ── diff stats ─────────────────────────────────────────────────
	Additions lipgloss.Style // emerald
	Deletions lipgloss.Style // red

	// ── diff line colors ───────────────────────────────────────────
	DiffAddition   lipgloss.Style // light green text on the addition tint
	DiffDeletion   lipgloss.Style // light red text on the deletion tint
	DiffHunkHeader lipgloss.Style // quiet blue-grey for @@ headers

	// ── tab bar ────────────────────────────────────────────────────
	TabActive   lipgloss.Style // accent text, bold + underlined, with padding
	TabInactive lipgloss.Style // muted text + padding

	// PR detail section tabs ("● Desc 2:Diff 3:Comments 4:Commits").
	SectionTabActive lipgloss.Style // violet bg + white text + padding

	// ── status bar ─────────────────────────────────────────────────
	StatusHelp    lipgloss.Style // light gray, readable but not attention-grabbing
	StatusLoading lipgloss.Style // warning
	StatusStale   lipgloss.Style // warning
	StatusError   lipgloss.Style // error + bold
	StatusFresh   lipgloss.Style // success
	StatusSep     lipgloss.Style // border colour

	// ── overlay / command palette ──────────────────────────────────
	BoxBorder lipgloss.Style // centred box with primary border + dark bg
	BoxTitle  lipgloss.Style // bold overlay title
	BoxNormal lipgloss.Style // readable light text for unselected rows
	BoxFooter lipgloss.Style // muted, faint
	BoxDiv    lipgloss.Style // border colour divider

	// PR number and author styles (commits tab)
	BoxPRNum    lipgloss.Style // cyan + bold for #1234
	BoxPRAuthor lipgloss.Style // muted for @author

	// Keymap overlay styles
	Keycap       lipgloss.Style // subtle bg + secondary fg key badge
	KeymapHeader lipgloss.Style // bold primary for category headers

	// Key hints (status bar, compose pane, inline hints).
	HintKey  lipgloss.Style // the key itself ("enter")
	HintDesc lipgloss.Style // what it does ("send")
}

// terminalWorkshop is pho's original palette.
var terminalWorkshop = Palette{
	Name: "terminal-workshop",

	Primary:   lipgloss.Color("#7C3AED"),
	Secondary: lipgloss.Color("#06B6D4"),
	Success:   lipgloss.Color("#10B981"),
	Warning:   lipgloss.Color("#F59E0B"),
	Error:     lipgloss.Color("#EF4444"),
	Muted:     lipgloss.Color("#6B7280"),
	Border:    lipgloss.Color("#30363D"),
	Subtle:    lipgloss.Color("#1F2937"),

	Text:       lipgloss.Color("#E6EDF3"),
	TextBright: lipgloss.Color("#FFFFFF"),
	AccentText: lipgloss.Color("#A78BFA"),
	TextDim:    lipgloss.Color("#9CA3AF"),
	Faint:      lipgloss.Color("#4B5563"),
	Highlight:  lipgloss.Color("#262338"),
	Selection:  lipgloss.Color("#3B2F66"),

	StateOpen:   lipgloss.Color("#238636"),
	StateMerged: lipgloss.Color("#8957E5"),
	StateClosed: lipgloss.Color("#DA3633"),
	StateDraft:  lipgloss.Color("#6E7681"),

	UnresolvedBorder: lipgloss.Color("#7A5A12"),
	AvatarPalette: [AvatarCount]lipgloss.Color{
		"#F472B6", "#60A5FA", "#34D399", "#FBBF24",
		"#A78BFA", "#F87171", "#22D3EE", "#FB923C",
	},

	DiffAddBg: lipgloss.Color("#12261E"),
	DiffDelBg: lipgloss.Color("#2D1619"),

	DiffAddEmphBg: lipgloss.Color("#1F5A3A"),
	DiffDelEmphBg: lipgloss.Color("#6E2630"),

	DiffAddFg:  lipgloss.Color("#7EE2A8"),
	DiffDelFg:  lipgloss.Color("#F7A8A8"),
	DiffHunkFg: lipgloss.Color("#6B8EAE"),

	SearchCurrentBg: lipgloss.Color("#F59E0B"),
	SearchOtherBg:   lipgloss.Color("#22D3EE"),
	SearchMatchFg:   lipgloss.Color("#0F172A"),
	SearchLineBg:    lipgloss.Color("#1D4ED8"),
	SearchActiveFg:  lipgloss.Color("#FDE047"),

	OnPrimary: lipgloss.Color("#FFFFFF"),
	HelpText:  lipgloss.Color("#9CA3AF"),
	BoxText:   lipgloss.Color("#CBD5E1"),
	HintKeyFg: lipgloss.Color("#C9D1D9"),

	Markdown: styles.TokyoNightStyleConfig,

	Base: lipgloss.Color("#0D1117"),
}

// Default constructs a Theme with the standard "Terminal Workshop" palette.
func Default() *Theme {
	return Build(terminalWorkshop)
}

// With returns a new theme built from t's palette after change edits it.
// It is the way to adjust colours: setting a colour on a built theme leaves
// its styles on the old value.
func (t *Theme) With(change func(*Palette)) *Theme {
	p := t.Palette
	change(&p)
	return Build(p)
}

// Build constructs a Theme, deriving every style from p. Colours p leaves
// empty are taken from the default palette, so a partial palette still
// draws tints, cursors and borders.
//
// Build reads the lipgloss colour profile, so call it after the profile is
// set: palettes with Colors256 use them on 256-colour terminals.
func Build(p Palette) *Theme {
	fillMissing(&p, terminalWorkshop)
	if p.Colors256 != nil && lipgloss.ColorProfile() == termenv.ANSI256 {
		p.apply256(p.Colors256)
	}
	t := &Theme{Palette: p}

	// Panel borders.
	t.Panel = lipgloss.NewStyle().
		Border(lipgloss.Border{Left: "│"}, true, false, false, false).
		BorderForeground(t.Border)

	t.PanelFocused = lipgloss.NewStyle().
		Border(lipgloss.Border{Left: "┃"}, true, false, false, false).
		BorderForeground(t.Primary)

	// Section dividers.
	t.Divider = lipgloss.NewStyle().
		Foreground(t.Border)

	// Row styles.
	t.NormalRow = lipgloss.NewStyle()

	// Text styles.
	t.Title = lipgloss.NewStyle().Bold(true)

	t.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Text)

	t.Bold = lipgloss.NewStyle().Bold(true)

	t.MutedTxt = lipgloss.NewStyle().
		Foreground(t.Muted)

	t.FaintTxt = lipgloss.NewStyle().Foreground(t.Faint)
	t.DimTxt = lipgloss.NewStyle().Foreground(t.TextDim)

	t.PrimaryTxt = lipgloss.NewStyle().
		Foreground(t.Primary)

	t.SecondaryTxt = lipgloss.NewStyle().
		Foreground(t.Secondary).
		Bold(true)

	t.Number = lipgloss.NewStyle().
		Foreground(t.Secondary).
		Bold(true)

	// CI / Review status icons.
	t.CISuccess = lipgloss.NewStyle().Foreground(t.Success)
	t.CIFailure = lipgloss.NewStyle().Foreground(t.Error)
	t.CIPending = lipgloss.NewStyle().Foreground(t.Warning)
	t.CIMuted = lipgloss.NewStyle().Foreground(t.Muted)

	t.ReviewApproved = lipgloss.NewStyle().Foreground(t.Success)
	t.ReviewChanges = lipgloss.NewStyle().Foreground(t.Error)
	t.ReviewRequired = lipgloss.NewStyle().Foreground(t.Warning)
	t.ReviewDraft = lipgloss.NewStyle().Foreground(t.Muted)
	t.ReviewMuted = lipgloss.NewStyle().Foreground(t.Muted)

	// Diff stats.
	t.Additions = lipgloss.NewStyle().Foreground(t.Success)
	t.Deletions = lipgloss.NewStyle().Foreground(t.Error)

	// Diff line colors.
	t.DiffAddition = lipgloss.NewStyle().Foreground(t.DiffAddFg).Background(t.DiffAddBg)
	t.DiffDeletion = lipgloss.NewStyle().Foreground(t.DiffDelFg).Background(t.DiffDelBg)
	t.DiffHunkHeader = lipgloss.NewStyle().Foreground(t.DiffHunkFg)

	// Tab bar.
	t.TabActive = lipgloss.NewStyle().
		Foreground(t.AccentText).
		Bold(true).
		Underline(true).
		Padding(0, 1)

	t.SectionTabActive = lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(t.OnPrimary).
		Bold(true).
		Padding(0, 1)

	t.TabInactive = lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1)

	// Status bar.
	t.StatusHelp = lipgloss.NewStyle().
		Foreground(t.HelpText)

	t.StatusLoading = lipgloss.NewStyle().
		Foreground(t.Warning)

	t.StatusStale = lipgloss.NewStyle().
		Foreground(t.Warning)

	t.StatusError = lipgloss.NewStyle().
		Foreground(t.Error).
		Bold(true)

	t.StatusFresh = lipgloss.NewStyle().
		Foreground(t.Success)

	t.StatusSep = lipgloss.NewStyle().
		Foreground(t.Border)

	// Command palette overlay.
	t.BoxBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary)

	t.BoxTitle = lipgloss.NewStyle().
		Foreground(t.Text).
		Bold(true).
		Padding(0, 1)

	t.BoxNormal = lipgloss.NewStyle().
		Foreground(t.BoxText)

	t.BoxFooter = lipgloss.NewStyle().
		Foreground(t.Muted).
		Faint(true)

	t.BoxDiv = lipgloss.NewStyle().
		Foreground(t.Border)

	t.BoxPRNum = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true)
	t.BoxPRAuthor = lipgloss.NewStyle().Foreground(t.Muted)

	// Keymap overlay.
	t.Keycap = lipgloss.NewStyle().
		Foreground(t.Secondary).
		Bold(true)

	t.KeymapHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Primary)

	t.HintKey = lipgloss.NewStyle().Foreground(t.HintKeyFg).Bold(true)
	t.HintDesc = lipgloss.NewStyle().Foreground(t.Muted)

	return t
}

// fillMissing sets every empty colour in p, avatar colours included, and an
// unset markdown style, from def.
func fillMissing(p *Palette, def Palette) {
	if reflect.ValueOf(p.Markdown).IsZero() {
		p.Markdown = def.Markdown
	}
	pv, dv := reflect.ValueOf(p).Elem(), reflect.ValueOf(def)
	for i := range pv.NumField() {
		switch f := pv.Field(i); f.Interface().(type) {
		case lipgloss.Color:
			if f.String() == "" {
				f.Set(dv.Field(i))
			}
		case [AvatarCount]lipgloss.Color:
			for j := range AvatarCount {
				if f.Index(j).String() == "" {
					f.Index(j).Set(dv.Field(i).Index(j))
				}
			}
		}
	}
}
