package prdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// commentEntry is a single comment or review entry in the Comments section.
type commentEntry struct {
	login         string
	state         string // review state ("APPROVED", "COMMENTED", etc.) or "" for plain comments
	ts            time.Time
	body          string
	path          string // empty for PR-level comments
	line          int    // 0 for PR-level comments
	contextLine   string // the raw diff line text
	isDraft       bool
	isThreadReply bool   // true for replies inside a review thread (not the root comment)
	threadID      string // non-empty for review thread comments; used for threaded replies
	commentID     string // non-empty for PR-level comments; used for comment replies
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

	// Review summaries (APPROVED, CHANGES_REQUESTED, etc.) — built before
	// threads so that when timestamps tie, top-level review comments sort
	// before inline review threads.
	for _, r := range m.Detail.Reviewers {
		if r.Login == "" {
			continue
		}
		state := r.State
		if state == "" {
			state = "COMMENTED"
		}
		// Skip empty COMMENTED summaries — they add no value when inline
		// comments (now in ReviewThreads) are shown separately.
		if r.Body == "" && state == "COMMENTED" {
			continue
		}
		entries = append(entries, commentEntry{
			login: r.Login,
			state: state,
			ts:    r.SubmittedAt,
			body:  r.Body,
		})
	}

	// PR-level comments (issue comments) — also before threads.
	for _, c := range m.Detail.Comments {
		if c.Login == "" {
			continue
		}
		entries = append(entries, commentEntry{
			login:     c.Login,
			state:     "",
			ts:        c.CreatedAt,
			body:      c.Body,
			commentID: c.ID,
		})
	}

	// Review threads (inline review threads with thread IDs) — built after
	// top-level comments so they sort after when timestamps tie.
	for _, thread := range m.Detail.ReviewThreads {
		if thread.ID == "" {
			continue
		}
		for i, c := range thread.Comments {
			if c.Login == "" {
				continue
			}
			entry := commentEntry{
				login:     c.Login,
				ts:        c.CreatedAt,
				body:      c.Body,
				path:      thread.Path,
				line:      thread.Line,
				threadID:  thread.ID,
				commentID: c.ID,
			}
			// First comment is the root; replies are indented in the UI.
			if i > 0 {
				entry.isThreadReply = true
			}
			if i == 0 {
				entry.contextLine = m.lookupDiffLine(thread.Path, thread.Line)
			}
			entries = append(entries, entry)
		}
	}

	// Backward-compatibility fallback: old cached data may have inline comments
	// inside PreviewReviewer.InlineComments instead of ReviewThreads.
	if len(m.Detail.ReviewThreads) == 0 {
		for _, r := range m.Detail.Reviewers {
			if r.Login == "" {
				continue
			}
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
		}
	}

	// Sort non-draft entries chronologically, with each review thread kept
	// as an atomic unit sorted by its earliest comment timestamp. When a review
	// summary and a thread unit were submitted around the same time (within a
	// 5-minute window), the review summary sorts before the thread — this
	// matches how GitHub groups a review body with its inline comments. Drafts
	// stay at the top.
	draftCount := len(m.drafts)
	if draftCount < len(entries) {
		type unit struct {
			entries  []commentEntry
			ts       time.Time
			category int // 0=summary, 1=prComment, 2=thread
		}
		var units []unit
		i := draftCount
		for i < len(entries) {
			if entries[i].threadID != "" {
				j := i + 1
				for j < len(entries) && entries[j].threadID == entries[i].threadID {
					j++
				}
				var key time.Time
				for k := i; k < j; k++ {
					if !entries[k].ts.IsZero() {
						if key.IsZero() || entries[k].ts.Before(key) {
							key = entries[k].ts
						}
					}
				}
				u := make([]commentEntry, j-i)
				copy(u, entries[i:j])
				units = append(units, unit{entries: u, ts: key, category: 2})
				i = j
			} else if entries[i].state != "" {
				units = append(units, unit{entries: []commentEntry{entries[i]}, ts: entries[i].ts, category: 0})
				i++
			} else {
				units = append(units, unit{entries: []commentEntry{entries[i]}, ts: entries[i].ts, category: 1})
				i++
			}
		}
		// Sort primarily by timestamp. Within a 5-minute window, review summaries
		// (category 0) come before PR comments (category 1) come before threads
		// (category 2). This groups a review body with its inline comments while
		// maintaining overall chronological order.
		const window = 5 * time.Minute
		for i := 1; i < len(units); i++ {
			for j := i; j > 0; j-- {
				a, b := units[j-1], units[j]
				aZero, bZero := a.ts.IsZero(), b.ts.IsZero()
				shouldSwap := false
				if aZero && !bZero {
					shouldSwap = true
				} else if !aZero && !bZero {
					if a.ts.After(b.ts) && a.ts.Sub(b.ts) > window {
						// Far apart — strict chronological order.
						shouldSwap = true
					} else if b.ts.Sub(a.ts) > window {
						// Already in order and far apart — no swap.
						break
					} else {
						// Within the same window — break ties by category.
						if a.category > b.category {
							shouldSwap = true
						} else {
							break
						}
					}
				}
				if shouldSwap {
					units[j-1], units[j] = units[j], units[j-1]
				} else {
					break
				}
			}
		}
		pos := draftCount
		for _, u := range units {
			copy(entries[pos:], u.entries)
			pos += len(u.entries)
		}
	}
	m.cachedCommentEntries = entries
	m.commentEntriesDirty = false
	return entries
}

// entryRowCount returns the display-row count for a single comment entry at cw columns.
// Layout: 1 header + (if root with path: 1 blank + 1 path:line + 1 contextLine) +
// (if body: 1 blank + bodyLines) + 1 trailing blank.
// Must exactly mirror what commentLines() generates for each entry.
func (m *PRDetailModel) entryRowCount(e commentEntry, cw int) int {
	rows := 1 // header line
	if !e.isThreadReply && e.path != "" && e.line > 0 {
		rows++ // blank after header
		rows++ // path:line line
		rows++ // context line
	}
	if e.body != "" {
		rows++ // blank before body
		innerW := max(cw-2, 1)
		if e.isThreadReply {
			innerW = max(cw-4, 1) // account for "  " indent inside the box
		}
		if m.mdRenderer != nil {
			rows += len(m.mdRenderer.Render(e.body, innerW))
		} else {
			rows += len(wrapParagraph(e.body, innerW))
		}
	}
	rows++ // trailing blank separator
	return rows
}

// entryRenderHeight returns the total rendered rows this entry contributes in
// the context of its group. Standalone entries and thread roots contribute a
// top border (+1) plus a bottom border (+1) when they are the only or last
// entry. Thread replies contribute no border rows.
func (m *PRDetailModel) entryRenderHeight(e commentEntry, cw int, entries []commentEntry, i int) int {
	h := m.entryRowCount(e, cw)
	isRoot := e.threadID == "" || i == 0 || entries[i-1].threadID != e.threadID
	isLast := e.threadID == "" || i == len(entries)-1 || entries[i+1].threadID != e.threadID
	if isRoot {
		h += 1 // top border
	}
	if isLast {
		h += 1 // bottom border
	}
	return h
}

// threadBounds returns the first and last index within the thread that
// entries[idx] belongs to. If the entry is standalone, first == last == idx.
func threadBounds(entries []commentEntry, idx int) (first, last int) {
	first, last = idx, idx
	if entries[idx].threadID == "" {
		return
	}
	for i := idx - 1; i >= 0; i-- {
		if entries[i].threadID == entries[idx].threadID {
			first = i
		} else {
			break
		}
	}
	for i := idx + 1; i < len(entries); i++ {
		if entries[i].threadID == entries[idx].threadID {
			last = i
		} else {
			break
		}
	}
	return
}

// commentEntryStartRows returns, for each entry, the tab-relative row index
// where its visible area starts. For standalone entries and thread roots this
// is the top border row; for thread replies this is the first content line.
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
	i := 0
	for i < len(entries) {
		e := entries[i]
		if e.threadID != "" {
			j := i + 1
			for j < len(entries) && entries[j].threadID == e.threadID {
				j++
			}
			// Thread group: parent + replies.
			result[i] = cursor
			cursor += 1 // top border
			for k := i + 1; k < j; k++ {
				cursor += m.entryRowCount(entries[k-1], cw)
				result[k] = cursor
			}
			cursor += m.entryRowCount(entries[j-1], cw)
			cursor += 1 // bottom border
			i = j
		} else {
			result[i] = cursor
			cursor += m.entryRowCount(e, cw) + 2 // top + bottom border
			i++
		}
	}
	return result
}

// commentRenderGroup is a contiguous slice of entries rendered as one unit:
// either a single standalone entry (start+1 == end) or a full review thread.
type commentRenderGroup struct {
	start int
	end   int // exclusive
}

func buildCommentRenderGroups(entries []commentEntry) []commentRenderGroup {
	var groups []commentRenderGroup
	i := 0
	for i < len(entries) {
		if entries[i].threadID != "" {
			j := i + 1
			for j < len(entries) && entries[j].threadID == entries[i].threadID {
				j++
			}
			groups = append(groups, commentRenderGroup{start: i, end: j})
			i = j
		} else {
			groups = append(groups, commentRenderGroup{start: i, end: i + 1})
			i++
		}
	}
	return groups
}

// commentLines returns the display lines for the Comments section.
// Review threads are rendered as shared rounded boxes: the root comment appears
// at the full inner width and replies appear with a 2-space indent.
// Active entry headers use Primary color; active groups use Primary border.
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

	innerW := max(cw-2, 1)
	lines := append([]string{}, sectionHeader...)

	for _, g := range buildCommentRenderGroups(entries) {
		if g.end-g.start == 1 {
			// Standalone entry: single box.
			i := g.start
			e := entries[i]
			active := i == activeIdx
			inner := m.buildCommentEntryInner(e, innerW, active)
			bc := m.theme.Border
			if active {
				bc = m.theme.Primary
			}
			borderStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Width(innerW).
				BorderForeground(bc)
			block := borderStyle.Render(strings.Join(inner, "\n"))
			lines = append(lines, strings.Split(block, "\n")...)
			continue
		}

		// Thread group: shared rounded box.
		var allInner []string
		groupActive := false
		for j := g.start; j < g.end; j++ {
			e := entries[j]
			active := j == activeIdx
			if active {
				groupActive = true
			}
			effectiveW := innerW
			if e.isThreadReply {
				effectiveW = max(innerW-2, 1)
			}
			entryInner := m.buildCommentEntryInner(e, effectiveW, active)
			if e.isThreadReply {
				for _, line := range entryInner {
					allInner = append(allInner, "  "+line)
				}
			} else {
				allInner = append(allInner, entryInner...)
			}
		}
		bc := m.theme.Border
		if groupActive {
			bc = m.theme.Primary
		}
		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Width(innerW).
			BorderForeground(bc)
		block := borderStyle.Render(strings.Join(allInner, "\n"))
		lines = append(lines, strings.Split(block, "\n")...)
	}
	lines = append(lines, "")
	return lines
}

// buildCommentEntryInner builds the content lines for a single comment entry
// (header, optional path/context, body, trailing blank) without any border or prefix.
func (m *PRDetailModel) buildCommentEntryInner(e commentEntry, innerW int, active bool) []string {
	var inner []string

	ts := ""
	if !e.ts.IsZero() {
		ts = relativeTime(e.ts)
	}

	var headerText string
	if m.theme != nil {
		style := m.theme.SecondaryTxt
		if active {
			style = m.theme.PrimaryTxt
		}
		headerText = style.Render("@" + e.login)
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

	if active {
		var hint string
		if e.path != "" && e.line > 0 {
			hint = "[Enter | r: Reply]"
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

	inner = append(inner, headerText)

	if !e.isThreadReply && e.path != "" && e.line > 0 {
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
		if m.theme != nil {
			ctxLine = lipgloss.NewStyle().Foreground(m.theme.Muted).Render(ctxLine)
		}
		inner = append(inner, ctxLine)
	}

	if e.body != "" {
		inner = append(inner, "")
		var bodyLines []string
		if m.mdRenderer != nil {
			bodyLines = m.mdRenderer.Render(e.body, innerW)
		} else {
			bodyLines = wrapParagraph(e.body, innerW)
		}
		inner = append(inner, bodyLines...)
	}
	inner = append(inner, "")
	return inner
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

// sortUnits sorts comment units by category first (summaries before PR comments
// before threads), then by timestamp within each category. Zero timestamps sort
// last within their category. Uses stable insertion sort to preserve build order
// among equal-timestamp entries.
type commentUnit struct {
	entries  []commentEntry
	ts       time.Time
	category int
}
