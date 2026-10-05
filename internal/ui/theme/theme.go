// Package theme defines all lipgloss styles for the pho terminal UI.
// A single Theme struct is constructed once at startup and passed to every
// panel model — no globals, no side effects.
package theme

import "github.com/charmbracelet/lipgloss"

// Theme holds every reusable lipgloss style used across the application.
// Call Default() to get the standard colour palette, or construct your own
// for a custom look.
type Theme struct {
	// ── colour values ──────────────────────────────────────────────
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
	AvatarPalette []lipgloss.Color

	// Diff line backgrounds.
	DiffAddBg lipgloss.Color
	DiffDelBg lipgloss.Color
	// Stronger backgrounds for the words that changed within a line.
	DiffAddEmphBg lipgloss.Color
	DiffDelEmphBg lipgloss.Color

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

// Default constructs a Theme with the standard "Terminal Workshop" palette.
func Default() *Theme {
	t := &Theme{
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
		AvatarPalette: []lipgloss.Color{
			"#F472B6", "#60A5FA", "#34D399", "#FBBF24",
			"#A78BFA", "#F87171", "#22D3EE", "#FB923C",
		},

		DiffAddBg: lipgloss.Color("#12261E"),
		DiffDelBg: lipgloss.Color("#2D1619"),

		DiffAddEmphBg: lipgloss.Color("#1F5A3A"),
		DiffDelEmphBg: lipgloss.Color("#6E2630"),
	}

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
	t.DiffAddition = lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE2A8")).Background(t.DiffAddBg)
	t.DiffDeletion = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7A8A8")).Background(t.DiffDelBg)
	t.DiffHunkHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B8EAE"))

	// Tab bar.
	t.TabActive = lipgloss.NewStyle().
		Foreground(t.AccentText).
		Bold(true).
		Underline(true).
		Padding(0, 1)

	t.SectionTabActive = lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	t.TabInactive = lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1)

	// Status bar.
	t.StatusHelp = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

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
		Foreground(lipgloss.Color("#CBD5E1"))

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

	t.HintKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#C9D1D9")).Bold(true)
	t.HintDesc = lipgloss.NewStyle().Foreground(t.Muted)

	return t
}
