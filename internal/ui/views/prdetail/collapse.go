package prdetail

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
)

// DiffLimits decides which files start collapsed and which are too large to
// show in pho at all (those only open on GitHub).
type DiffLimits struct {
	// CollapseLines: files with more changed lines start collapsed.
	CollapseLines int
	// CollapseLineWidth: files with a longer line start collapsed.
	CollapseLineWidth int
	// MaxLines: files with more changed lines can't be expanded.
	MaxLines int
	// MaxLineWidth: files with a longer line can't be expanded.
	MaxLineWidth int
	// RowBudget: when expanded files would exceed this many rows, the
	// largest are collapsed until the diff fits.
	RowBudget int
}

// DefaultDiffLimits returns the limits used when the config sets none.
func DefaultDiffLimits() DiffLimits {
	return DiffLimits{
		CollapseLines:     1000,
		CollapseLineWidth: 5000,
		MaxLines:          5000,
		MaxLineWidth:      20000,
		RowBudget:         20000,
	}
}

// withDefaults fills unset (zero or negative) limits from DefaultDiffLimits.
func (l DiffLimits) withDefaults() DiffLimits {
	d := DefaultDiffLimits()
	pick := func(v, def int) int {
		if v > 0 {
			return v
		}
		return def
	}
	return DiffLimits{
		CollapseLines:     pick(l.CollapseLines, d.CollapseLines),
		CollapseLineWidth: pick(l.CollapseLineWidth, d.CollapseLineWidth),
		MaxLines:          pick(l.MaxLines, d.MaxLines),
		MaxLineWidth:      pick(l.MaxLineWidth, d.MaxLineWidth),
		RowBudget:         pick(l.RowBudget, d.RowBudget),
	}
}

type collapseReason uint8

const (
	reasonNone collapseReason = iota
	reasonGenerated
	reasonLarge
	reasonLongLines
	reasonNoPatch
	reasonBudget
	reasonUser
)

func (r collapseReason) label() string {
	switch r {
	case reasonGenerated:
		return "generated"
	case reasonLarge:
		return "large diff"
	case reasonLongLines:
		return "very long lines"
	case reasonNoPatch:
		return "GitHub sent no diff"
	case reasonBudget:
		return "collapsed to keep this PR fast"
	}
	return ""
}

// fileView is the per-file collapse state for the loaded diff.
type fileView struct {
	collapsed bool
	// tooLarge files stay collapsed; they only open on GitHub.
	tooLarge bool
	reason   collapseReason
}

// diffLayout is the single source of truth for diff row positions: every
// file's first row and row count, honouring collapsed files.
type diffLayout struct {
	fileStart []int
	fileRows  []int
	total     int
}

// collapsedFileRows is the height of a collapsed file: blank + header bar +
// blank + one placeholder row.
const collapsedFileRows = diffFileHeaderRows + 1

// placeholderLine marks the cursor stop on a collapsed file's placeholder row.
const placeholderLine = -1

func (c diffCursorLine) isPlaceholder() bool { return c.LineIdx == placeholderLine }

// generatedNames and generatedSuffixes match files GitHub also treats as
// generated: lockfiles, vendored and built output, minified bundles.
var (
	generatedNames = map[string]bool{
		"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
		"go.sum": true, "Cargo.lock": true, "poetry.lock": true,
		"composer.lock": true, "Gemfile.lock": true, "Pipfile.lock": true,
		"bun.lockb": true, "npm-shrinkwrap.json": true, "flake.lock": true,
	}
	generatedSuffixes = []string{
		".lock", ".min.js", ".min.css", ".map", ".snap", ".pb.go",
		"_pb2.py", ".pb.cc", ".pb.h",
	}
	generatedDirs = []string{"vendor/", "node_modules/", "dist/"}
)

func isGeneratedPath(p string) bool {
	base := path.Base(p)
	if generatedNames[base] {
		return true
	}
	for _, s := range generatedSuffixes {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	if strings.Contains(base, "_generated.") || strings.Contains(base, ".generated.") {
		return true
	}
	for _, d := range generatedDirs {
		if strings.HasPrefix(p, d) || strings.Contains(p, "/"+d) {
			return true
		}
	}
	return false
}

// classifyFile returns how a file starts: collapsed or not, and whether it
// is too large to expand.
func classifyFile(f *diffmodel.DiffFile, lim DiffLimits) fileView {
	if f.IsBinary {
		return fileView{}
	}
	changed := f.Additions + f.Deletions
	switch {
	case f.PatchUnavailable:
		return fileView{collapsed: true, tooLarge: true, reason: reasonNoPatch}
	case changed > lim.MaxLines:
		return fileView{collapsed: true, tooLarge: true, reason: reasonLarge}
	case f.MaxLineLen > lim.MaxLineWidth:
		return fileView{collapsed: true, tooLarge: true, reason: reasonLongLines}
	case isGeneratedPath(f.NewPath):
		return fileView{collapsed: true, reason: reasonGenerated}
	case changed > lim.CollapseLines:
		return fileView{collapsed: true, reason: reasonLarge}
	case f.MaxLineLen > lim.CollapseLineWidth:
		return fileView{collapsed: true, reason: reasonLongLines}
	}
	return fileView{}
}

// pinnedPaths returns paths that must not start collapsed: files with the
// user's drafts or with review threads.
func (m *PRDetailModel) pinnedPaths() map[string]bool {
	pinned := make(map[string]bool)
	for _, d := range m.drafts {
		pinned[d.Path] = true
	}
	if m.Detail != nil {
		for _, t := range m.Detail.ReviewThreads {
			pinned[t.Path] = true
		}
	}
	return pinned
}

// resetCollapse drops collapse state for a newly loaded diff and applies the
// automatic rules.
func (m *PRDetailModel) resetCollapse() {
	m.fileViews, m.viewsFor, m.collapseTouched = nil, nil, false
	m.applyAutoCollapse()
}

// applyAutoCollapse sets every file's starting collapse state. It runs when
// a diff loads and again when drafts or review threads arrive, until the user
// toggles a file themselves.
func (m *PRDetailModel) applyAutoCollapse() {
	if m.Diff == nil {
		m.fileViews, m.viewsFor = nil, nil
		m.rebuildDiffLayout()
		return
	}
	if m.collapseTouched && m.viewsFor == m.Diff {
		return
	}
	lim := m.Limits.withDefaults()
	pinned := m.pinnedPaths()
	// Never hide the file the user is on when the rules re-run.
	pin := func(fi int) {
		if fi >= 0 && fi < len(m.Diff.Files) {
			pinned[m.Diff.Files[fi].NewPath] = true
		}
	}
	if m.validLineCursor() {
		pin(m.diffCursor.FileIdx)
	}
	if m.visual.Active {
		pin(m.visual.FileIdx)
	}
	views := make([]fileView, len(m.Diff.Files))
	expandedRows := 0
	for i := range m.Diff.Files {
		f := &m.Diff.Files[i]
		v := classifyFile(f, lim)
		if v.collapsed && !v.tooLarge && (pinned[f.NewPath] || pinned[f.OldPath]) {
			v = fileView{}
		}
		views[i] = v
		if !v.collapsed {
			expandedRows += diffFileDisplayRows(f)
		}
	}
	if expandedRows > lim.RowBudget {
		// Collapse the largest unpinned files until the rest fits.
		order := make([]int, 0, len(views))
		for i, v := range views {
			f := &m.Diff.Files[i]
			if !v.collapsed && !f.IsBinary && !pinned[f.NewPath] && !pinned[f.OldPath] {
				order = append(order, i)
			}
		}
		slices.SortStableFunc(order, func(a, b int) int {
			return cmp.Compare(diffFileDisplayRows(&m.Diff.Files[b]), diffFileDisplayRows(&m.Diff.Files[a]))
		})
		for _, i := range order {
			if expandedRows <= lim.RowBudget {
				break
			}
			views[i] = fileView{collapsed: true, reason: reasonBudget}
			expandedRows -= diffFileDisplayRows(&m.Diff.Files[i]) - collapsedFileRows
		}
	}
	m.fileViews, m.viewsFor = views, m.Diff
	m.rebuildDiffLayout()
}

// views returns the collapse state for the current diff, creating an
// all-expanded one when it belongs to another diff.
func (m *PRDetailModel) views() []fileView {
	if m.Diff == nil {
		return nil
	}
	if m.viewsFor != m.Diff || len(m.fileViews) != len(m.Diff.Files) {
		m.fileViews, m.viewsFor = make([]fileView, len(m.Diff.Files)), m.Diff
	}
	return m.fileViews
}

func (m *PRDetailModel) fileCollapsed(i int) bool {
	return m.viewsFor == m.Diff && i >= 0 && i < len(m.fileViews) && m.fileViews[i].collapsed
}

func (m *PRDetailModel) fileTooLarge(i int) bool {
	return m.viewsFor == m.Diff && i >= 0 && i < len(m.fileViews) && m.fileViews[i].tooLarge
}

// fileDisplayRows returns file i's row count in the current layout.
func (m *PRDetailModel) fileDisplayRows(i int) int {
	if m.fileCollapsed(i) {
		return collapsedFileRows
	}
	return diffFileDisplayRows(&m.Diff.Files[i])
}

// rows returns the current layout, recomputing it when it was built for a
// different diff.
func (m *PRDetailModel) rows() *diffLayout {
	if m.layoutFor != m.Diff || (m.Diff != nil && len(m.layout.fileStart) != len(m.Diff.Files)) {
		m.computeLayout()
	}
	return &m.layout
}

func (m *PRDetailModel) computeLayout() {
	m.layout, m.layoutFor = diffLayout{}, m.Diff
	if m.Diff != nil {
		n := len(m.Diff.Files)
		m.layout.fileStart = make([]int, n)
		m.layout.fileRows = make([]int, n)
		row := 0
		for i := range m.Diff.Files {
			rows := m.fileDisplayRows(i)
			m.layout.fileStart[i] = row
			m.layout.fileRows[i] = rows
			row += rows
		}
		m.layout.total = row
	}
	m.leftPanel.Collapsed = m.leftPanel.Collapsed[:0]
	if m.Diff != nil {
		for i := range m.Diff.Files {
			m.leftPanel.Collapsed = append(m.leftPanel.Collapsed, m.fileCollapsed(i))
		}
	}
}

// rebuildDiffLayout recomputes row positions and everything derived from
// them (cursor index, search rows). Call after the diff or any collapse
// state changes.
func (m *PRDetailModel) rebuildDiffLayout() {
	anchor, keep := m.diffViewAnchor()
	m.computeLayout()
	cursor := m.diffCursor
	m.buildNavigableIndex()
	m.setDiffCursor(cursor)
	m.normalizeDiffRows()
	if keep {
		m.restoreDiffViewAnchor(anchor)
	}
}

// diffViewAnchor is what the user is looking at, so a layout change (a file
// collapsing or expanding elsewhere) doesn't move it on screen: the cursor's
// distance from the top of the viewport, and the file at the top.
type diffViewAnchor struct {
	cursor       diffCursorLine
	cursorOffset int
	hasCursor    bool
	topFile      int
	topOffset    int
}

// diffScrollPos returns the Diff tab's scroll, which lives in ContentScroll
// only while the Diff tab is shown.
func (m *PRDetailModel) diffScrollPos() int {
	if m.activeTab == TabDiff {
		return m.ContentScroll
	}
	return m.diffScroll
}

func (m *PRDetailModel) setDiffScrollPos(row int) {
	if m.activeTab == TabDiff {
		m.ContentScroll = row
		m.clampContentScroll()
		return
	}
	m.diffScroll = max(row, 0)
}

// diffViewAnchor captures the anchor from the current (pre-change) layout.
// ok is false when there is no layout for this diff yet.
func (m *PRDetailModel) diffViewAnchor() (a diffViewAnchor, ok bool) {
	if m.Diff == nil || m.layoutFor != m.Diff || len(m.layout.fileStart) == 0 {
		return a, false
	}
	scroll := m.diffScrollPos()
	if m.validDiffCursor() {
		a.cursor, a.hasCursor = m.diffCursor, true
		a.cursorOffset = m.navigableRows[m.navIdx] - scroll
	}
	for i, start := range m.layout.fileStart {
		if start > scroll {
			break
		}
		a.topFile, a.topOffset = i, scroll-start
	}
	return a, true
}

// restoreDiffViewAnchor scrolls so the cursor (or, if its line is gone, the
// top file) sits where it was on screen.
func (m *PRDetailModel) restoreDiffViewAnchor(a diffViewAnchor) {
	if a.hasCursor {
		if idx, ok := m.navIdxMap[a.cursor]; ok {
			m.setDiffScrollPos(m.navigableRows[idx] - a.cursorOffset)
			return
		}
	}
	if a.topFile < len(m.layout.fileStart) {
		off := min(a.topOffset, max(m.layout.fileRows[a.topFile]-1, 0))
		m.setDiffScrollPos(m.layout.fileStart[a.topFile] + off)
	}
}

// setFileCollapsed collapses or expands file i, keeping the cursor on the
// same file. Too-large files never expand. Returns false when nothing changed.
func (m *PRDetailModel) setFileCollapsed(i int, collapsed bool) bool {
	if m.Diff == nil || i < 0 || i >= len(m.Diff.Files) || m.Diff.Files[i].IsBinary {
		return false
	}
	v := &m.views()[i]
	if v.collapsed == collapsed || (v.tooLarge && !collapsed) {
		return false
	}
	v.collapsed = collapsed
	if collapsed && v.reason == reasonNone {
		v.reason = reasonUser
	}
	m.collapseTouched = true
	if m.visual.Active && m.visual.FileIdx == i {
		m.visual.Active = false
	}
	if m.diffCursor.FileIdx == i {
		if collapsed {
			m.diffCursor = diffCursorLine{FileIdx: i, HunkIdx: placeholderLine, LineIdx: placeholderLine}
		} else if len(m.Diff.Files[i].Hunks) > 0 {
			m.diffCursor = diffCursorLine{FileIdx: i}
		}
	}
	m.rebuildDiffLayout()
	return true
}

// expandFileForJump expands file i before a jump lands on one of its lines.
// It returns false when the file is too large to show, so the caller should
// land on the placeholder instead.
func (m *PRDetailModel) expandFileForJump(i int) bool {
	if m.fileTooLarge(i) {
		return false
	}
	if m.fileCollapsed(i) {
		m.setFileCollapsed(i, false)
	}
	return true
}

// toggleCurrentFile handles `z` on the file under the diff cursor.
func (m *PRDetailModel) toggleCurrentFile() {
	m.ensureDiffCursor()
	fi := m.diffCursor.FileIdx
	if m.Diff == nil || fi < 0 || fi >= len(m.Diff.Files) {
		return
	}
	if m.fileTooLarge(fi) {
		m.diffNotice = "Too large to show here · O: open on GitHub"
		return
	}
	m.setFileCollapsed(fi, !m.fileCollapsed(fi))
	m.scrollToCursor(scrollPadding)
}

// toggleAllFiles handles `Z`: expand everything that can expand, or, when
// nothing is collapsed, collapse every file.
func (m *PRDetailModel) toggleAllFiles() {
	if m.Diff == nil {
		return
	}
	anyExpandable := false
	for i := range m.Diff.Files {
		if m.fileCollapsed(i) && !m.fileTooLarge(i) {
			anyExpandable = true
			break
		}
	}
	for i := range m.views() {
		v := &m.fileViews[i]
		if m.Diff.Files[i].IsBinary || v.tooLarge {
			continue
		}
		v.collapsed = !anyExpandable
		if v.collapsed && v.reason == reasonNone {
			v.reason = reasonUser
		}
	}
	m.collapseTouched = true
	m.visual.Active = false
	if fi := m.diffCursor.FileIdx; fi >= 0 && fi < len(m.Diff.Files) {
		if m.fileCollapsed(fi) {
			m.diffCursor = diffCursorLine{FileIdx: fi, HunkIdx: placeholderLine, LineIdx: placeholderLine}
		} else if m.diffCursor.isPlaceholder() {
			m.diffCursor = diffCursorLine{FileIdx: fi}
		}
	}
	m.rebuildDiffLayout()
	m.scrollToCursor(scrollPadding)
}

// collapsedPlaceholder renders a collapsed file's single body row.
func (m *PRDetailModel) collapsedPlaceholder(i int) string {
	f := &m.Diff.Files[i]
	v := fileView{collapsed: true}
	if vs := m.views(); i < len(vs) {
		v = vs[i]
	}
	lines := diffFileLineCount(f)
	if f.PatchUnavailable || lines == 0 {
		lines = f.Additions + f.Deletions
	}
	count := formatThousands(lines) + " lines"
	parts := []string{"▸ " + count + " hidden"}
	if v.tooLarge {
		parts = []string{"▸ " + count, "too large to show here"}
		if v.reason == reasonNoPatch {
			parts[1] = "GitHub sent no diff for this file"
		}
	} else if label := v.reason.label(); label != "" && v.reason != reasonUser {
		parts = append(parts, label)
	}
	if !v.tooLarge {
		parts = append(parts, "z expand")
	}
	parts = append(parts, "O open on GitHub")
	return "      " + strings.Join(parts, " · ")
}

func formatThousands(n int) string {
	s := fmt.Sprint(n)
	if n < 0 || len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// emitOpenBrowserFile opens the file under the diff cursor on GitHub.
func (m *PRDetailModel) emitOpenBrowserFile() tea.Cmd {
	m.ensureDiffCursor()
	fi := m.diffCursor.FileIdx
	if m.Diff == nil || fi < 0 || fi >= len(m.Diff.Files) {
		return nil
	}
	f := &m.Diff.Files[fi]
	p := f.NewPath
	if f.Status == "removed" {
		p = f.OldPath
	}
	msg := OpenBrowserFile{Repo: m.Summary.Repo, Number: m.Summary.Number, Path: p}
	if m.CommitMode {
		msg.CommitRepo, msg.CommitSHA = m.Repo, m.Commit.SHA
	}
	return func() tea.Msg { return msg }
}
