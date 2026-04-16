package prdetail

import (
	"fmt"
	"strings"

	diffmodel "github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/domain"
)

// ContentSection holds the display-row range for one section of the unified content viewport.
// Sections are always ordered: Description → Diff → Comments.
type ContentSection struct {
	Section  domain.PRDetailSection // SectionDescription, SectionDiff, or SectionComments
	StartRow int                    // first absolute display row in the viewport
	RowCount int                    // number of display rows; 0 means the section is skipped
}

// buildContentSections precomputes the section slice based on the current model state.
// contentWidth is the usable text-column width inside the content area.
//
// Rules:
//   - Description RowCount = 0 when no body is available (section visually absent).
//   - Comments section is omitted entirely when there are no review nodes.
//   - All other sections are always present (Diff always ≥ 1 row when loaded or loading).
func (m *PRDetailModel) buildContentSections(contentWidth int) []ContentSection {
	cursor := 0

	// ── Description ──────────────────────────────────────────────────────────
	descLines := m.descriptionLines(contentWidth)
	desc := ContentSection{
		Section:  domain.SectionDescription,
		StartRow: cursor,
		RowCount: len(descLines),
	}
	cursor += len(descLines)

	// ── Diff ─────────────────────────────────────────────────────────────────
	diffCount := m.diffSectionRowCount()
	diff := ContentSection{
		Section:  domain.SectionDiff,
		StartRow: cursor,
		RowCount: diffCount,
	}
	cursor += diffCount

	sections := []ContentSection{desc, diff}

	// ── Comments (omitted when empty) ────────────────────────────────────────
	cLines := m.commentLines()
	if len(cLines) > 0 {
		sections = append(sections, ContentSection{
			Section:  domain.SectionComments,
			StartRow: cursor,
			RowCount: len(cLines),
		})
	}

	return sections
}

// totalRowsInSections returns the sum of all section RowCounts.
func totalRowsInSections(sections []ContentSection) int {
	n := 0
	for _, s := range sections {
		n += s.RowCount
	}
	return n
}

// activeSectionAt returns the section whose range contains scroll.
// Used by the scroll-spy tab indicators to decide which label to highlight.
func activeSectionAt(sections []ContentSection, scroll int) domain.PRDetailSection {
	active := domain.SectionDescription
	for _, s := range sections {
		if s.RowCount == 0 {
			continue
		}
		if scroll >= s.StartRow {
			active = s.Section
		}
	}
	return active
}

// findSection returns the ContentSection for target and whether it has RowCount > 0.
func findSection(sections []ContentSection, target domain.PRDetailSection) (ContentSection, bool) {
	for _, s := range sections {
		if s.Section == target {
			return s, s.RowCount > 0
		}
	}
	return ContentSection{}, false
}

// diffFileDisplayRows returns the number of display rows a DiffFile occupies:
// 1 (file header) + 1 per hunk header + 1 per diff line.
// Mirrors parse.fileDisplayRows but is available inside the UI package so
// the model can patch legacy cache entries that have DisplayRows == 0.
func diffFileDisplayRows(f *diffmodel.DiffFile) int {
	rows := 1 // file header
	for _, h := range f.Hunks {
		rows++ // hunk header
		rows += len(h.Lines)
	}
	return rows
}

// ── Source line generators ────────────────────────────────────────────────────

// descriptionLines returns the display lines for the Description section.
// Returns nil (RowCount = 0) when no body is available or body is empty.
func (m *PRDetailModel) descriptionLines(contentWidth int) []string {
	if m.Detail == nil {
		if m.DetailLoading {
			return []string{"Loading…"}
		}
		return nil
	}
	body := strings.TrimSpace(m.Detail.BodyExcerpt)
	if body == "" {
		return nil // empty description → section omitted
	}

	cw := max(contentWidth, 1)
	var lines []string

	// Section header: bold + muted
	header := "Description"
	if m.theme != nil {
		header = m.theme.MutedTxt.Bold(true).Render(header)
	}
	lines = append(lines, header)
	// Divider rule
	lines = append(lines, strings.Repeat("─", cw))
	// Word-wrapped body
	lines = append(lines, wrapParagraph(body, cw)...)
	return lines
}

// diffSectionRowCount returns the number of display rows for the Diff section.
//
// Returns 0 only when the diff is not loading and not loaded (truly absent).
// Returns 1 for a loading placeholder or an empty loaded diff.
// Returns sum-of-DisplayRows for a loaded diff with files.
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
	for _, f := range m.Diff.Files {
		total += f.DisplayRows
	}
	if total == 0 {
		return 1 // safety: at least one row
	}
	return total
}

// commentLines returns the display lines for the Comments section.
// Returns nil when there are no review nodes to display.
func (m *PRDetailModel) commentLines() []string {
	if m.Detail == nil {
		return nil
	}
	var lines []string
	for _, r := range m.Detail.Reviewers {
		if r.Login == "" {
			continue
		}
		state := r.State
		if state == "" {
			state = "commented"
		}
		lines = append(lines, fmt.Sprintf("@%s: %s", r.Login, state))
	}
	return lines
}

// ── Render ────────────────────────────────────────────────────────────────────

// renderContentLines renders the visible content lines using the overscan algorithm.
// Returns exactly contentH lines (blank-padded as needed).
//
// overscan = 100 rows pre-rendered on each side of the visible window prevents blank
// flicker when the user scrolls quickly.
func (m *PRDetailModel) renderContentLines(
	sections []ContentSection, scroll, contentH, contentWidth int,
) []string {
	total := totalRowsInSections(sections)

	if total == 0 {
		// All sections empty → centered "No content" message.
		out := make([]string, contentH)
		mid := contentH / 2
		msg := "No content"
		if m.theme != nil {
			msg = m.theme.MutedTxt.Render(msg)
		}
		blank := strings.Repeat(" ", max(contentWidth, 0))
		for i := range out {
			if i == mid {
				out[i] = msg
			} else {
				out[i] = blank
			}
		}
		return out
	}

	const overscan = 100
	renderStart := max(0, scroll-overscan)
	renderEnd := min(total, scroll+contentH+overscan)

	// Pre-compute source lines once (cheap for desc + comments).
	descLines := m.descriptionLines(contentWidth)
	cLines := m.commentLines()

	// collected maps absolute row index → rendered string.
	collected := make(map[int]string, contentH+overscan*2)

	for _, sec := range sections {
		if sec.RowCount == 0 {
			continue
		}
		secEnd := sec.StartRow + sec.RowCount
		overlapStart := max(sec.StartRow, renderStart)
		overlapEnd := min(secEnd, renderEnd)
		if overlapStart >= overlapEnd {
			continue
		}
		localStart := overlapStart - sec.StartRow
		localEnd := overlapEnd - sec.StartRow

		switch sec.Section {
		case domain.SectionDescription:
			for i := localStart; i < localEnd; i++ {
				if i < len(descLines) {
					collected[sec.StartRow+i] = descLines[i]
				}
			}
		case domain.SectionDiff:
			diffLines := m.renderDiffSectionLines(localStart, localEnd, contentWidth)
			for i, line := range diffLines {
				collected[sec.StartRow+localStart+i] = line
			}
		case domain.SectionComments:
			for i := localStart; i < localEnd; i++ {
				if i < len(cLines) {
					collected[sec.StartRow+i] = cLines[i]
				}
			}
		}
	}

	blank := strings.Repeat(" ", max(contentWidth, 0))
	out := make([]string, contentH)
	for i := range contentH {
		if line, ok := collected[scroll+i]; ok {
			out[i] = line
		} else {
			out[i] = blank
		}
	}
	return out
}

// renderDiffSectionLines renders the diff section rows [localStart, localEnd).
// Applies file-level virtualization: only files whose row ranges overlap
// [localStart, localEnd) are processed.
//
// Each DiffFile maps to display rows as follows:
//
//	row 0         : file header  ("─── path" or "─── old → new")
//	row 1         : first hunk header  ("@@ … @@")
//	rows 2..N     : diff lines from that hunk (raw text with +/- prefix)
//	row N+1       : second hunk header (if any), then its lines, etc.
//
// This gives the user readable diff content without requiring Chunk F's
// full syntax-highlighted renderer.
func (m *PRDetailModel) renderDiffSectionLines(localStart, localEnd, _ int) []string {
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

	// Walk files using locally-computed cumulative row offsets.
	// (DiffFile.StartRow is authoritative when set by the service; we recompute
	// here so tests that skip the service still get correct virtualization.)
	outIdx := 0
	fileRow := 0

	for _, f := range m.Diff.Files {
		dr := f.DisplayRows
		if dr <= 0 {
			dr = 1
		}
		fileEnd := fileRow + dr

		overlapStart := max(fileRow, localStart)
		overlapEnd := min(fileEnd, localEnd)
		if overlapStart >= overlapEnd {
			fileRow += dr
			continue
		}

		// Build the flat display-row slice for this file only for the rows we need.
		// Row layout: [fileHeader, hunkHeader, line…, hunkHeader, line…, …]
		type displayRow struct{ text string }
		rows := make([]displayRow, 0, dr)

		// row 0: file header
		if f.IsBinary {
			rows = append(rows, displayRow{"⊘ binary: " + f.NewPath})
		} else {
			header := f.NewPath
			if f.Status == "renamed" && f.OldPath != "" && f.OldPath != f.NewPath {
				header = f.OldPath + " → " + f.NewPath
			}
			rows = append(rows, displayRow{"─── " + header})
		}

		// rows 1..: hunk headers + diff lines
		for _, hunk := range f.Hunks {
			rows = append(rows, displayRow{hunk.Header})
			for _, dl := range hunk.Lines {
				rows = append(rows, displayRow{dl.Raw})
			}
		}

		// Pad to DisplayRows if the model has more rows than content.
		for len(rows) < dr {
			rows = append(rows, displayRow{""})
		}

		// Emit the rows that overlap [overlapStart, overlapEnd).
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

	return out
}
