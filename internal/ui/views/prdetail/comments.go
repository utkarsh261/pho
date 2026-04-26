package prdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

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
	if !m.commentEntriesDirty && m.cachedCommentEntries != nil {
		return m.cachedCommentEntries
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
	m.cachedCommentEntries = entries
	m.commentEntriesDirty = false
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

// commentEntryStartRows returns, for each entry, the tab-relative row index
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
	// Pad with a trailing blank line so the last comment's bottom border
	// doesn't sit flush against the viewport edge.
	lines = append(lines, "")
	return lines
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
