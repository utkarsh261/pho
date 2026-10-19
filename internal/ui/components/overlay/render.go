package overlay

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const (
	maxBoxWidth = 80
	// themeBoxWidth fits a theme row (edge, name column, swatch, "current"
	// tag) and the border: the picker's content never gets wider.
	themeBoxWidth = 48
	// boxChrome is the rows around the result list: top border, query,
	// divider and bottom border.
	boxChrome = 4
)

// styles are the palette's styles, all plain when no theme is set.
type styles struct {
	border, title, faint, dim, prompt, cursor lipgloss.Style
	text, textSel, match, num, edge, name     lipgloss.Style
	open, merged, closed, draft               lipgloss.Style
	selBg                                     lipgloss.Color
	th                                        *theme.Theme
}

func (m Model) styles() styles {
	th := m.theme
	if th == nil {
		p := lipgloss.NewStyle()
		return styles{border: p, title: p, faint: p, dim: p, prompt: p, cursor: p,
			text: p, textSel: p, match: p, num: p, edge: p, name: p,
			open: p, merged: p, closed: p, draft: p}
	}
	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return styles{
		border:  fg(th.Primary),
		title:   fg(th.TextBright).Bold(true),
		faint:   fg(th.Faint),
		dim:     fg(th.TextDim),
		prompt:  fg(th.AccentText).Bold(true),
		cursor:  fg(th.Primary),
		text:    fg(th.Text),
		textSel: fg(th.TextBright).Bold(true),
		match:   fg(th.AccentText).Bold(true).Underline(true),
		num:     fg(th.AccentText),
		edge:    fg(th.AccentText),
		name:    fg(th.Text).Bold(true),
		open:    fg(th.StateOpen),
		merged:  fg(th.StateMerged),
		closed:  fg(th.StateClosed),
		draft:   fg(th.StateDraft),
		selBg:   th.Highlight,
		th:      th,
	}
}

// boxSize returns the palette's outer size. The box grows with the result
// list up to defaultSearchLimit rows, so a short list doesn't leave a tall
// empty box.
func (m Model) boxSize() (int, int) {
	// Below this the border, query and one result row don't fit.
	if m.width < 4 || m.height < boxChrome+1 {
		return 0, 0
	}
	boxW := int(math.Round(float64(m.width) * 0.6))
	boxW = min(max(boxW, minBoxWidth), maxBoxWidth, m.width)
	if m.pickKind == pickTheme {
		boxW = min(themeBoxWidth, m.width)
	}
	boxH := min(boxChrome+m.resultRows(), m.height)
	return boxW, boxH
}

// resultRows is the number of rows given to the result list (at least one,
// for the empty state).
func (m Model) resultRows() int {
	avail := m.height - boxChrome - 2
	rows := min(len(m.results), defaultSearchLimit, avail)
	return max(rows, 1)
}

func (m Model) visibleResultCount() int {
	if _, boxH := m.boxSize(); boxH == 0 {
		return 0
	}
	return m.resultRows()
}

// boxOrigin places the box horizontally centred, a fifth of the way down, so
// its top edge stays put while the result list grows and shrinks. The theme
// picker's list is short and fixed, so it sits in the middle instead, centred
// on its unfiltered height so filtering doesn't move it.
func (m Model) boxOrigin(boxW, boxH int) (row, col int) {
	row = max(min(m.height/5, m.height-boxH), 0)
	if m.pickKind == pickTheme {
		full := min(boxChrome+min(len(m.pickPool), defaultSearchLimit), m.height)
		row = max(min((m.height-full)/2, m.height-boxH), 0)
	}
	col = max((m.width-boxW)/2, 0)
	return row, col
}

// View renders the palette on a blank background.
func (m Model) View() string {
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return ""
	}
	blank := strings.Repeat(" ", m.width)
	bg := make([]string, m.height)
	for i := range bg {
		bg[i] = blank
	}
	return m.composite(strings.Join(bg, "\n"), 0)
}

// ViewOver composites the palette onto bg, which is dimmed so the palette
// stands out. Only the box footprint is replaced.
func (m Model) ViewOver(bg string) string {
	return m.ViewOverStatus(bg, "")
}

// ViewOverStatus is ViewOver for a screen made of body and a status bar
// below it: only the body is dimmed, since the status bar describes the
// palette's own keys.
func (m Model) ViewOverStatus(body, status string) string {
	bg, dimRows := body, 0
	// The theme picker previews on the screen behind it, so leave it lit.
	if m.theme != nil && m.pickKind != pickTheme {
		dimRows = strings.Count(body, "\n") + 1
	}
	switch {
	case strings.TrimSpace(body) == "":
		bg, dimRows = status, 0
	case status != "":
		bg += "\n" + status
	}
	return m.composite(bg, dimRows)
}

// composite draws the box over bg, dimming bg's first dimRows rows.
func (m Model) composite(bg string, dimRows int) string {
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		return bg
	}
	boxLines := strings.Split(m.renderBox(boxW, boxH), "\n")
	startRow, startCol := m.boxOrigin(boxW, boxH)

	result := strings.Split(bg, "\n")
	for i, line := range result {
		if i < dimRows {
			line = theme.Backdrop(m.theme, line)
		}
		if j := i - startRow; j >= 0 && j < len(boxLines) {
			left := ansi.Cut(line, 0, startCol)
			if w := ansi.StringWidth(left); w < startCol {
				left += strings.Repeat(" ", startCol-w)
			}
			line = left + boxLines[j] + ansi.Cut(line, startCol+boxW, m.width)
		}
		result[i] = line
	}
	return strings.Join(result, "\n")
}

// renderBox draws the palette: title and position in the top border, the
// query, a divider, the results, and a loading note in the bottom border.
// Key hints live in the status bar.
func (m Model) renderBox(boxW, boxH int) string {
	st := m.styles()
	innerW := max(boxW-2, 0)

	lines := make([]string, 0, boxH)
	lines = append(lines, m.borderLine(st, "╭", "╮", st.title.Render(m.boxTitle()), m.topRight(st), innerW))
	lines = append(lines, m.side(st, m.queryLine(st, innerW), innerW))
	lines = append(lines, st.border.Render("├"+strings.Repeat("─", innerW)+"┤"))

	rows := boxH - boxChrome
	visible := m.visibleResults(rows)
	if len(visible) == 0 && rows > 0 {
		lines = append(lines, m.side(st, "   "+st.dim.Render(m.emptyText()), innerW))
	}
	for i, r := range visible {
		lines = append(lines, m.side(st, m.resultLine(st, r, m.scrollOffset+i == m.selectedIndex, innerW), innerW))
	}
	for len(lines) < boxH-1 {
		lines = append(lines, m.side(st, "", innerW))
	}
	lines = append(lines, m.borderLine(st, "╰", "╯", "", m.loadingNote(st), innerW))
	return strings.Join(lines, "\n")
}

// side wraps content in the box's vertical borders, fitted to innerW.
func (m Model) side(st styles, content string, innerW int) string {
	return st.border.Render("│") + fit(content, innerW) + st.border.Render("│")
}

// borderLine draws a horizontal border with left and right labels set into
// it: "╭─ left ──── right ─╮". The right label is dropped first when space
// runs out, then the left one is truncated.
func (m Model) borderLine(st styles, l, r, left, right string, innerW int) string {
	seg := func(s string) string {
		if s == "" {
			return ""
		}
		return " " + s + " "
	}
	lw, rw := lipgloss.Width(seg(left)), lipgloss.Width(seg(right))
	if 2+lw+rw > innerW {
		right, rw = "", 0
	}
	if 2+lw > innerW {
		left = ansi.Truncate(left, max(innerW-4, 0), "…")
		lw = lipgloss.Width(seg(left))
	}
	fill := max(innerW-2-lw-rw, 0)
	return st.border.Render(l+"─") + seg(left) + st.border.Render(strings.Repeat("─", fill)) +
		seg(right) + st.border.Render("─"+r)
}

func (m Model) topRight(st styles) string {
	if m.pickKind == pickLabel {
		return m.labelTopRight(st)
	}
	if len(m.results) == 0 {
		return ""
	}
	return st.faint.Render(fmt.Sprintf("%d/%d", m.selectedIndex+1, len(m.results)))
}

func (m Model) loadingNote(st styles) string {
	if m.hydrating && m.pickKind == pickNone {
		return st.dim.Render("Loading…")
	}
	return ""
}

func (m Model) queryLine(st styles, innerW int) string {
	cur := min(m.cursor, len(m.query))
	line := " " + st.prompt.Render("›") + " " + st.text.Render(m.query[:cur]) + st.cursor.Render("▏") + st.text.Render(m.query[cur:])
	if m.query == "" {
		line += st.faint.Render(m.placeholder())
	}
	return fit(line, innerW)
}

func (m Model) placeholder() string {
	switch m.pickKind {
	case pickRepo:
		return "Filter repositories"
	case pickTheme:
		return "Filter themes"
	case pickFile:
		return "Filter files"
	case pickLabel:
		return "Filter or create labels"
	}
	return "Search pull requests and repositories"
}

func (m Model) emptyText() string {
	if m.pickKind == pickLabel {
		return m.labelEmptyText()
	}
	switch {
	case m.query != "":
		return fmt.Sprintf("No matches for “%s”", m.query)
	case m.pickKind == pickRepo:
		return "No repositories"
	case m.pickKind == pickTheme:
		return "No themes"
	case m.pickKind == pickFile:
		return "No files"
	case m.hydrating:
		return "Indexing pull requests…"
	default:
		return "No pull requests yet"
	}
}

func (m Model) visibleResults(rows int) []domain.SearchResult {
	if rows <= 0 || len(m.results) == 0 {
		return nil
	}
	start := min(max(m.scrollOffset, 0), len(m.results))
	end := min(start+rows, len(m.results))
	return m.results[start:end]
}

// resultLine renders one result: an accent edge on the selected row, a
// state glyph, the number, the title with the query match emphasised, and
// right-aligned metadata. The selected row is tinted across its full width.
func (m Model) resultLine(st styles, r domain.SearchResult, selected bool, width int) string {
	switch r.Kind {
	case themeResult:
		return m.themeLine(st, r.Title, selected, width)
	case fileResult:
		return m.fileLine(st, r.Title, selected, width)
	case labelResult:
		return m.labelLine(st, r.Title, selected, width)
	case labelCreateResult:
		return m.labelCreateLine(st, r.Title, selected, width)
	}
	edge := " "
	if selected {
		edge = st.edge.Render("▎")
	}
	text := st.text
	if selected {
		text = st.textSel
	}

	var glyph, label, meta string
	switch r.Kind {
	case domain.SearchResultRepo:
		glyph = st.dim.Render("▣")
		owner, name, ok := strings.Cut(r.Repo, "/")
		if !ok {
			owner, name = "", r.Repo
		} else {
			owner += "/"
		}
		label = st.dim.Render(owner) + st.name.Render(name)
		meta = st.faint.Render("repository")
	default:
		glyph = m.glyphStyle(st, r.State, r.IsDraft).Render(prStateGlyph(r.State, r.IsDraft))
		num := fmt.Sprintf("%-*s", m.numWidth(), fmt.Sprintf("#%d", r.Number))
		label = st.num.Render(num) + "  "
		var parts []string
		if r.Repo != "" && r.Repo != m.activeRepo {
			parts = append(parts, r.Repo)
		}
		if r.Author != "" {
			parts = append(parts, r.Author)
		}
		meta = st.dim.Render(strings.Join(parts, " · "))
	}

	prefix := edge + " " + glyph + " " + label
	metaW := lipgloss.Width(meta)
	if metaW > width/3 {
		meta = ansi.Truncate(meta, width/3, "…")
		metaW = lipgloss.Width(meta)
	}
	titleW := width - lipgloss.Width(prefix) - metaW - 3
	if r.Kind == domain.SearchResultRepo {
		titleW = 0 // the repo name is the label
	}
	title := ""
	if titleW > 0 {
		title = highlightMatch(ansi.Truncate(r.Title, titleW, "…"), m.query, text, st.match)
	}
	left := prefix + title
	gap := max(width-lipgloss.Width(left)-metaW-1, 1)
	line := fit(left+strings.Repeat(" ", gap)+meta, width)
	if selected && st.th != nil {
		return st.th.FillRow(st.selBg, width, line)
	}
	return line
}

// numWidth is the width of the widest "#N" among the results, so titles
// line up.
func (m Model) numWidth() int {
	w := 0
	for _, r := range m.results {
		if r.Kind == domain.SearchResultPR {
			w = max(w, len(fmt.Sprintf("#%d", r.Number)))
		}
	}
	return w
}

// highlightMatch renders text in base, with the first case-insensitive
// occurrence of query in hl. The search service matches by substring, so
// this marks why the result matched (when it matched on the title).
func highlightMatch(text, query string, base, hl lipgloss.Style) string {
	return highlightMatchWith(text, query,
		func(s string) string { return base.Render(s) },
		func(s string) string { return hl.Render(s) })
}

func highlightMatchWith(text, query string, base, hl func(string) string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	lower := strings.ToLower(text)
	i := strings.Index(lower, q)
	// Lowercasing can change byte lengths outside ASCII; skip highlighting
	// rather than slice at the wrong offset.
	if q == "" || i < 0 || len(lower) != len(text) {
		return base(text)
	}
	j := i + len(q)
	return base(text[:i]) + hl(text[i:j]) + base(text[j:])
}

func prStateGlyph(state domain.PRState, isDraft bool) string {
	if isDraft {
		return "○"
	}
	switch state {
	case domain.PRStateMerged:
		return "✓"
	case domain.PRStateClosed:
		return "✕"
	default:
		return "◆"
	}
}

func (m Model) glyphStyle(st styles, state domain.PRState, isDraft bool) lipgloss.Style {
	if isDraft {
		return st.draft
	}
	switch state {
	case domain.PRStateMerged:
		return st.merged
	case domain.PRStateClosed:
		return st.closed
	default:
		return st.open
	}
}

// fit truncates or pads s to exactly width cells.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if w := lipgloss.Width(s); w > width {
		s = ansi.Truncate(s, width, "…")
	}
	if w := lipgloss.Width(s); w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// themeLine renders a theme picker row: the name, a swatch of the theme's
// accent and status colours, and a tag on the theme active at open.
func (m Model) themeLine(st styles, name string, selected bool, width int) string {
	edge := " "
	text := st.text
	if selected {
		edge = st.edge.Render("▎")
		text = st.textSel
	}
	const nameW = 20
	left := edge + "  " + highlightMatch(name, m.query, text, st.match) +
		strings.Repeat(" ", max(nameW-lipgloss.Width(name), 1))

	dots := make([]string, 0, 5)
	for _, c := range m.themeSwatch[name] {
		dots = append(dots, lipgloss.NewStyle().Foreground(c).Render("●"))
	}
	swatch := strings.Join(dots, " ")

	tag := ""
	if name == m.themeCurrent {
		tag = st.faint.Render("current")
	}
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(swatch)-lipgloss.Width(tag)-2, 1)
	line := fit(left+swatch+strings.Repeat(" ", gap)+tag, width)
	if selected && st.th != nil {
		return st.th.FillRow(st.selBg, width, line)
	}
	return line
}

func (m Model) boxTitle() string {
	if m.pickKind != pickNone {
		return m.pickHint
	}
	return "Go to"
}

// fileLine renders a file picker row: the change status, the path with its
// directory dimmed and the query match emphasised, and the line counts. A
// path too long for the row loses its start, so the file name stays.
func (m Model) fileLine(st styles, path string, selected bool, width int) string {
	f := m.fileItems[path]
	edge, name := " ", st.text
	if selected {
		edge, name = st.edge.Render("▎"), st.textSel
	}
	status, adds, dels := fileStatusLetter(f.Status), fmt.Sprintf("+%d", f.Additions), fmt.Sprintf("-%d", f.Deletions)
	if th := st.th; th != nil {
		switch f.Status {
		case "added":
			status = th.Additions.Render(status)
		case "removed":
			status = th.Deletions.Render(status)
		case "renamed":
			status = th.CIPending.Render(status)
		default:
			status = th.DimTxt.Render(status)
		}
		adds, dels = th.Additions.Render(adds), th.Deletions.Render(dels)
	}
	prefix := edge + " " + status + " "
	meta := adds + " " + dels
	pathW := width - lipgloss.Width(prefix) - lipgloss.Width(meta) - 3
	if pathW <= 0 {
		return fit(prefix+meta, width)
	}
	shown := path
	if ansi.StringWidth(shown) > pathW {
		shown = ansi.TruncateLeft(shown, ansi.StringWidth(shown)-pathW+1, "…")
	}

	// Style a span of shown by which side of the last "/" it falls on.
	dirEnd := strings.LastIndex(shown, "/") + 1
	span := func(from, to int, hl bool) string {
		if from >= to {
			return ""
		}
		if hl {
			return st.match.Render(shown[from:to])
		}
		mid := min(max(dirEnd, from), to)
		return st.dim.Render(shown[from:mid]) + name.Render(shown[mid:to])
	}
	// Mark the last occurrence, so a match in the file name wins over one
	// in its directory. highlightMatch marks the first and styles the text
	// as one span, hence the separate pass here.
	styled := span(0, len(shown), false)
	q := strings.ToLower(strings.TrimSpace(m.query))
	lower := strings.ToLower(shown)
	if i := strings.LastIndex(lower, q); q != "" && i >= 0 && len(lower) == len(shown) {
		j := i + len(q)
		styled = span(0, i, false) + span(i, j, true) + span(j, len(shown), false)
	}

	left := prefix + styled
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(meta)-1, 1)
	line := fit(left+strings.Repeat(" ", gap)+meta, width)
	if selected && st.th != nil {
		return st.th.FillRow(st.selBg, width, line)
	}
	return line
}

func fileStatusLetter(status string) string {
	switch status {
	case "added":
		return "A"
	case "removed":
		return "D"
	case "renamed":
		return "R"
	default:
		return "M"
	}
}
