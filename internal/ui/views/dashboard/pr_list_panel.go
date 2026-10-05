package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/timefmt"
)

type tabSnapshot struct {
	PRs        []domain.PullRequestSummary
	TotalCount int
	Truncated  bool
	// Scanned is how many open PRs a filtered tab was classified from.
	Scanned int
}

// PageStatus is the lazy-loading state of the All tab.
type PageStatus struct {
	Loading bool
	Failed  bool
	Capped  bool
}

type PRListPanelModel struct {
	Tabs    map[domain.DashboardTab]tabSnapshot
	Paging  PageStatus
	Active  domain.DashboardTab
	Cursor  int
	Scroll  int
	Width   int
	Height  int
	theme   *theme.Theme
	lastKey string
}

func NewPRListPanelModel() *PRListPanelModel {
	return &PRListPanelModel{
		Tabs:   make(map[domain.DashboardTab]tabSnapshot),
		Active: domain.TabMyPRs,
	}
}

func (m *PRListPanelModel) Init() tea.Cmd { return nil }

func (m *PRListPanelModel) SetRect(width, height int) {
	m.Width = width
	m.Height = height
	m.ensureVisible()
}

func (m *PRListPanelModel) SetTheme(th *theme.Theme) {
	m.theme = th
}

func (m *PRListPanelModel) SetTabSnapshot(tab domain.DashboardTab, prs []domain.PullRequestSummary, totalCount int, truncated bool) {
	if m.Tabs == nil {
		m.Tabs = make(map[domain.DashboardTab]tabSnapshot)
	}
	m.Tabs[tab] = tabSnapshot{
		PRs:        append([]domain.PullRequestSummary(nil), prs...),
		TotalCount: totalCount,
		Truncated:  truncated,
	}
	m.ensureVisible()
}

// SetTabScanned records how many open PRs a filtered tab was built from, so
// its footer can say the list may be incomplete.
func (m *PRListPanelModel) SetTabScanned(tab domain.DashboardTab, scanned int) {
	if m.Tabs == nil {
		return
	}
	snap := m.Tabs[tab]
	snap.Scanned = scanned
	m.Tabs[tab] = snap
}

// SelectNumber moves the cursor to the PR with this number in the active tab.
// Returns false (cursor unchanged) when the PR isn't in the list.
func (m *PRListPanelModel) SelectNumber(number int) bool {
	for i, pr := range m.currentPRs() {
		if pr.Number == number {
			m.Cursor = i
			m.ensureVisible()
			return true
		}
	}
	return false
}

// VisibleItemCount is how many PR rows fit in the panel.
func (m *PRListPanelModel) VisibleItemCount() int {
	return m.visibleItemCount()
}

func (m *PRListPanelModel) SetActiveTab(tab domain.DashboardTab) {
	m.Active = tab
	m.Cursor = 0
	m.Scroll = 0
	m.ensureVisible()
}

func (m *PRListPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetRect(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		prevKey := m.lastKey
		if msg.String() != "g" {
			m.lastKey = ""
		}
		switch msg.String() {
		case "j", "down":
			if m.moveCursor(1) {
				return m, m.selectCurrentCmd()
			}
		case "k", "up":
			if m.moveCursor(-1) {
				return m, m.selectCurrentCmd()
			}
		case "g":
			if prevKey == "g" {
				if m.moveCursorTo(0) {
					return m, m.selectCurrentCmd()
				}
			} else {
				m.lastKey = "g"
			}
			return m, nil
		case "G":
			prs := m.currentPRs()
			if m.moveCursorTo(len(prs) - 1) {
				return m, m.selectCurrentCmd()
			}
			return m, nil
		case "ctrl+d":
			if m.moveCursor(m.visibleItemCount() / 2) {
				return m, m.selectCurrentCmd()
			}
			return m, nil
		case "ctrl+u":
			if m.moveCursor(-(m.visibleItemCount() / 2)) {
				return m, m.selectCurrentCmd()
			}
			return m, nil
		case "h", "left":
			next := nextTab(m.Active, -1)
			if next != m.Active {
				m.Active = next
				m.Cursor = 0
				m.Scroll = 0
				m.ensureVisible()
				return m, changeTabCmd(next)
			}
		case "l", "right":
			next := nextTab(m.Active, 1)
			if next != m.Active {
				m.Active = next
				m.Cursor = 0
				m.Scroll = 0
				m.ensureVisible()
				return m, changeTabCmd(next)
			}
		case "enter":
			if cmd := m.selectCurrentCmd(); cmd != nil {
				return m, cmd
			}
		}
	case SelectRepoMsg:
		return m, nil
	}
	return m, nil
}

func (m *PRListPanelModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}
	header := " Pull requests"
	if m.theme != nil {
		header = m.theme.Header.Render(header)
	}
	header = fitLine(header, m.Width)
	lines := []string{
		header,
		fitLine("", m.Width),
		fitLine(m.renderTabBarThemed(), m.Width),
		fitLine("", m.Width),
	}
	rows := m.visibleRows()
	if len(rows) == 0 {
		empty := "  No pull requests here"
		if m.theme != nil {
			empty = m.theme.MutedTxt.Render(empty)
		}
		lines = append(lines, fitLine(empty, m.Width))
		lines = append(lines, fitLine("", m.Width))
		return renderBlock(lines, m.Width, m.Height)
	}
	for i, row := range rows {
		lines = append(lines, fitLine(row.line1, m.Width))
		lines = append(lines, fitLine(row.line2, m.Width))
		if i < len(rows)-1 {
			lines = append(lines, fitLine("", m.Width))
		}
	}
	if footer := m.footerLine(); footer != "" {
		lines = append(lines, fitLine("", m.Width))
		lines = append(lines, fitLine(footer, m.Width))
	} else {
		lines = append(lines, fitLine("", m.Width))
	}
	return renderBlock(lines, m.Width, m.Height)
}

func (m *PRListPanelModel) moveCursor(delta int) bool {
	prs := m.currentPRs()
	if len(prs) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return false
	}
	next := m.Cursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(prs) {
		next = len(prs) - 1
	}
	changed := next != m.Cursor
	m.Cursor = next
	m.ensureVisible()
	return changed
}

func (m *PRListPanelModel) moveCursorTo(pos int) bool {
	prs := m.currentPRs()
	if len(prs) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return false
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(prs) {
		pos = len(prs) - 1
	}
	changed := pos != m.Cursor
	m.Cursor = pos
	m.ensureVisible()
	return changed
}

func (m *PRListPanelModel) currentSnapshot() tabSnapshot {
	if m.Tabs == nil {
		return tabSnapshot{}
	}
	snap, ok := m.Tabs[m.Active]
	if !ok {
		return tabSnapshot{}
	}
	return snap
}

func (m *PRListPanelModel) currentPRs() []domain.PullRequestSummary {
	return m.currentSnapshot().PRs
}

func (m *PRListPanelModel) currentSelected() (domain.PullRequestSummary, bool) {
	prs := m.currentPRs()
	if len(prs) == 0 || m.Cursor < 0 || m.Cursor >= len(prs) {
		return domain.PullRequestSummary{}, false
	}
	return prs[m.Cursor], true
}

func (m *PRListPanelModel) selectCurrentCmd() tea.Cmd {
	pr, ok := m.currentSelected()
	if !ok {
		return nil
	}
	return selectPRCmd(m.Active, m.Cursor, pr)
}

func (m *PRListPanelModel) visibleRows() []prRow {
	prs := m.currentPRs()
	if len(prs) == 0 || m.Width <= 0 || m.Height <= 0 {
		return nil
	}
	maxRows := m.visibleItemCount()
	if maxRows <= 0 {
		return nil
	}
	start := m.Scroll
	if start < 0 {
		start = 0
	}
	if start > len(prs) {
		start = len(prs)
	}
	end := start + maxRows
	if end > len(prs) {
		end = len(prs)
	}
	rows := make([]prRow, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, m.renderRow(prs[i], i))
	}
	return rows
}

func (m *PRListPanelModel) renderRow(pr domain.PullRequestSummary, index int) prRow {
	selected := index == m.Cursor
	if m.theme == nil {
		return m.renderRowPlain(pr, selected)
	}
	th := m.theme

	edge := " "
	if selected {
		edge = th.PrimaryTxt.Render("▎")
	}
	glyph := m.stateGlyph(pr)
	num := th.Number.Render(fmt.Sprintf("#%d", pr.Number))
	meta := m.ciIconStyled(pr.CIStatus) + " " + m.reviewIconStyled(pr.ReviewDecision, pr.IsDraft)

	prefixW := 1 + 1 + 1 + lipgloss.Width(num) + 1 // edge glyph sp num sp
	metaW := lipgloss.Width(meta) + 1              // trailing pad
	titleMax := max(m.Width-prefixW-metaW-2, 1)
	titleStyle := lipgloss.NewStyle().Foreground(th.Text)
	if selected {
		titleStyle = titleStyle.Bold(true).Foreground(th.TextBright)
	}
	title := titleStyle.Render(truncateText(pr.Title, titleMax))
	left := edge + glyph + " " + num + " " + title
	gap := max(m.Width-lipgloss.Width(left)-metaW, 1)
	line1 := left + strings.Repeat(" ", gap) + meta + " "

	branch := pr.HeadRefName
	if branch == "" {
		branch = pr.BaseRefName
	}
	var sub []string
	if branch != "" {
		sub = append(sub, branch)
	}
	if pr.Author != "" {
		sub = append(sub, pr.Author)
	}
	if age := timefmt.Relative(pr.UpdatedAt, time.Now()); age != "" {
		sub = append(sub, age)
	}
	line2 := edge + "   " + th.MutedTxt.Render(strings.Join(sub, " · "))

	if selected {
		line1 = theme.FillBg(th.Highlight, m.Width, line1)
		line2 = theme.FillBg(th.Highlight, m.Width, line2)
	}
	return prRow{line1: line1, line2: line2}
}

func (m *PRListPanelModel) renderRowPlain(pr domain.PullRequestSummary, selected bool) prRow {
	bar := " "
	if selected {
		bar = "▌"
	}
	meta := fmt.Sprintf("%s %s", ciIcon(pr.CIStatus), reviewIcon(pr.ReviewDecision, pr.IsDraft))
	prefix := m.prNumberStyled(pr.Number)
	titleMax := max(m.Width-lipgloss.Width(bar)-lipgloss.Width(prefix)-1-lipgloss.Width(meta)-2, 1)
	line1 := fmt.Sprintf("%s%s %s  %s", bar, prefix, truncateText(pr.Title, titleMax), meta)
	branch := pr.HeadRefName
	if branch == "" {
		branch = pr.BaseRefName
	}
	return prRow{line1: line1, line2: strings.TrimRight(bar+" "+branch, " ")}
}

// stateGlyph renders a coloured dot for the PR's state.
func (m *PRListPanelModel) stateGlyph(pr domain.PullRequestSummary) string {
	c := m.theme.StateOpen
	switch {
	case pr.IsDraft:
		c = m.theme.StateDraft
	case pr.State == domain.PRStateMerged:
		c = m.theme.StateMerged
	case pr.State == domain.PRStateClosed:
		c = m.theme.StateClosed
	}
	return lipgloss.NewStyle().Foreground(c).Render("●")
}

func (m *PRListPanelModel) renderTabBar() string {
	parts := make([]string, 0, len(dashboardTabOrder))
	for _, tab := range dashboardTabOrder {
		count := len(m.currentSnapshotFor(tab).PRs)
		label := fmt.Sprintf("%s(%d)", tabLabel(tab), count)
		if tab == m.Active {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " | ")
}

// tabBarTier controls how much of the tab bar is shown; higher tiers are
// more compact and are used when the panel is too narrow.
type tabBarTier int

const (
	tabBarFull       tabBarTier = iota // full labels, every count
	tabBarShort                        // short labels, every count
	tabBarActiveOnly                   // short labels, count on the active tab
	tabBarSingle                       // only the active tab, with position
)

// renderTabBarThemed picks the widest tab bar tier that fits the panel, so
// the active tab is never clipped off the end.
func (m *PRListPanelModel) renderTabBarThemed() string {
	if m.theme == nil {
		return m.renderTabBar()
	}
	for _, tier := range []tabBarTier{tabBarFull, tabBarShort, tabBarActiveOnly} {
		if bar := m.renderTabBarTier(tier); lipgloss.Width(bar) <= m.Width {
			return bar
		}
	}
	return m.renderTabBarTier(tabBarSingle)
}

func (m *PRListPanelModel) renderTabBarTier(tier tabBarTier) string {
	th := m.theme
	if tier == tabBarSingle {
		idx := max(indexOfTab(m.Active), 0)
		active := th.TabActive.Render(fmt.Sprintf("%s %d", tabShortLabel(m.Active), m.tabCount(m.Active)))
		pos := th.FaintTxt.Render(fmt.Sprintf("%d/%d", idx+1, len(dashboardTabOrder)))
		return th.FaintTxt.Render("‹") + active + th.FaintTxt.Render("›") + " " + pos
	}
	parts := make([]string, 0, 2*len(dashboardTabOrder))
	for i, tab := range dashboardTabOrder {
		label := tabLabel(tab)
		if tier != tabBarFull {
			label = tabShortLabel(tab)
		}
		count := m.tabCount(tab)
		if tab == m.Active {
			parts = append(parts, th.TabActive.Render(fmt.Sprintf("%s %d", label, count)))
		} else if tier == tabBarActiveOnly {
			parts = append(parts, th.TabInactive.Render(label))
		} else {
			parts = append(parts, th.TabInactive.Render(label+" "+th.FaintTxt.Render(fmt.Sprint(count))))
		}
		if i < len(dashboardTabOrder)-1 {
			parts = append(parts, " ")
		}
	}
	return strings.Join(parts, "")
}

// tabCount is the number shown next to a tab label. The All tab shows the
// repo's open-PR total rather than how many pages have loaded so far.
func (m *PRListPanelModel) tabCount(tab domain.DashboardTab) int {
	snap := m.currentSnapshotFor(tab)
	if tab == domain.TabAll && snap.TotalCount > len(snap.PRs) {
		return snap.TotalCount
	}
	return len(snap.PRs)
}

func (m *PRListPanelModel) ciIconStyled(status domain.CIStatus) string {
	icon := ciIcon(status)
	if m.theme == nil {
		return icon
	}
	switch status {
	case domain.CIStatusSuccess:
		return m.theme.CISuccess.Render(icon)
	case domain.CIStatusFailure, domain.CIStatusError:
		return m.theme.CIFailure.Render(icon)
	case domain.CIStatusPending:
		return m.theme.CIPending.Render(icon)
	default:
		return m.theme.CIMuted.Render(icon)
	}
}

func (m *PRListPanelModel) reviewIconStyled(decision domain.ReviewDecision, isDraft bool) string {
	icon := reviewIcon(decision, isDraft)
	if m.theme == nil {
		return icon
	}
	if isDraft {
		return m.theme.ReviewDraft.Render(icon)
	}
	switch decision {
	case domain.ReviewDecisionApproved:
		return m.theme.ReviewApproved.Render(icon)
	case domain.ReviewDecisionChangesRequested:
		return m.theme.ReviewChanges.Render(icon)
	case domain.ReviewDecisionReviewRequired:
		return m.theme.ReviewRequired.Render(icon)
	default:
		return m.theme.ReviewMuted.Render(icon)
	}
}

func (m *PRListPanelModel) prNumberStyled(number int) string {
	s := fmt.Sprintf("#%d ", number)
	if m.theme != nil {
		return m.theme.Number.Render(s)
	}
	return s
}

func (m *PRListPanelModel) currentSnapshotFor(tab domain.DashboardTab) tabSnapshot {
	if m.Tabs == nil {
		return tabSnapshot{}
	}
	return m.Tabs[tab]
}

func (m *PRListPanelModel) footerLine() string {
	snap := m.currentSnapshot()
	if !snap.Truncated {
		return ""
	}
	total := max(snap.TotalCount, len(snap.PRs))
	var line string
	switch m.Active {
	case domain.TabAll:
		line = fmt.Sprintf("%d of %d open", len(snap.PRs), total)
		switch {
		case m.Paging.Failed:
			line += " · couldn't load more · R to retry"
		case m.Paging.Loading:
			line += " · loading…"
		case m.Paging.Capped:
			line += " · limit reached · search the palette for more"
		default:
			line += " · ↓ for more"
		}
	case domain.TabMyPRs, domain.TabNeedsReview:
		// Filtered tabs are classified from the loaded open PRs only.
		line = fmt.Sprintf("%d from newest %d of %d open", len(snap.PRs), max(snap.Scanned, len(snap.PRs)), total)
	default:
		line = fmt.Sprintf("Showing %d of %d open PRs", len(snap.PRs), total)
	}
	if m.theme != nil {
		return " " + m.theme.MutedTxt.Render(line)
	}
	return line
}

func (m *PRListPanelModel) visibleItemCount() int {
	if m.Height <= 6 {
		return 0
	}
	available := m.Height - 6
	if available < 2 {
		return 0
	}
	return (available + 1) / 3
}

func (m *PRListPanelModel) ensureVisible() {
	prs := m.currentPRs()
	if len(prs) == 0 {
		m.Cursor = 0
		m.Scroll = 0
		return
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(prs) {
		m.Cursor = len(prs) - 1
	}
	visible := m.visibleItemCount()
	if visible <= 0 {
		m.Scroll = 0
		return
	}
	if m.Cursor < m.Scroll {
		m.Scroll = m.Cursor
	}
	if m.Cursor >= m.Scroll+visible {
		m.Scroll = m.Cursor - visible + 1
	}
	maxScroll := len(prs) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.Scroll > maxScroll {
		m.Scroll = maxScroll
	}
	if m.Scroll < 0 {
		m.Scroll = 0
	}
}

type prRow struct {
	line1 string
	line2 string
}
