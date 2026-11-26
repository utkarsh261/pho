package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

func (m *Model) searchTab(id domain.DashboardTab) (domain.SearchTab, bool) {
	for _, t := range m.deps.SearchTabs {
		if t.ID == id {
			return t, true
		}
	}
	return domain.SearchTab{}, false
}

// loadActiveSearchTabCmd loads the active tab when it's a custom one. Custom
// tabs load only while shown, so extra tabs cost nothing until opened; a
// cache hit makes reopening one free.
func (m *Model) loadActiveSearchTabCmd(force bool) tea.Cmd {
	tab, ok := m.searchTab(m.state.Dashboard.ActiveTab)
	repo, hasRepo := m.selectedRepo()
	if !ok || !hasRepo || m.deps.Dashboard == nil {
		return nil
	}
	key := jobKey(repo.FullName, string(tab.ID))
	if m.state.Jobs.InFlight[key] {
		return nil
	}
	m.state.Jobs.InFlight[key] = true
	m.prList.SetTabStatus(tab.ID, true, "")
	m.syncStatus()
	return cmds.LoadSearchTabCmd(m.deps.Dashboard, repo, tab, force)
}

// clearSearchTabs empties every custom tab, so a repo switch never shows
// the previous repo's results while the new ones load.
func (m *Model) clearSearchTabs() {
	clear(m.searchResults)
	for _, t := range m.deps.SearchTabs {
		delete(m.state.Dashboard.PRsByTab, t.ID)
		delete(m.state.Dashboard.FreshnessByTab, t.ID)
		m.prList.SetTabSnapshot(t.ID, nil, 0, false)
	}
}

func (m *Model) handleSearchTabLoaded(msg cmds.SearchTabLoaded) tea.Cmd {
	delete(m.state.Jobs.InFlight, jobKey(msg.Repo, string(msg.Tab)))
	repo, ok := m.selectedRepo()
	if !ok || !sameRepo(repo.FullName, msg.Repo) {
		m.syncStatus()
		return nil
	}
	if msg.Err != nil {
		m.logError("search tab load failed", "repo", msg.Repo, "tab", string(msg.Tab), "err", msg.Err)
		m.recordError(domain.ErrorKindNetwork, msg.Err, msg.Repo)
		if msg.Snapshot.FetchedAt.IsZero() {
			m.prList.SetTabStatus(msg.Tab, false, shortError(msg.Err))
			m.syncStatus()
			return nil
		}
	} else {
		m.clearErrors()
	}

	prev, hadPrev := m.currentSelectedPR()
	if m.searchResults == nil {
		m.searchResults = map[domain.DashboardTab]domain.SearchTabSnapshot{}
	}
	m.searchResults[msg.Tab] = msg.Snapshot
	m.regroupSearchTab(msg.Tab, msg.Snapshot)
	m.state.Dashboard.LastRefreshAt[msg.Tab] = msg.Snapshot.FetchedAt
	m.state.Dashboard.FreshnessByTab[msg.Tab] = freshnessFor(msg.Err)
	m.logDebug("search tab loaded", "tab", string(msg.Tab), "pr_count", len(msg.Snapshot.PRs))
	m.syncStatus()
	if msg.Tab != m.state.Dashboard.ActiveTab {
		return nil
	}
	m.restoreSelection(prev, hadPrev)
	return m.syncCurrentSelection()
}

// regroupSearchTabs lists every loaded custom tab against the current
// stacks, as regroupInvolving does for Involving.
func (m *Model) regroupSearchTabs() {
	for tab, snap := range m.searchResults {
		m.regroupSearchTab(tab, snap)
	}
}

func (m *Model) regroupSearchTab(tab domain.DashboardTab, snap domain.SearchTabSnapshot) {
	prs := m.groupStacks(snap.PRs)
	m.state.Dashboard.PRsByTab[tab] = prs
	m.prList.SetTabSnapshot(tab, prs, snap.TotalCount, snap.Truncated)
}

// shortError keeps the first line of err, short enough for the list panel.
func shortError(err error) string {
	s, _, _ := strings.Cut(err.Error(), "\n")
	if r := []rune(s); len(r) > 80 {
		s = string(r[:79]) + "…"
	}
	return s
}
