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

	// ── panel borders ──────────────────────────────────────────────
	Panel        lipgloss.Style // normal panel left border (gray)
	PanelFocused lipgloss.Style // focused panel left border (violet)

	// ── section dividers ───────────────────────────────────────────
	Divider lipgloss.Style // horizontal rule in Border color

	// ── row styles ─────────────────────────────────────────────────
	SelectedRow lipgloss.Style // subtle background tint
	NormalRow   lipgloss.Style // no decoration

	// ── text styles ────────────────────────────────────────────────
	Title       lipgloss.Style // bold
	Header      lipgloss.Style // panel heading bar (inverted bg + bold label)
	Bold        lipgloss.Style // bold, normal fg
	MutedTxt    lipgloss.Style // muted fg
	PrimaryTxt  lipgloss.Style // primary fg colour
	SecondaryTxt lipgloss.Style // secondary fg + bold
	Number      lipgloss.Style // secondary fg + bold (for #123 PR numbers)

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

	// ── tab bar ────────────────────────────────────────────────────
	TabActive   lipgloss.Style // violet bg + white text + padding
	TabInactive lipgloss.Style // muted text + padding

	// ── status bar ─────────────────────────────────────────────────
	StatusHelp    lipgloss.Style // muted + faint
	StatusLoading lipgloss.Style // warning
	StatusStale   lipgloss.Style // warning
	StatusError   lipgloss.Style // error + bold
	StatusFresh   lipgloss.Style // success
	StatusSep     lipgloss.Style // border colour

	// ── overlay / command palette ──────────────────────────────────
	BoxBorder   lipgloss.Style // centred box with primary border
	BoxTitle    lipgloss.Style // centred, bold, primary
	BoxQuery    lipgloss.Style // bold text
	BoxCursor   lipgloss.Style // primary cursor marker
	BoxSelected lipgloss.Style // primary fg + bold
	BoxNormal   lipgloss.Style // muted
	BoxFooter   lipgloss.Style // muted, faint
	BoxDiv      lipgloss.Style // border colour divider
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
		Border:    lipgloss.Color("#374151"),
		Subtle:    lipgloss.Color("#1F2937"),
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
	t.SelectedRow = lipgloss.NewStyle().
		Background(t.Subtle)

	t.NormalRow = lipgloss.NewStyle()

	// Text styles.
	t.Title = lipgloss.NewStyle().Bold(true)

	t.Header = lipgloss.NewStyle().
		Background(lipgloss.Color("#1E293B")).
		Bold(true).
		Foreground(lipgloss.Color("#E2E8F0"))

	t.Bold = lipgloss.NewStyle().Bold(true)

	t.MutedTxt = lipgloss.NewStyle().
		Foreground(t.Muted)

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

	// Tab bar.
	t.TabActive = lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	t.TabInactive = lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1)

	// Status bar.
	t.StatusHelp = lipgloss.NewStyle().
		Foreground(t.Muted).
		Faint(true)

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
		Foreground(t.Primary).
		Bold(true)

	t.BoxQuery = lipgloss.NewStyle().Bold(true)

	t.BoxCursor = lipgloss.NewStyle().
		Foreground(t.Primary)

	t.BoxSelected = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	t.BoxNormal = lipgloss.NewStyle().
		Foreground(t.Muted)

	t.BoxFooter = lipgloss.NewStyle().
		Foreground(t.Muted).
		Faint(true)

	t.BoxDiv = lipgloss.NewStyle().
		Foreground(t.Border)

	return t
}
