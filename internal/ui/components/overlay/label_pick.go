package overlay

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/labels"
)

// maxLabelNameLen is GitHub's limit on a label name, in characters.
const maxLabelNameLen = 50

// labelResult marks a label row; labelCreateResult the "Create label" row,
// whose Title is the name to create.
const (
	labelResult       domain.SearchResultKind = "label"
	labelCreateResult domain.SearchResultKind = "label-create"
)

// LabelItem is one label in the label picker.
type LabelItem struct {
	Name        string
	Color       string
	Description string
	// New marks a label the user asked to create; it doesn't exist yet.
	New bool
}

// labelPick is the label picker's state. Marks live here, keyed by name,
// rather than in the filtered results, so filtering never loses them.
type labelPick struct {
	items   map[string]LabelItem
	byFold  map[string]string // foldName(name) -> name, to dedupe by case
	applied map[string]bool   // labels on the PR (the baseline)
	marked  map[string]bool   // labels the PR will have after Enter
	order   []string          // applied names in PR order, frozen at open
	created []string          // new labels, in the order they were added

	loading   bool   // repo labels not in yet
	loaded    bool   // repo labels in, so "doesn't exist" is known
	hold      bool   // full PR label list not in yet
	holdErr   string // the full PR label list failed to load
	truncated bool   // repo has more labels than were listed
	err       string
}

// OpenLabelPick switches the palette into the label chooser, starting from
// the PR's current labels while the repository's labels load (SetRepoLabels).
// The caller handles Enter and Esc itself, reading LabelChange.
func (m *Model) OpenLabelPick(current []domain.Label) {
	m.pickKind = pickLabel
	m.pickHint = "Labels"
	m.labels = labelPick{
		items:   map[string]LabelItem{},
		byFold:  map[string]string{},
		applied: map[string]bool{},
		marked:  map[string]bool{},
		loading: true,
	}
	m.setAppliedLabels(current)
	m.rebuildLabelPool()
}

// IsLabelPick reports whether the palette is the label chooser.
func (m Model) IsLabelPick() bool {
	return m.pickKind == pickLabel
}

// SetRepoLabels merges in the repository's labels, listed after the PR's own
// in alphabetical order, and enables creating labels that aren't among them.
func (m *Model) SetRepoLabels(list []domain.RepoLabel, truncated bool) {
	if m.pickKind != pickLabel {
		return
	}
	for _, l := range list {
		if name, ok := m.labels.byFold[foldName(l.Name)]; ok {
			it := m.labels.items[name]
			if it.New {
				continue // it now exists; leave the user's pending row alone
			}
			it.Description = l.Description
			if it.Color == "" {
				it.Color = l.Color
			}
			m.labels.items[name] = it
			continue
		}
		m.addLabelItem(LabelItem{Name: l.Name, Color: l.Color, Description: l.Description})
	}
	m.labels.loading, m.labels.loaded, m.labels.err = false, true, ""
	m.labels.truncated = truncated
	m.rebuildLabelPool()
}

// SetLabelsErr records that the repository's labels failed to load. The PR's
// own labels stay listed (and removable); creating is off, since whether a
// label exists is unknown.
func (m *Model) SetLabelsErr(msg string) {
	if m.pickKind != pickLabel {
		return
	}
	m.labels.loading, m.labels.err = false, msg
	m.refreshResults()
}

// SetPRLabelsLoading marks the picker as waiting for the PR's full label
// list (its preview holds only the first page); the border says it's loading.
func (m *Model) SetPRLabelsLoading() {
	m.labels.hold = true
}

// LoadingPRLabels reports whether the PR's full label list is still loading.
func (m Model) LoadingPRLabels() bool {
	return m.pickKind == pickLabel && m.labels.hold
}

// SetPRLabelsErr records that the PR's full label list failed to load, so
// labels past the first page may show unmarked.
func (m *Model) SetPRLabelsErr(msg string) {
	if m.pickKind == pickLabel {
		m.labels.holdErr = msg
	}
}

// SetPRLabels replaces the baseline with the PR's full label list, keeping
// any toggles the user already made.
func (m *Model) SetPRLabels(full []domain.Label) {
	if m.pickKind != pickLabel {
		return
	}
	flipped := map[string]bool{}
	for name := range m.labels.items {
		if m.labels.marked[name] != m.labels.applied[name] {
			flipped[name] = true
		}
	}
	m.labels.applied, m.labels.order = map[string]bool{}, nil
	m.setAppliedLabels(full)
	for name := range m.labels.items {
		m.labels.marked[name] = m.labels.applied[name] != flipped[name]
	}
	m.labels.hold = false
	m.rebuildLabelPool()
}

func (m *Model) setAppliedLabels(current []domain.Label) {
	for _, l := range current {
		name, ok := m.labels.byFold[foldName(l.Name)]
		if !ok {
			name = l.Name
			m.addLabelItem(LabelItem{Name: l.Name, Color: l.Color})
		}
		if m.labels.applied[name] {
			continue
		}
		m.labels.applied[name], m.labels.marked[name] = true, true
		m.labels.order = append(m.labels.order, name)
	}
}

func (m *Model) addLabelItem(it LabelItem) {
	m.labels.items[it.Name] = it
	m.labels.byFold[foldName(it.Name)] = it.Name
}

// foldName is the key labels are matched by: the name as drawn (see
// labels.CleanName), ignoring case.
func foldName(name string) string {
	return strings.ToLower(labels.CleanName(name))
}

// rebuildLabelPool orders the rows: the PR's labels as they were at open,
// then labels being created, then the rest A→Z. Toggling never reorders.
func (m *Model) rebuildLabelPool() {
	first := map[string]bool{}
	names := make([]string, 0, len(m.labels.items))
	for _, n := range append(append([]string{}, m.labels.order...), m.labels.created...) {
		if _, ok := m.labels.items[n]; ok && !first[n] {
			first[n] = true
			names = append(names, n)
		}
	}
	var rest []string
	for n := range m.labels.items {
		if !first[n] {
			rest = append(rest, n)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		a, b := strings.ToLower(rest[i]), strings.ToLower(rest[j])
		if a != b {
			return a < b
		}
		return rest[i] < rest[j]
	})
	names = append(names, rest...)

	m.pickPool = make([]domain.SearchResult, len(names))
	for i, n := range names {
		m.pickPool[i] = domain.SearchResult{Kind: labelResult, Title: n}
	}
	m.refreshResults()
}

// createRow is the "Create label" row for the current query, if one is due:
// the repo's labels are loaded and no label matches the query, ignoring case.
func (m Model) createRow() (domain.SearchResult, bool) {
	name := labels.CleanName(m.query)
	if name == "" || !m.labels.loaded {
		return domain.SearchResult{}, false
	}
	if _, exists := m.labels.byFold[foldName(name)]; exists {
		return domain.SearchResult{}, false
	}
	return domain.SearchResult{Kind: labelCreateResult, Title: name}, true
}

// ToggleMark flips the highlighted label, or on the "Create label" row adds
// the new label, marked, and clears the filter. advance moves the selection
// down after a plain toggle (Tab). It reports whether anything changed.
func (m *Model) ToggleMark(advance bool) bool {
	if m.pickKind != pickLabel || m.selectedIndex < 0 || m.selectedIndex >= len(m.results) {
		return false
	}
	r := m.results[m.selectedIndex]
	if r.Kind == labelCreateResult {
		return m.addNewLabel(r.Title)
	}
	it, ok := m.labels.items[r.Title]
	if !ok {
		return false
	}
	if it.New {
		// Not created yet: unmarking drops it.
		delete(m.labels.items, it.Name)
		delete(m.labels.byFold, foldName(it.Name))
		delete(m.labels.marked, it.Name)
		m.labels.created = removeString(m.labels.created, it.Name)
		m.rebuildLabelPool()
		return true
	}
	m.labels.marked[it.Name] = !m.labels.marked[it.Name]
	if advance {
		m.moveSelection(1)
	}
	return true
}

func (m *Model) addNewLabel(name string) bool {
	if utf8.RuneCountInString(name) > maxLabelNameLen {
		return false
	}
	m.addLabelItem(LabelItem{Name: name, Color: labels.PresetColor(name), New: true})
	m.labels.marked[name] = true
	m.labels.created = append(m.labels.created, name)
	m.query, m.cursor = "", 0
	m.rebuildLabelPool()
	for i, r := range m.results {
		if r.Title == name && r.Kind == labelResult {
			m.selectedIndex = i
			m.ensureSelectionVisible()
			break
		}
	}
	return true
}

// LabelChange is what closing the picker saves: labels to create, then add and remove,
// each in row order.
func (m Model) LabelChange() domain.LabelChange {
	var c domain.LabelChange
	if m.pickKind != pickLabel {
		return c
	}
	for _, r := range m.pickPool {
		it := m.labels.items[r.Title]
		marked, applied := m.labels.marked[it.Name], m.labels.applied[it.Name]
		switch {
		case it.New && marked:
			c.Create = append(c.Create, domain.Label{Name: it.Name, Color: it.Color})
			c.Add = append(c.Add, it.Name)
		case marked && !applied:
			c.Add = append(c.Add, it.Name)
		case applied && !marked:
			c.Remove = append(c.Remove, it.Name)
		}
	}
	return c
}

// MarkedLabels is the PR's label list after the change: kept labels in their
// PR order, then added ones in row order.
func (m Model) MarkedLabels() []domain.Label {
	var out []domain.Label
	for _, r := range m.pickPool {
		if it := m.labels.items[r.Title]; m.labels.marked[it.Name] {
			out = append(out, domain.Label{Name: it.Name, Color: it.Color})
		}
	}
	return out
}

// PendingLabelCount is how many labels saving would add or remove.
func (m Model) PendingLabelCount() int {
	c := m.LabelChange()
	return len(c.Add) + len(c.Remove)
}

func removeString(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// labelTopRight is the label picker's border note: loading, then a load
// error, then pending changes, then how many labels the PR has.
func (m Model) labelTopRight(st styles) string {
	switch {
	case m.labels.holdErr != "":
		return st.dim.Render("Couldn't load this PR's labels")
	case m.labels.loading || m.labels.hold:
		return st.dim.Render("Loading…")
	case m.labels.err != "":
		return st.dim.Render("Couldn't load labels")
	}
	if n := m.PendingLabelCount(); n > 0 {
		return st.num.Render(fmt.Sprintf("%d pending", n))
	}
	note := fmt.Sprintf("%d applied", len(m.labels.applied))
	if m.labels.truncated {
		note += " · first 1000 shown"
	}
	return st.faint.Render(note)
}

func (m Model) labelEmptyText() string {
	switch {
	case m.labels.err != "":
		return "Couldn't load labels: " + m.labels.err
	case m.labels.loading:
		return "Loading labels…"
	case m.query != "":
		return fmt.Sprintf("No matches for “%s”", m.query)
	default:
		return "No labels in this repository"
	}
}

// labelLine renders a label row: a check box, the coloured dot, the name with
// the query match emphasised, and the description (or "new") dimmed.
func (m Model) labelLine(st styles, name string, selected bool, width int) string {
	it := m.labels.items[name]
	edge, text := " ", st.text
	if selected {
		edge, text = st.edge.Render("▎"), st.textSel
	}
	box := "[ ]"
	if m.labels.marked[name] {
		box = st.num.Render("[x]")
	}
	shown := ansi.Truncate(labels.CleanName(name), m.labelNameWidth(), "…")
	left := edge + box + " " + labels.Dot(st.th, it.Color) + " " + highlightMatch(shown, m.query, text, st.match)

	desc := it.Description
	if it.New {
		desc = "new"
	}
	if desc != "" {
		pad := max(m.labelNameWidth()-ansi.StringWidth(shown), 0) + 2
		left += strings.Repeat(" ", pad) + st.dim.Render(labels.CleanName(desc))
	}
	line := fit(left, width)
	if selected && st.th != nil {
		return st.th.FillRow(st.selBg, width, line)
	}
	return line
}

// labelNameWidth is the name column: the widest listed name, capped so
// descriptions keep some room.
func (m Model) labelNameWidth() int {
	w := 0
	for _, r := range m.results {
		if r.Kind == labelResult {
			w = max(w, ansi.StringWidth(labels.CleanName(r.Title)))
		}
	}
	return min(w, 24)
}

// labelCreateLine renders the "Create label" row, or why it can't be used.
func (m Model) labelCreateLine(st styles, name string, selected bool, width int) string {
	edge := " "
	if selected {
		edge = st.edge.Render("▎")
	}
	var body string
	if utf8.RuneCountInString(name) > maxLabelNameLen {
		body = st.dim.Render(fmt.Sprintf("Name too long (max %d)", maxLabelNameLen))
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			st.text.Render("Create label "), labels.Dot(st.th, labels.PresetColor(name)), " ",
			st.textSel.Render("“"+name+"”"))
	}
	line := fit(edge+" "+st.num.Render("+")+"   "+body, width)
	if selected && st.th != nil {
		return st.th.FillRow(st.selBg, width, line)
	}
	return line
}
