package prdetail

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/ui/theme"
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
	isResolved           bool   // true for entries in a resolved thread
	isResolvedSummary    bool   // true for the collapsed summary row of a resolved thread
	isThreadStart        bool   // true for the first comment of a thread (root)
	resolverLogin        string // who resolved the thread (empty if unresolved)
	replyCount           int    // number of comments in the thread (for summary)
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
		// Resolved + collapsed → emit a single summary entry.
		if thread.IsResolved && !m.isThreadExpanded(thread.ID, true) {
			summary := commentEntry{
				threadID:          thread.ID,
				path:              thread.Path,
				line:              thread.Line,
				isResolved:        true,
				isResolvedSummary: true,
				resolverLogin:     thread.ResolvedBy,
				replyCount:        len(thread.Comments),
			}
			if len(thread.Comments) > 0 {
				summary.ts = thread.Comments[0].CreatedAt
			}
			entries = append(entries, summary)
			continue
		}
		for i, c := range thread.Comments {
			if c.Login == "" {
				continue
			}
			entry := commentEntry{
				login:         c.Login,
				ts:            c.CreatedAt,
				body:          c.Body,
				path:          thread.Path,
				line:          thread.Line,
				threadID:      thread.ID,
				commentID:     c.ID,
				isResolved:    thread.IsResolved,
				resolverLogin: thread.ResolvedBy,
			}
			if i > 0 {
				entry.isThreadReply = true
			}
			if i == 0 {
				entry.isThreadStart = true
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
		floatBefore := map[int]int{}
		for _, ins := range insertions {
			floatBefore[ins.insertBeforeIdx] = ins.reviewIdx
		}
		placed := map[int]bool{}
		var orderedUnits []unit
		for i := range units {
			if placed[i] {
				continue
			}
			if revIdx, ok := floatBefore[i]; ok && !placed[revIdx] {
				orderedUnits = append(orderedUnits, units[revIdx])
				placed[revIdx] = true
			}
			orderedUnits = append(orderedUnits, units[i])
			placed[i] = true
		}
		units = orderedUnits
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
	// Resolved summary entries render as 1-2 wrapped lines + trailing blank.
	if e.isResolvedSummary {
		innerW := max(cw-2, 1)
		if e.indentByParentReview {
			innerW = max(innerW-2, 1) // narrower box under parent review, as commentLines renders it
		}
		summaryText := m.buildResolvedSummaryText(e)
		rows := len(wrapParagraph(summaryText, innerW))
		rows++ // trailing blank
		return rows
	}

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
		rows += len(m.commentBodyLines(e.body, innerW))
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

	// Section header (3 rows): blank + summary line + blank.
	sectionHeader := []string{"", m.commentsSummaryLine(entries), ""}

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
			if e.threadID != "" && !e.isResolved {
				bc = m.theme.UnresolvedBorder // open threads still need attention
			}
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
		if !entries[g.start].isResolved {
			bc = m.theme.UnresolvedBorder // open threads still need attention
		}
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

// commentsSummaryLine renders "3 comments · 2 unresolved" for the section header.
// Only entries with text count as comments: a bare approval is a review, not a
// comment, and a collapsed resolved thread stands for all of its comments.
func (m *PRDetailModel) commentsSummaryLine(entries []commentEntry) string {
	n := 0
	for _, e := range entries {
		switch {
		case e.isDraft:
		case e.isResolvedSummary:
			n += e.replyCount
		case e.body != "":
			n++
		}
	}
	noun := "comments"
	if n == 1 {
		noun = "comment"
	}
	if m.theme == nil {
		return fmt.Sprintf("%d %s", n, noun)
	}
	th := m.theme
	bright := lipgloss.NewStyle().Foreground(th.Text).Bold(true)
	line := bright.Render(fmt.Sprint(n)) + th.MutedTxt.Render(" "+noun)
	if u := m.unresolvedThreadCount(); u > 0 {
		line += th.FaintTxt.Render("  ·  ") +
			th.ReviewRequired.Render(fmt.Sprintf("● %d unresolved", u))
	}
	if d := len(m.drafts); d > 0 {
		label := "pending drafts"
		if d == 1 {
			label = "pending draft"
		}
		line += th.FaintTxt.Render("  ·  ") + th.CIPending.Render(fmt.Sprintf("%d %s", d, label))
	}
	return line
}

// avatarColor picks a stable colour for login from the theme's avatar palette.
func avatarColor(login string, palette []lipgloss.Color) lipgloss.Color {
	if len(palette) == 0 {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(login))
	return palette[h.Sum32()%uint32(len(palette))]
}

// reviewVerb renders the action an entry represents ("approved", "commented", ...).
func (m *PRDetailModel) reviewVerb(e commentEntry) string {
	th := m.theme
	switch {
	case e.isDraft:
		return th.CIPending.Render("pending draft")
	case e.state == "APPROVED":
		return th.ReviewApproved.Render("✓ approved")
	case e.state == "CHANGES_REQUESTED":
		return th.ReviewChanges.Render("✗ requested changes")
	case e.state == "DISMISSED":
		return th.MutedTxt.Render("dismissed review")
	case e.state != "":
		return th.MutedTxt.Render("reviewed")
	case e.isThreadReply:
		return th.MutedTxt.Render("replied")
	default:
		return th.MutedTxt.Render("commented")
	}
}

// buildCommentEntryInner builds the content lines for a single comment entry
// (header, optional path/context, body, trailing blank) without any border or prefix.
// Every line is kept within innerW so the enclosing box never re-wraps it;
// the row count must match entryRowCount exactly.
func (m *PRDetailModel) buildCommentEntryInner(e commentEntry, innerW int, active bool) []string {
	// Resolved collapsed summary: one-line entry.
	if e.isResolvedSummary {
		return m.buildResolvedSummaryInner(e, innerW, active)
	}

	var inner []string

	ts := ""
	if !e.ts.IsZero() {
		ts = relativeTime(e.ts)
	}

	var headerText string
	if m.theme != nil {
		th := m.theme
		faint := th.FaintTxt
		if e.isDraft {
			headerText = th.CIPending.Render("◌ ") + lipgloss.NewStyle().Bold(true).Foreground(th.Text).Render("Draft")
		} else {
			nameStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Text)
			if active {
				nameStyle = nameStyle.Foreground(th.TextBright)
			}
			headerText = lipgloss.NewStyle().Foreground(avatarColor(e.login, th.AvatarPalette)).Render("●") + " " + nameStyle.Render(e.login)
		}
		headerText += "  " + m.reviewVerb(e)
		// Optional parts are added only while they fit, so narrow cards drop
		// them whole instead of cutting them mid-word.
		var optional []string
		if ts != "" {
			optional = append(optional, faint.Render(ts))
		}
		if e.isResolved && e.isThreadStart {
			badge := "✓ resolved"
			if e.resolverLogin != "" {
				badge += " by " + e.resolverLogin
			}
			optional = append(optional, th.ReviewApproved.Render(badge))
		}
		for _, opt := range optional {
			if next := headerText + faint.Render(" · ") + opt; lipgloss.Width(next) <= innerW {
				headerText = next
			}
		}
	} else {
		headerText = "@" + e.login
		if e.state != "" {
			headerText += " · " + e.state
		}
		if ts != "" {
			headerText += " · " + ts
		}
		if e.isResolved && e.isThreadStart {
			headerText += " · Resolved ✓"
		}
	}

	if active {
		if hint := m.buildEntryHint(e); hint != "" {
			if m.theme != nil {
				hint = m.theme.RenderHints(hint)
			}
			if pad := innerW - lipgloss.Width(headerText) - lipgloss.Width(hint); pad >= 2 {
				headerText += strings.Repeat(" ", pad) + hint
			}
		}
	}

	inner = append(inner, truncateText(headerText, innerW))

	if !e.isThreadReply && e.path != "" && e.line > 0 {
		inner = append(inner, "")
		inner = append(inner, truncateText(m.renderCommentLocation(e.path, e.line), innerW))
		inner = append(inner, m.renderCommentContext(e.contextLine, e.line, innerW))
	}

	if e.body != "" {
		inner = append(inner, "")
		inner = append(inner, m.commentBodyLines(e.body, innerW)...)
	}
	inner = append(inner, "")
	return inner
}

// commentBodyLines renders a comment body to lines no wider than w. Words
// longer than w (long URLs, paths) come back from the renderers as over-wide
// lines; those are hard-wrapped here exactly as the card border would wrap
// them, so entryRowCount and buildCommentEntryInner always agree on height.
func (m *PRDetailModel) commentBodyLines(body string, w int) []string {
	var lines []string
	if m.mdRenderer != nil {
		lines = m.mdRenderer.Render(body, w)
	} else {
		lines = wrapParagraph(body, w)
	}
	out := lines[:0:0]
	for _, l := range lines {
		if lipgloss.Width(l) <= w {
			out = append(out, l)
			continue
		}
		out = append(out, strings.Split(lipgloss.NewStyle().Width(w).Render(l), "\n")...)
	}
	return out
}

// renderCommentLocation renders "dir/" dimmed + "file.go" + ":40".
func (m *PRDetailModel) renderCommentLocation(path string, line int) string {
	if m.theme == nil {
		return fmt.Sprintf("%s:%d", path, line)
	}
	th := m.theme
	dir, base := "", path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		dir, base = path[:i+1], path[i+1:]
	}
	return th.MutedTxt.Render(dir) +
		lipgloss.NewStyle().Foreground(th.Text).Render(base) +
		th.FaintTxt.Render(":") +
		lipgloss.NewStyle().Foreground(th.Secondary).Render(fmt.Sprint(line))
}

// renderCommentContext renders the commented diff line as a one-line code
// snippet: line-number gutter + diff-tinted code, exactly one row.
func (m *PRDetailModel) renderCommentContext(raw string, line, innerW int) string {
	if raw == "" {
		raw = " "
	}
	if m.theme == nil {
		return truncateText(raw, innerW)
	}
	th := m.theme
	gutter := th.FaintTxt.Render(fmt.Sprintf("%4d │ ", line))
	codeW := max(innerW-lipgloss.Width(gutter), 1)
	switch {
	case strings.HasPrefix(raw, "+"):
		return gutter + theme.FillBg(th.DiffAddBg, codeW, th.DiffAddition.Render(raw))
	case strings.HasPrefix(raw, "-"):
		return gutter + theme.FillBg(th.DiffDelBg, codeW, th.DiffDeletion.Render(raw))
	default:
		return gutter + truncateText(th.DimTxt.Render(raw), codeW)
	}
}

// buildEntryHint returns the active-entry hint string for the given entry,
// or "" if no hint applies. The m: Resolve/Unresolve portion is gated at
// m.Width >= 60 to protect the header layout on narrow terminals.
func (m *PRDetailModel) buildEntryHint(e commentEntry) string {
	var parts []string

	if e.isResolvedSummary {
		parts = append(parts, "enter: expand")
		parts = append(parts, "r: reply")
	} else if e.path != "" && e.line > 0 {
		parts = append(parts, "enter: go to line")
		parts = append(parts, "r: reply")
	} else if !e.isDraft {
		parts = append(parts, "r: reply")
	}

	// The m: Resolve/Unresolve portion only for thread entries.
	if e.threadID != "" && m.Width >= 60 {
		if e.isResolved {
			parts = append(parts, "m: unresolve")
		} else {
			parts = append(parts, "m: resolve")
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// buildResolvedSummaryText returns the plain-text summary string for a collapsed
// resolved thread (without ANSI styling or hint), used for row-count computation.
func (m *PRDetailModel) buildResolvedSummaryText(e commentEntry) string {
	parts := []string{"✓ Resolved"}
	if e.path != "" && e.line > 0 {
		parts[0] += fmt.Sprintf(" thread on %s:%d", e.path, e.line)
	}
	noun := "comments"
	if e.replyCount == 1 {
		noun = "comment"
	}
	parts = append(parts, fmt.Sprintf("%d %s", e.replyCount, noun))
	if e.resolverLogin != "" {
		parts = append(parts, "by "+e.resolverLogin)
	}
	return strings.Join(parts, " · ")
}

// buildResolvedSummaryInner renders a collapsed resolved thread as a muted
// summary ("✓ Resolved thread on path:line · N comments · by x"), wrapped
// exactly as entryRowCount expects, plus a trailing blank.
func (m *PRDetailModel) buildResolvedSummaryInner(e commentEntry, innerW int, active bool) []string {
	wrapped := wrapParagraph(m.buildResolvedSummaryText(e), innerW)
	out := make([]string, 0, len(wrapped)+1)
	for i, l := range wrapped {
		// A single word longer than the card (a long path) must be cut, not
		// left for the border to re-wrap, or the row count would drift.
		l = truncateText(l, innerW)
		if m.theme != nil {
			style := m.theme.MutedTxt
			if active {
				style = m.theme.DimTxt
			}
			if i == 0 && strings.HasPrefix(l, "✓") {
				l = m.theme.ReviewApproved.Render("✓") + style.Render(strings.TrimPrefix(l, "✓"))
			} else {
				l = style.Render(l)
			}
		}
		out = append(out, l)
	}
	if active && len(out) == 1 {
		if hint := m.buildEntryHint(e); hint != "" {
			if m.theme != nil {
				hint = m.theme.RenderHints(hint)
			}
			if pad := innerW - lipgloss.Width(out[0]) - lipgloss.Width(hint); pad >= 2 {
				out[0] += strings.Repeat(" ", pad) + hint
			}
		}
	}
	return append(out, "")
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
