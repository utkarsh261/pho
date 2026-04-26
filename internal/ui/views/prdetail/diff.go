package prdetail

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
)

// maxDiffDisplayRows is the cap on rendered diff rows before a truncation banner is shown.
const maxDiffDisplayRows = 20000

// diffFileHeaderRows is the number of display rows before the first hunk
// header in each diff file: blank padding + dashed separator + file header bar.
const diffFileHeaderRows = 3

// diffFileDisplayRows returns the UI display-row count for one DiffFile:
//
//	row 0   : blank padding before separator
//	row 1   : dashed separator line
//	row 2   : file header bar (styled background + bold)
//	row 3.. : hunk header + diff lines (repeated per hunk)
//	        : binary files get exactly 1 placeholder row at row 3
//
// This is the authoritative source for per-file row counts used by both
// diffSectionRowCount and renderDiffSectionLines, so they stay in sync
// regardless of what f.DisplayRows holds (legacy cache entries may have 0).
func diffFileDisplayRows(f *diffmodel.DiffFile) int {
	rows := diffFileHeaderRows // blank + separator + header bar
	if f.IsBinary {
		return rows + 1 // +1 for the "📄 Binary file (no diff available)" placeholder row
	}
	for _, h := range f.Hunks {
		rows++ // hunk header
		rows += len(h.Lines)
	}
	return rows
}

// diffSectionRowCount returns the number of display rows for the Diff section.
// Always derives the count from hunk structure via diffFileDisplayRows so that
// legacy cache entries with DisplayRows==0 are handled correctly.
//
// Returns 0 only when the diff is not loading and not loaded (truly absent).
// Returns 1 for a loading placeholder or an empty loaded diff.
// Caps at maxDiffDisplayRows+1 when the raw total exceeds the limit; the +1
// reserves a row for the truncation banner.
func (m *PRDetailModel) diffSectionRowCount() int {
	if m.Diff == nil {
		if m.DiffLoading {
			return 1 // "Loading diff…" placeholder
		}
		return 0 // not loaded, not loading — truly absent
	}
	if len(m.Diff.Files) == 0 {
		return 1 // "No changes" placeholder
	}
	total := 0
	for i := range m.Diff.Files {
		total += diffFileDisplayRows(&m.Diff.Files[i])
	}
	if total == 0 {
		return 1 // safety
	}
	if total > maxDiffDisplayRows {
		return maxDiffDisplayRows + 1 // cap + banner row
	}
	return total
}

// renderDiffTab renders the Diff tab content at the given scroll and viewport
// dimensions. Returns exactly contentH lines (blank-padded).
func (m *PRDetailModel) renderDiffTab(scroll, contentH, contentWidth int) []string {
	localStart := scroll
	localEnd := scroll + contentH
	return m.renderDiffSectionLines(localStart, localEnd, contentWidth)
}

// renderDiffSectionLines renders the diff section rows [localStart, localEnd).
// Applies file-level virtualization: only files whose row ranges overlap
// [localStart, localEnd) are processed. Rendering stops at maxDiffDisplayRows;
// a truncation banner is injected at that position when the diff is larger.
//
// Row layout per DiffFile (via diffFileDisplayRows):
//
//	row 0   : blank line (padding before separator)
//	row 1   : dashed separator "╌╌╌╌╌" (muted)
//	row 2   : file header bar  (Subtle bg, bold, full-width)
//	row 3   : first hunk header "@@ … @@" (cyan+bold) — or binary placeholder
//	rows 4+ : diff lines: additions (green), deletions (red), context (plain)
//	          second hunk header if any, then its lines, etc.
func (m *PRDetailModel) renderDiffSectionLines(localStart, localEnd, contentWidth int) []string {
	n := localEnd - localStart
	out := make([]string, n)

	if m.Diff == nil || len(m.Diff.Files) == 0 {
		if n > 0 {
			if m.DiffLoading && m.Diff == nil {
				out[0] = "Loading diff…"
			} else {
				out[0] = "No changes"
			}
		}
		return out
	}

	cw := max(contentWidth, 1)

	// Determine whether truncation is needed (recompute real total here).
	realTotal := 0
	for i := range m.Diff.Files {
		realTotal += diffFileDisplayRows(&m.Diff.Files[i])
	}
	needsTruncation := realTotal > maxDiffDisplayRows

	// Build themed styles once.
	var separatorLine string
	var fileHeaderStyle lipgloss.Style
	var truncStyle lipgloss.Style

	hunkHeaderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE")).Bold(true)
	additionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
	deletionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	currentWordStyle, otherWordStyle, currentLineBg := m.searchHighlightStyles()
	searchCtx := m.buildSearchRenderContext()

	dashStr := strings.Repeat("╌", cw)
	if m.theme != nil {
		hunkHeaderStyle = m.theme.DiffHunkHeader
		additionStyle = m.theme.DiffAddition
		deletionStyle = m.theme.DiffDeletion
		separatorLine = m.theme.MutedTxt.Render(dashStr)
		fileHeaderStyle = lipgloss.NewStyle().
			Background(m.theme.Subtle).
			Bold(true).
			Width(cw)
		truncStyle = m.theme.MutedTxt
	} else {
		separatorLine = dashStr
		fileHeaderStyle = lipgloss.NewStyle().Bold(true).Width(cw)
		truncStyle = lipgloss.NewStyle()
	}

	outIdx := 0
	fileRow := 0
	globalLineIndex := 0

	for i := range m.Diff.Files {
		// Stop iterating once we've passed the truncation boundary.
		if fileRow >= maxDiffDisplayRows {
			break
		}

		f := &m.Diff.Files[i]
		dr := diffFileDisplayRows(f)

		// Clamp the file's effective end to the truncation limit.
		effectiveEnd := fileRow + dr
		if effectiveEnd > maxDiffDisplayRows {
			effectiveEnd = maxDiffDisplayRows
		}

		overlapStart := max(fileRow, localStart)
		overlapEnd := min(effectiveEnd, localEnd)
		if overlapStart >= overlapEnd {
			fileRow += dr
			globalLineIndex += diffFileLineCount(f)
			continue
		}

		// Build the flat display-row slice for this file.
		type displayRow struct{ text string }
		rows := make([]displayRow, 0, dr)

		// row 0: blank padding
		rows = append(rows, displayRow{""})

		// row 1: dashed separator
		rows = append(rows, displayRow{separatorLine})

		// row 2: file header bar
		var label string
		if f.Status == "renamed" && f.OldPath != "" && f.OldPath != f.NewPath {
			label = " " + f.OldPath + " → " + f.NewPath
		} else if f.NewPath != "" {
			label = " " + f.NewPath
		} else {
			label = " " + f.OldPath
		}
		rows = append(rows, displayRow{fileHeaderStyle.Render(label)})

		if f.IsBinary {
			// row 3: binary placeholder (no hunk content)
			rows = append(rows, displayRow{truncStyle.Render("📄 Binary file (no diff available)")})
		} else {
			// rows 3+: hunk headers + diff lines
			for hi, hunk := range f.Hunks {
				rows = append(rows, displayRow{hunkHeaderStyle.Render(hunk.Header)})
				for li, dl := range hunk.Lines {
					baseStyle := lipgloss.NewStyle()
					switch dl.Kind {
					case "addition":
						baseStyle = additionStyle
					case "deletion":
						baseStyle = deletionStyle
					}
					s := m.renderSearchMatchLine(
						dl.Raw,
						i,
						globalLineIndex,
						searchCtx,
						baseStyle,
						currentWordStyle,
						otherWordStyle,
						currentLineBg,
					)

					// Apply visual selection highlight, cursor highlight, or draft indicator.
					isSelected := m.visual.Active && m.visual.FileIdx == i && m.visual.HunkIdx == hi &&
						li >= m.visual.StartLine && li <= m.visual.EndLine
					isCursor := !isSelected && m.activeTab == TabDiff && m.leftPanel.Focus == FocusContent &&
						m.validDiffCursor() &&
						m.diffCursor.FileIdx == i && m.diffCursor.HunkIdx == hi && m.diffCursor.LineIdx == li
					isDrafted := !isSelected && !isCursor && m.draftCovered[hunkLineKey{i, hi, li}]

					if isSelected {
						if m.theme != nil {
							s = m.theme.ListSelected.Width(cw).Render(s)
						} else {
							s = lipgloss.NewStyle().Reverse(true).Width(cw).Render(s)
						}
					} else if isCursor {
						s = lipgloss.NewStyle().Reverse(true).Width(cw).Render(s)
					} else if isDrafted {
						if m.theme != nil {
							s = m.theme.ListOpened.Width(cw).Render(s)
						}
					}

					rows = append(rows, displayRow{s})
					globalLineIndex++
				}
			}
		}

		// Pad to dr if content is shorter (e.g. non-binary file with no hunks).
		for len(rows) < dr {
			rows = append(rows, displayRow{""})
		}

		// Emit only the rows that overlap [overlapStart, overlapEnd).
		for row := overlapStart; row < overlapEnd; row++ {
			localRow := row - fileRow
			if outIdx < n {
				if localRow < len(rows) {
					out[outIdx] = rows[localRow].text
				} else {
					out[outIdx] = ""
				}
				outIdx++
			}
		}

		fileRow += dr
	}

	// Inject truncation banner at row maxDiffDisplayRows if needed and in window.
	if needsTruncation {
		bannerIdx := maxDiffDisplayRows - localStart
		if bannerIdx >= 0 && bannerIdx < n {
			out[bannerIdx] = truncStyle.Render("… diff truncated (too large to display)")
		}
	}

	return out
}
