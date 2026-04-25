package prdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
)

// buildInlineBody formats a slice of inline comments into a markdown string
// used as the body of a review entry when the reviewer left no summary text.
func buildInlineBody(comments []domain.PreviewInlineComment) string {
	parts := make([]string, 0, len(comments))
	for _, c := range comments {
		if c.Body == "" {
			continue
		}
		loc := c.Path
		if c.Line > 0 {
			loc = fmt.Sprintf("%s:%d", c.Path, c.Line)
		}
		parts = append(parts, fmt.Sprintf("**%s**\n%s", loc, c.Body))
	}
	return strings.Join(parts, "\n\n")
}

func relativeTime(t time.Time) string {
	age := time.Since(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	case age < 3*24*time.Hour:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	default:
		if t.Year() == time.Now().Year() {
			return t.Format("Jan 02")
		}
		return t.Format("Jan 02 2006")
	}
}

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
	// Row count for sections uses -1 (no active entry) so it stays stable regardless
	// of cursor position — active highlight doesn't change the row count.
	cLines := m.commentLines(contentWidth, -1)
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
const maxDiffDisplayRows = 20000

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

// commentEntry is a single comment or review entry in the Comments section.
type commentEntry struct {
	login       string
	state       string // review state ("APPROVED", "COMMENTED", etc.) or "" for plain comments
	ts          time.Time
	body        string
	path        string  // empty for PR-level comments
	line        int     // 0 for PR-level comments
	contextLine string  // the raw diff line text
	isDraft     bool
}

// commentEntries returns the sorted slice of comment/review entries for the current PR.
// Draft entries appear first, followed by real entries sorted chronologically.
// Returns nil when detail is not loaded.
func (m *PRDetailModel) commentEntries() []commentEntry {
	if m.Detail == nil {
		return nil
	}
	var entries []commentEntry

	// Drafts first.
	for _, d := range m.drafts {
		entries = append(entries, commentEntry{
			login:       "[DRAFT]",
			ts:          d.CreatedAt,
			body:        d.Body,
			path:        d.Path,
			line:        d.Line,
			contextLine: d.ContextLine,
			isDraft:     true,
		})
	}

	// Real inline comments from reviewers.
	for _, r := range m.Detail.Reviewers {
		if r.Login == "" {
			continue
		}
		// Add inline comments as individual entries.
		for _, ic := range r.InlineComments {
			entries = append(entries, commentEntry{
				login:       r.Login,
				state:       r.State,
				ts:          r.SubmittedAt,
				body:        ic.Body,
				path:        ic.Path,
				line:        ic.Line,
				contextLine: m.lookupDiffLine(ic.Path, ic.Line),
			})
		}
		// Add the review summary entry only if it has a body or no inline comments.
		state := r.State
		if state == "" {
			state = "COMMENTED"
		}
		body := r.Body
		if body == "" && len(r.InlineComments) > 0 {
			continue // skip empty summary when inline comments exist
		}
		entries = append(entries, commentEntry{
			login: r.Login,
			state: state,
			ts:    r.SubmittedAt,
			body:  body,
		})
	}
	for _, c := range m.Detail.Comments {
		if c.Login == "" {
			continue
		}
		entries = append(entries, commentEntry{
			login: c.Login,
			state: "",
			ts:    c.CreatedAt,
			body:  c.Body,
		})
	}

	// Sort non-draft entries by timestamp: zero times go last.
	// Drafts stay at the top (indices 0..draftCount-1).
	draftCount := len(m.drafts)
	for i := draftCount + 1; i < len(entries); i++ {
		for j := i; j > draftCount; j-- {
			a, b := entries[j-1], entries[j]
			aZero, bZero := a.ts.IsZero(), b.ts.IsZero()
			if aZero && !bZero {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else if !aZero && !bZero && a.ts.After(b.ts) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}
	return entries
}

// entryRowCount returns the display-row count for a single comment entry at cw columns.
// Layout: 1 header + (if path: 1 blank + 1 path:line + 1 contextLine) + (if body: 1 blank + bodyLines) + 1 trailing blank.
// Body wraps at innerW = cw-2 to match the width inside the rounded border in commentLines.
// Must exactly mirror what commentLines() generates for each entry.
func (m *PRDetailModel) entryRowCount(e commentEntry, cw int) int {
	rows := 1 // header line
	if e.path != "" && e.line > 0 {
		rows++ // blank after header
		rows++ // path:line line
		rows++ // context line
	}
	if e.body != "" {
		rows++ // blank before body
		innerW := max(cw-2, 1)
		if m.mdRenderer != nil {
			rows += len(m.mdRenderer.Render(e.body, innerW))
		} else {
			rows += len(wrapParagraph(e.body, innerW))
		}
	}
	rows++ // trailing blank separator
	return rows
}

// commentEntryStartRows returns, for each entry, the absolute content-row index
// where its border-top line appears. Every entry is always rendered with a
// rounded border, so heights are constant regardless of which entry is active.
// The section header occupies 3 rows before the first entry.
// Returns nil when there are no entries.
func (m *PRDetailModel) commentEntryStartRows(contentWidth int) []int {
	entries := m.commentEntries()
	if len(entries) == 0 {
		return nil
	}
	cw := max(contentWidth, 1)
	result := make([]int, len(entries))
	cursor := 3 // section header rows: blank + separator + "Comments" label
	for i, e := range entries {
		result[i] = cursor
		cursor += m.entryRowCount(e, cw) + 2 // +2 for top + bottom border (always present)
	}
	return result
}

// commentLines returns the display lines for the Comments section.
// Every entry is rendered inside a rounded border; the active entry's border
// uses the Primary color, inactive entries use the muted Border color.
// The active entry also shows a right-aligned "[r: Reply]" hint in its header.
// Returns nil when detail is not loaded.
func (m *PRDetailModel) commentLines(contentWidth int, activeIdx int) []string {
	if m.Detail == nil {
		return nil
	}
	cw := max(contentWidth, 1)

	entries := m.commentEntries()

	// Section header: blank + separator + label.
	var sectionHeader []string
	sectionHeader = append(sectionHeader, "")
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

	// innerW is the content width inside the border (1 char left + 1 char right).
	innerW := max(cw-2, 1)

	lines := append([]string{}, sectionHeader...)
	for i, e := range entries {
		active := i == activeIdx

		// Format timestamp.
		ts := ""
		if !e.ts.IsZero() {
			ts = relativeTime(e.ts)
		}

		// Build header text.
		var headerText string
		if m.theme != nil {
			headerText = m.theme.SecondaryTxt.Render("@" + e.login)
			if e.state != "" {
				headerText += m.theme.MutedTxt.Render(" · " + e.state)
			}
			if ts != "" {
				headerText += m.theme.MutedTxt.Render(" · " + ts)
			}
		} else {
			headerText = "@" + e.login
			if e.state != "" {
				headerText += " · " + e.state
			}
			if ts != "" {
				headerText += " · " + ts
			}
		}

		// Active entry gets a right-aligned hint in the header.
		if active {
			var hint string
			if e.path != "" && e.line > 0 {
				hint = "[Enter]"
			} else if !e.isDraft {
				hint = "[r: Reply]"
			}
			if hint != "" {
				if m.theme != nil {
					hint = m.theme.MutedTxt.Render(hint)
				}
				pad := innerW - lipgloss.Width(headerText) - lipgloss.Width(hint)
				if pad > 0 {
					headerText += strings.Repeat(" ", pad) + hint
				} else {
					headerText += " " + hint
				}
			}
		}

		// Collect inner lines (same structure for active and inactive).
		var inner []string
		inner = append(inner, headerText)
		if e.path != "" && e.line > 0 {
			inner = append(inner, "")
			loc := fmt.Sprintf("%s:%d", e.path, e.line)
			if m.theme != nil {
				loc = m.theme.MutedTxt.Render(loc)
			}
			inner = append(inner, loc)
			ctxLine := e.contextLine
			if ctxLine == "" {
				ctxLine = " "
			}
			// Render context line in a subtle/monospace style.
			if m.theme != nil {
				ctxLine = lipgloss.NewStyle().Foreground(m.theme.Muted).Render(ctxLine)
			}
			inner = append(inner, ctxLine)
		}
		if e.body != "" {
			inner = append(inner, "")
			bodyW := innerW
			var bodyLines []string
			if m.mdRenderer != nil {
				bodyLines = m.mdRenderer.Render(e.body, bodyW)
			} else {
				bodyLines = wrapParagraph(e.body, bodyW)
			}
			inner = append(inner, bodyLines...)
		}
		inner = append(inner, "") // trailing blank

		// Border color: primary for active, muted for inactive.
		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Width(innerW)
		if m.theme != nil {
			bc := m.theme.Border
			if active {
				bc = m.theme.Primary
			}
			borderStyle = borderStyle.BorderForeground(bc)
		}
		block := borderStyle.Render(strings.Join(inner, "\n"))
		lines = append(lines, strings.Split(block, "\n")...)
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
	cLines := m.commentLines(contentWidth, m.commentCursor)

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

// renderDescriptionTab renders the Description tab content at the given scroll
// and viewport dimensions. Returns exactly contentH lines (blank-padded).
func (m *PRDetailModel) renderDescriptionTab(scroll, contentH, contentWidth int) []string {
	lines := m.descriptionLines(contentWidth)
	blank := strings.Repeat(" ", max(contentWidth, 0))
	out := make([]string, contentH)
	for i := range contentH {
		idx := scroll + i
		if idx >= 0 && idx < len(lines) {
			out[i] = lines[idx]
		} else {
			out[i] = blank
		}
	}
	return out
}

// renderDiffTab renders the Diff tab content at the given scroll and viewport
// dimensions. Returns exactly contentH lines (blank-padded).
func (m *PRDetailModel) renderDiffTab(scroll, contentH, contentWidth int) []string {
	localStart := scroll
	localEnd := scroll + contentH
	return m.renderDiffSectionLines(localStart, localEnd, contentWidth)
}

// renderCommentsTab renders the Comments tab content at the given scroll and
// viewport dimensions. Returns exactly contentH lines (blank-padded).
func (m *PRDetailModel) renderCommentsTab(scroll, contentH, contentWidth int) []string {
	lines := m.commentLines(contentWidth, m.commentCursor)
	blank := strings.Repeat(" ", max(contentWidth, 0))
	out := make([]string, contentH)
	for i := range contentH {
		idx := scroll + i
		if idx >= 0 && idx < len(lines) {
			out[i] = lines[idx]
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

	// Build a set of hunk line indices covered by drafts so the indicator
	// highlights the full selected range, even when it spans both LEFT
	// (deletion) and RIGHT (addition) sides.
	type hunkLineKey struct{ fileIdx, hunkIdx, lineIdx int }
	draftCovered := make(map[hunkLineKey]bool)
	for _, d := range m.drafts {
		for fi, f := range m.Diff.Files {
			if f.NewPath != d.Path && f.OldPath != d.Path {
				continue
			}
			for hi, h := range f.Hunks {
				startLI, endLI := -1, -1
				for li, dl := range h.Lines {
					for _, a := range dl.Anchors {
						if a.Path != d.Path || a.Line == nil {
							continue
						}
						if d.StartLine > 0 && a.Side == d.StartSide && *a.Line == d.StartLine {
							startLI = li
						}
						if a.Side == d.Side && *a.Line == d.Line {
							endLI = li
						}
					}
				}
				if endLI >= 0 {
					if startLI < 0 {
						startLI = endLI
					}
					for li := startLI; li <= endLI; li++ {
						draftCovered[hunkLineKey{fi, hi, li}] = true
					}
				}
			}
		}
	}

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

					// Apply visual selection highlight or draft indicator.
					isSelected := m.visual.Active && m.visual.FileIdx == i && m.visual.HunkIdx == hi &&
						li >= m.visual.StartLine && li <= m.visual.EndLine
					isDrafted := !isSelected && draftCovered[hunkLineKey{i, hi, li}]

					if isSelected {
						if m.theme != nil {
							s = m.theme.ListSelected.Width(cw).Render(s)
						} else {
							s = lipgloss.NewStyle().Reverse(true).Width(cw).Render(s)
						}
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
