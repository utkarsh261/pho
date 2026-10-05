package prdetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// maxDiffDisplayRows is the cap on rendered diff rows before a truncation banner is shown.
const maxDiffDisplayRows = 20000

// diffFileHeaderRows is the number of display rows before the first hunk
// header in each diff file: blank padding + file header bar + blank padding.
const diffFileHeaderRows = 3

// diffFileDisplayRows returns the UI display-row count for one DiffFile:
//
//	row 0   : blank padding
//	row 1   : file header bar (status, path, stats)
//	row 2   : blank padding
//	row 3.. : hunk header + diff lines (repeated per hunk)
//	        : binary files get exactly 1 placeholder row at row 3
//
// This is the authoritative source for per-file row counts used by both
// diffSectionRowCount and renderDiffSectionLines, so they stay in sync
// regardless of what f.DisplayRows holds (legacy cache entries may have 0).
func diffFileDisplayRows(f *diffmodel.DiffFile) int {
	rows := diffFileHeaderRows // blank + header bar + blank
	if f.IsBinary {
		return rows + 1 // +1 for the "Binary file (no diff available)" placeholder row
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
func (m *PRDetailModel) renderDiffSectionLines(localStart, localEnd, contentWidth int) []string {
	defer m.log().Timer("render diff section")()
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
	var truncStyle lipgloss.Style

	hunkHeaderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE")).Bold(true)
	additionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
	deletionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	currentWordStyle, otherWordStyle, currentLineBg := m.searchHighlightStyles()
	searchCtx := m.buildSearchRenderContext()

	if m.theme != nil {
		hunkHeaderStyle = m.theme.DiffHunkHeader
		additionStyle = m.theme.DiffAddition
		deletionStyle = m.theme.DiffDeletion
		truncStyle = m.theme.MutedTxt
	} else {
		truncStyle = lipgloss.NewStyle()
	}

	outIdx := 0
	fileRow := 0
	globalLineIndex := 0

	// Pre-compute cursor validity once; avoid O(n²) from calling
	// validDiffCursor (which calls diffLineToDisplayRow) inside the line loop.
	hasValidCursor := m.validDiffCursor()
	cursorFileIdx, cursorHunkIdx, cursorLineIdx := -1, -1, -1
	if hasValidCursor {
		cursorFileIdx = m.diffCursor.FileIdx
		cursorHunkIdx = m.diffCursor.HunkIdx
		cursorLineIdx = m.diffCursor.LineIdx
	}

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

		// row 1: file header bar
		if fileRow+1 >= overlapStart && fileRow+1 < overlapEnd {
			rows = append(rows, displayRow{m.renderDiffFileHeader(f, cw)})
		} else {
			rows = append(rows, displayRow{})
		}

		// row 2: breathing room under the header
		rows = append(rows, displayRow{""})

		if f.IsBinary {
			// row 3: binary placeholder (no hunk content)
			rows = append(rows, displayRow{truncStyle.Render("      Binary file (no diff available)")})
		} else {
			// rows 3+: hunk headers + diff lines
			// Only rows inside [overlapStart, overlapEnd) are styled; the rest get
			// an empty placeholder so row indices (and globalLineIndex, which
			// search relies on) stay aligned. This keeps a frame proportional to
			// the viewport instead of the file.
			inWindow := func(local int) bool {
				r := fileRow + local
				return r >= overlapStart && r < overlapEnd
			}
			for hi, hunk := range f.Hunks {
				hunkStart := len(rows)
				if inWindow(hunkStart) {
					rows = append(rows, displayRow{m.diffGutter("", " ⋯") + hunkHeaderStyle.Render(hunk.Header)})
				} else {
					rows = append(rows, displayRow{})
				}
				if fileRow+hunkStart+1+len(hunk.Lines) <= overlapStart || fileRow+hunkStart+1 >= overlapEnd {
					// Hunk body entirely outside the window: placeholders only.
					for range hunk.Lines {
						rows = append(rows, displayRow{})
					}
					globalLineIndex += len(hunk.Lines)
					continue
				}
				var changed map[int]byteRange
				if m.theme != nil {
					changed = m.hunkWordDiffs(&f.Hunks[hi])
				}
				for li, dl := range hunk.Lines {
					if !inWindow(len(rows)) {
						rows = append(rows, displayRow{})
						globalLineIndex++
						continue
					}
					isSelected := m.visual.Active && m.visual.FileIdx == i && m.visual.HunkIdx == hi &&
						li >= m.visual.StartLine && li <= m.visual.EndLine
					isCursor := !isSelected && m.isInDiffSection() &&
						hasValidCursor &&
						cursorFileIdx == i && cursorHunkIdx == hi && cursorLineIdx == li
					isDrafted := !isSelected && !isCursor && m.draftCovered[hunkLineKey{i, hi, li}]

					baseStyle := lipgloss.NewStyle()
					var lineBg lipgloss.Color
					switch dl.Kind {
					case "addition":
						baseStyle = additionStyle
						if m.theme != nil {
							lineBg = m.theme.DiffAddBg
						}
					case "deletion":
						baseStyle = deletionStyle
						if m.theme != nil {
							lineBg = m.theme.DiffDelBg
						}
					}
					if isSelected || isCursor || isDrafted {
						// The row tint replaces the add/delete background.
						baseStyle = baseStyle.UnsetBackground()
						lineBg = ""
					}
					var s string
					if rg, ok := changed[li]; ok && lineBg != "" && rg.end > rg.start &&
						len(searchCtx.lineRanges[globalLineIndex]) == 0 {
						// Emphasise the words that changed against the paired line.
						emphBg := m.theme.DiffAddEmphBg
						if dl.Kind == "deletion" {
							emphBg = m.theme.DiffDelEmphBg
						}
						s = baseStyle.Render(dl.Raw[:rg.start]) +
							baseStyle.Background(emphBg).Render(dl.Raw[rg.start:rg.end]) +
							baseStyle.Render(dl.Raw[rg.end:])
					} else {
						s = m.renderSearchMatchLine(
							dl.Raw,
							i,
							globalLineIndex,
							searchCtx,
							baseStyle,
							currentWordStyle,
							otherWordStyle,
							currentLineBg,
						)
					}

					marker := ""
					switch {
					case isSelected:
						marker = "selected"
					case isCursor:
						marker = "cursor"
					case isDrafted:
						marker = "draft"
					}
					gutter := m.diffGutter(marker, diffLineNumber(dl))
					bodyW := max(cw-diffGutterWidth, 1)

					switch {
					case m.theme == nil && (isSelected || isCursor):
						s = lipgloss.NewStyle().Reverse(true).Width(cw).Render(gutter + s)
					case m.theme == nil:
						s = gutter + truncateText(s, bodyW)
					case isSelected:
						s = theme.FillBg(m.theme.Selection, cw, gutter+s)
					case isCursor:
						s = theme.FillBg(m.theme.Highlight, cw, gutter+s)
					case lineBg != "":
						s = gutter + theme.FillBg(lineBg, bodyW, s)
					case isDrafted:
						s = theme.FillBg(m.theme.Subtle, cw, gutter+s)
					default:
						s = gutter + truncateText(s, bodyW)
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

// diffGutterWidth is the width of the left gutter on every diff line:
// 1 marker column + a 5-column line-number field. Numbers up to 9999 get a
// trailing space; 5-digit numbers use the whole field.
const diffGutterWidth = 6

// diffLineNumber returns the line number shown in the gutter: the new-side
// number for additions/context, the old-side number for deletions.
func diffLineNumber(dl diffmodel.DiffLine) string {
	n := dl.NewLine
	if n == nil {
		n = dl.OldLine
	}
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%d", *n)
}

// diffGutter renders the marker + right-aligned line number gutter, always
// exactly diffGutterWidth cells wide. marker is "", "cursor", "selected" or "draft".
func (m *PRDetailModel) diffGutter(marker, num string) string {
	const field = diffGutterWidth - 1
	numStr := fmt.Sprintf("%4s ", num)
	if w := lipgloss.Width(numStr); w > field {
		numStr = fmt.Sprintf("%*s", field, num) // 5 digits: drop the trailing space
		if r := []rune(numStr); len(r) > field {
			numStr = "…" + string(r[len(r)-field+1:]) // 6+ digits: keep the low digits, mark the cut
		}
	}
	if m.theme == nil {
		mk := " "
		if marker == "draft" {
			mk = "●"
		}
		return mk + numStr
	}
	numStyle := m.theme.FaintTxt
	mk := " "
	switch marker {
	case "cursor", "selected":
		mk = m.theme.PrimaryTxt.Render("▎")
		numStyle = lipgloss.NewStyle().Foreground(m.theme.Text)
	case "draft":
		mk = m.theme.CIPending.Render("●")
	}
	return mk + numStyle.Render(numStr)
}

// renderDiffFileHeader renders the per-file header bar:
// status letter, path (or old → new), and +/- stats right-aligned.
func (m *PRDetailModel) renderDiffFileHeader(f *diffmodel.DiffFile, cw int) string {
	var label string
	if f.Status == "renamed" && f.OldPath != "" && f.OldPath != f.NewPath {
		label = f.OldPath + " → " + f.NewPath
	} else if f.NewPath != "" {
		label = f.NewPath
	} else {
		label = f.OldPath
	}
	if m.theme == nil {
		return lipgloss.NewStyle().Bold(true).Width(cw).Render(" " + label)
	}
	th := m.theme
	stats := th.Additions.Render(fmt.Sprintf("+%d", f.Additions)) + " " + th.Deletions.Render(fmt.Sprintf("-%d", f.Deletions)) +
		"  " + diffStatBlocks(f.Additions, f.Deletions, th)
	left := " " + fileStatusLetter(*f, th) + "  " + lipgloss.NewStyle().Bold(true).Foreground(th.Text).Render(label)
	maxLeft := max(cw-lipgloss.Width(stats)-2, 1)
	if lipgloss.Width(left) > maxLeft {
		left = truncateText(left, maxLeft)
	}
	gap := max(cw-lipgloss.Width(left)-lipgloss.Width(stats)-1, 1)
	return theme.FillBg(th.Subtle, cw, left+strings.Repeat(" ", gap)+stats)
}

// diffStatBlocks renders GitHub's five-square change bar: green squares for
// the share of added lines, red for deleted, grey when nothing changed.
func diffStatBlocks(additions, deletions int, th *theme.Theme) string {
	const n = 5
	total := additions + deletions
	if total == 0 {
		return th.FaintTxt.Render(strings.Repeat("■", n))
	}
	green := (additions*n + total/2) / total
	if additions > 0 && green == 0 {
		green = 1
	}
	if deletions > 0 && green == n {
		green = n - 1
	}
	return th.Additions.Render(strings.Repeat("■", green)) + th.Deletions.Render(strings.Repeat("■", n-green))
}

// hunkWordDiffs returns the cached intra-line changes for h, computing them
// on first use. The cache is dropped when m.Diff is replaced.
func (m *PRDetailModel) hunkWordDiffs(h *diffmodel.DiffHunk) map[int]byteRange {
	if m.wordDiffsFor != m.Diff {
		m.wordDiffs, m.wordDiffsFor = nil, m.Diff
	}
	if r, ok := m.wordDiffs[h]; ok {
		return r
	}
	if m.wordDiffs == nil {
		m.wordDiffs = make(map[*diffmodel.DiffHunk]map[int]byteRange)
	}
	r := intraLineChanges(h.Lines)
	m.wordDiffs[h] = r
	return r
}
