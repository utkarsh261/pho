package prdetail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// commentEntry is a single comment or review entry in the Comments section.
type commentEntry struct {
	login                string
	state                string // review state ("APPROVED", "COMMENTED", etc.) or "" for plain comments
	ts                   time.Time
	body                 string
	path                 string // empty for PR-level comments
	line                 int    // 0 for PR-level comments
	contextLine          string // the raw diff line text
	isDraft              bool
	isThreadReply        bool   // true for replies inside a review thread (not the root comment)
	threadID             string // non-empty for review thread comments; used for threaded replies
	commentID            string // non-empty for PR-level comments; used for comment replies
	indentByParentReview bool   // true when this thread belongs to a parent review summary
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
	// as an atomic unit sorted by its earliest comment timestamp. Review
	// summaries are floated just before the first thread they are associated
	// with (submitted within 5 minutes after the thread's earliest comment);
	// all other items remain in strict timestamp order. Drafts stay at the top.
	draftCount := len(m.drafts)
	if draftCount < len(entries) {
		type unit struct {
			entries []commentEntry
			ts      time.Time
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
				units = append(units, unit{entries: u, ts: key})
				i = j
			} else {
				units = append(units, unit{entries: []commentEntry{entries[i]}, ts: entries[i].ts})
				i++
			}
		}
		// Strict chronological sort by timestamp. Zero timestamps sort last.
		sort.SliceStable(units, func(i, j int) bool {
			aZero, bZero := units[i].ts.IsZero(), units[j].ts.IsZero()
			if aZero && !bZero {
				return false
			}
			if !aZero && bZero {
				return true
			}
			if !aZero && !bZero {
				return units[i].ts.Before(units[j].ts)
			}
			return false
		})
		// Float review summaries before their associated threads. A review
		// summary is associated with a thread if it was submitted within 5
		// minutes after the thread's earliest comment. Each review summary is
		// inserted before the first (earliest) thread it's associated with.
		const window = 5 * time.Minute
		type reviewInfo struct {
			idx int
			ts  time.Time
		}
		var reviews []reviewInfo
		for i, u := range units {
			if len(u.entries) == 1 && u.entries[0].state != "" && u.entries[0].threadID == "" {
				reviews = append(reviews, reviewInfo{idx: i, ts: u.ts})
			}
		}
		type insertion struct {
			reviewIdx       int
			insertBeforeIdx int
		}
		assigned := map[int]bool{}
		var insertions []insertion
		for ti, u := range units {
			if len(u.entries) > 0 && u.entries[0].threadID != "" {
				earliest := u.ts
				for _, r := range reviews {
					if assigned[r.idx] {
						continue
					}
					if !r.ts.IsZero() && !earliest.IsZero() && r.ts.After(earliest) && r.ts.Sub(earliest) <= window {
						if r.idx < ti {
							continue
						}
						insertions = append(insertions, insertion{reviewIdx: r.idx, insertBeforeIdx: ti})
						assigned[r.idx] = true
						break
					}
				}
			}
		}
		// Apply insertions in reverse order to preserve indices.
		sort.Slice(insertions, func(i, j int) bool {
			return insertions[i].reviewIdx > insertions[j].reviewIdx
		})
		for _, ins := range insertions {
			reviewUnit := units[ins.reviewIdx]
			units = append(units[:ins.reviewIdx], units[ins.reviewIdx+1:]...)
			insertAt := ins.insertBeforeIdx
			if ins.reviewIdx < ins.insertBeforeIdx {
				insertAt--
			}
			units = append(units[:insertAt], append([]unit{reviewUnit}, units[insertAt:]...)...)
		}
		pos := draftCount
		for _, u := range units {
			copy(entries[pos:], u.entries)
			pos += len(u.entries)
		}
	}
	// Mark thread groups whose earliest comment falls within 5 minutes before
	// a review summary. These threads are visually indented under their parent.
	const indentWindow = 5 * time.Minute
	type reviewInfo struct {
		ts time.Time
	}
	var reviews []reviewInfo
	for _, e := range entries[draftCount:] {
		if e.state != "" && e.threadID == "" {
			reviews = append(reviews, reviewInfo{ts: e.ts})
		}
	}
	i := draftCount
	for i < len(entries) {
		if entries[i].threadID != "" {
			tid := entries[i].threadID
			j := i + 1
			for j < len(entries) && entries[j].threadID == tid {
				j++
			}
			var earliest time.Time
			for k := i; k < j; k++ {
				if !entries[k].ts.IsZero() && (earliest.IsZero() || entries[k].ts.Before(earliest)) {
					earliest = entries[k].ts
				}
			}
			if !earliest.IsZero() {
				for _, rev := range reviews {
					if !rev.ts.IsZero() && rev.ts.After(earliest) && rev.ts.Sub(earliest) <= indentWindow {
						for k := i; k < j; k++ {
							entries[k].indentByParentReview = true
						}
						break
					}
				}
			}
			i = j
		} else {
			i++
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
		if e.indentByParentReview {
			innerW = max(innerW-2, 1) // narrower box under parent review
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
			indented := e.indentByParentReview
			entryInnerW := innerW
			if indented {
				entryInnerW = max(innerW-2, 1)
			}
			inner := m.buildCommentEntryInner(e, entryInnerW, active)
			bc := m.theme.Border
			if active {
				bc = m.theme.Primary
			}
			borderStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Width(entryInnerW).
				BorderForeground(bc)
			block := borderStyle.Render(strings.Join(inner, "\n"))
			blockLines := strings.Split(block, "\n")
			if indented {
				for i := range blockLines {
					blockLines[i] = "  " + blockLines[i]
				}
			}
			lines = append(lines, blockLines...)
			continue
		}

		// Thread group: shared rounded box.
		indented := entries[g.start].indentByParentReview
		boxW := innerW
		if indented {
			boxW = max(innerW-2, 1)
		}
		var allInner []string
		groupActive := false
		for j := g.start; j < g.end; j++ {
			e := entries[j]
			active := j == activeIdx
			if active {
				groupActive = true
			}
			effectiveW := boxW
			if e.isThreadReply {
				effectiveW = max(boxW-2, 1)
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
			Width(boxW).
			BorderForeground(bc)
		block := borderStyle.Render(strings.Join(allInner, "\n"))
		blockLines := strings.Split(block, "\n")
		if indented {
			for i := range blockLines {
				blockLines[i] = "  " + blockLines[i]
			}
		}
		lines = append(lines, blockLines...)
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
