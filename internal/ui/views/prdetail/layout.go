package prdetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var panelHeadBorder = lipgloss.Border{
	Top:         "─",
	Bottom:      "─",
	Left:        "│",
	Right:       "│",
	TopLeft:     "┌",
	TopRight:    "┐",
	BottomLeft:  "├",
	BottomRight: "┤",
}

const (
	// LeftPanelWidth is the fixed outer width of the left panel (including side borders).
	LeftPanelWidth = 42
	// MinWidthForSidebar is the minimum terminal width at which the sidebar is shown.
	MinWidthForSidebar = 80

	// lpInner is the usable content width inside the left panel border and padding.
	lpInner = LeftPanelWidth - 4

	lpPaddingWidth = 2
	lpStatsWidth   = 10
	lpPathMax      = lpInner - lpPaddingWidth - lpStatsWidth

	lpCIIconWidth   = 2
	lpCIStatusWidth = 5
	lpCINameMax     = lpInner - lpCIIconWidth - 1 - lpCIStatusWidth
)

// truncatePathLeft truncates path from the LEFT so the filename (right side) stays visible.
// Returns a string of exactly maxWidth runes: padded with spaces if shorter than maxWidth,
// or "…" + rightmost (maxWidth-1) runes if longer.
func truncatePathLeft(path string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(path)
	if len(runes) <= maxWidth {
		return path + strings.Repeat(" ", maxWidth-len(runes))
	}
	if maxWidth == 1 {
		return "…"
	}
	// Keep the rightmost (maxWidth-1) runes, prepend "…".
	return "…" + string(runes[len(runes)-(maxWidth-1):])
}

// formatFileStats formats additions/deletions into a string of exactly lpStatsWidth visible chars.
// Output format: " +N -N" right-aligned (with a leading space).
// Uses rune-width arithmetic so "…" (1 rune) counts as 1 char, not 3 bytes.
func formatFileStats(additions, deletions int) string {
	inner := []rune(fmt.Sprintf("+%d -%d", additions, deletions))
	budget := lpStatsWidth - 1 // Leave at least 1 leading space visually
	if len(inner) > budget {
		inner = append(inner[:budget-1], '…')
	}
	return lipgloss.NewStyle().Width(lpStatsWidth).Align(lipgloss.Right).Render(string(inner))
}

func computeCIHeight(viewportHeight, numChecks int) int {
	if numChecks == 0 {
		return 0
	}
	if viewportHeight < 10 {
		return 0
	}
	maxH := max(int(float64(viewportHeight)*0.3), 5)
	contentH := max(numChecks, 1)
	// 4 overhead rows + actual list rows
	h := min(max(4+contentH, 5), maxH)
	return h
}

// clamp returns v clamped to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func truncateText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := lipgloss.Width(s)
	if visible <= width {
		return s
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	// Truncate using lipgloss which handles ANSI correctly
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func fitLine(s string, width int) string {
	// Use lipgloss Width() so ANSI background/padding is preserved.
	truncated := truncateText(s, width)
	return lipgloss.NewStyle().Width(width).Render(truncated)
}

func renderBlock(lines []string, width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	out := make([]string, 0, height)
	for i := range height {
		if i < len(lines) {
			out = append(out, fitLine(lines[i], width))
			continue
		}
		out = append(out, strings.Repeat(" ", width))
	}
	return strings.Join(out, "\n")
}

func wrapParagraph(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if lipgloss.Width(candidate) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current = candidate
	}
	lines = append(lines, current)
	return lines
}
