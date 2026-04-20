package prdetail

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
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

	// ── Comments ─────────────────────────────────────────────────────────────
	// Always present when detail is loaded (shows "No reviews" when empty).
	// Omitted only while detail is still loading.
	cLines := m.commentLines(contentWidth)
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

// maxDiffDisplayRows is the cap on rendered diff rows before a truncation banner is shown.
const maxDiffDisplayRows = 5000

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
	rows := 3 // blank + separator + header bar
	if f.IsBinary {
		return rows + 1 // +1 for the "📄 Binary file (no diff available)" placeholder row
	}
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

	// Divider rule — muted color
	divider := strings.Repeat("─", cw)
	if m.theme != nil {
		divider = m.theme.MutedTxt.Render(divider)
	}
	lines = append(lines, divider)

	// Markdown-rendered body
	if m.mdRenderer != nil {
		lines = append(lines, m.mdRenderer.Render(body, cw)...)
	} else {
		lines = append(lines, wrapParagraph(body, cw)...)
	}
	return lines
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

// commentLines returns the display lines for the Comments section.
// Renders formal reviews (reviews field) and regular PR comments (comments field)
// interleaved chronologically. Each item renders as:
//
//	header line (@login · state · date) + blank + wrapped body + blank separator
//
// Reviews with no body still appear (approval-only reviews).
// Returns nil when detail is not loaded.
// Returns ["No reviews"] placeholder when detail is loaded but nothing to show.
func (m *PRDetailModel) commentLines(contentWidth ...int) []string {
	if m.Detail == nil {
		return nil
	}
	cw := 80
	if len(contentWidth) > 0 && contentWidth[0] > 0 {
		cw = contentWidth[0]
	}

	type entry struct {
		login string
		state string // for reviews; empty for plain comments
		ts    time.Time
		body  string
	}

	var entries []entry

	for _, r := range m.Detail.Reviewers {
		if r.Login == "" {
			continue
		}
		state := r.State
		if state == "" {
			state = "COMMENTED"
		}
		entries = append(entries, entry{
			login: r.Login,
			state: state,
			ts:    r.SubmittedAt,
			body:  r.Body,
		})
	}

	for _, c := range m.Detail.Comments {
		if c.Login == "" {
			continue
		}
		entries = append(entries, entry{
			login: c.Login,
			state: "",
			ts:    c.CreatedAt,
			body:  c.Body,
		})
	}

	// Sort chronologically by timestamp (zero times go last).
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			aZero, bZero := a.ts.IsZero(), b.ts.IsZero()
			if aZero && !bZero {
				// a has no timestamp → move to end
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else if !aZero && !bZero && a.ts.After(b.ts) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}

	// Section header: blank padding + dashed separator + bold label.
	var sectionHeader []string
	sectionHeader = append(sectionHeader, "") // blank padding before separator
	sep := strings.Repeat("╌", cw)
	label := "Comments"
	if m.theme != nil {
		sep = m.theme.MutedTxt.Render(sep)
		label = m.theme.MutedTxt.Bold(true).Render(label)
	}
	sectionHeader = append(sectionHeader, sep)
	sectionHeader = append(sectionHeader, label)

	if len(entries) == 0 {
		msg := "No reviews"
		if m.theme != nil {
			msg = m.theme.MutedTxt.Render(msg)
		}
		return append(sectionHeader, msg)
	}

	lines := append([]string{}, sectionHeader...)
	for _, e := range entries {
		// Format timestamp.
		ts := ""
		if !e.ts.IsZero() {
			if e.ts.Year() == time.Now().Year() {
				ts = e.ts.Format("Jan 02")
			} else {
				ts = e.ts.Format("Jan 02 2006")
			}
		}

		// Header line: "@login · STATE · date" (state omitted for plain comments)
		var header string
		if m.theme != nil {
			header = m.theme.SecondaryTxt.Render("@" + e.login)
			if e.state != "" {
				header += m.theme.MutedTxt.Render(" · " + e.state)
			}
			if ts != "" {
				header += m.theme.MutedTxt.Render(" · " + ts)
			}
		} else {
			header = "@" + e.login
			if e.state != "" {
				header += " · " + e.state
			}
			if ts != "" {
				header += " · " + ts
			}
		}
		lines = append(lines, header)

		if e.body != "" {
			lines = append(lines, "") // blank after header
			lines = append(lines, wrapParagraph(e.body, cw)...)
		}

		// Blank separator between blocks.
		lines = append(lines, "")
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
	cLines := m.commentLines(contentWidth)

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
			Foreground(lipgloss.Color("#E2E8F0")).
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
			for _, hunk := range f.Hunks {
				rows = append(rows, displayRow{hunkHeaderStyle.Render(hunk.Header)})
				for _, dl := range hunk.Lines {
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
