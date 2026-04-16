package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	achdashboard "github.com/utkarsh261/pho/internal/application/dashboard"
	"github.com/utkarsh261/pho/internal/domain"
	gitlog "github.com/utkarsh261/pho/internal/log"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
	"github.com/utkarsh261/pho/internal/ui/keymap"
	"github.com/utkarsh261/pho/internal/ui/layout"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

// SearchService combines the interactive palette search and index rebuild APIs.
type SearchService interface {
	overlay.SearchService
	cmds.SearchService
}

// Dependencies wires the root model to application services.
type Dependencies struct {
	Viewer    cmds.ViewerService
	Discovery cmds.DiscoveryService
	Dashboard cmds.DashboardService
	Search    SearchService
	PR        cmds.PRService

	Root string
	Host string

	Classifier achdashboard.SummaryTabClassifier
	Now        func() time.Time

	Logger *gitlog.Logger
}

type Model struct {
	deps Dependencies

	log   *gitlog.Logger
	state domain.AppState

	// viewStack tracks the stack of base views. Top element is current view.
	// Always has at least one element (PrimaryViewDashboard).
	viewStack []domain.PrimaryView

	// prDetail holds the PR detail sub-model when the top view is PR detail.
	prDetail *prdetail.PRDetailModel

	layout layout.LayoutState

	repoPanel *dashboard.RepoPanelModel
	prList    *dashboard.PRListPanelModel
	preview   *dashboard.PreviewPanelModel
	status    *dashboard.StatusBarModel
	palette   overlay.Model

	focus         domain.FocusTarget
	previousFocus domain.FocusTarget

	classifier achdashboard.SummaryTabClassifier

	currentDashboard domain.DashboardSnapshot
	currentInvolving domain.InvolvingSnapshot
	currentRecent    domain.RecentSnapshot

	hydratedRepos map[string]struct{}
	theme         *theme.Theme
}

var dashboardFocusCycle = []domain.FocusTarget{
	domain.FocusRepoPanel,
	domain.FocusPRListPanel,
	domain.FocusPreviewPanel,
}

// NewModel builds the root model with empty state and wired child views.
func NewModel(deps Dependencies) *Model {
	nowFn := deps.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	classifier := deps.Classifier
	if classifier == nil {
		classifier = achdashboard.DefaultSummaryTabClassifier{}
	}

	if deps.Host == "" {
		deps.Host = "github.com"
	}

	log := deps.Logger
	if log == nil {
		log = gitlog.NewNop()
	}

	m := &Model{
		deps:          deps,
		log:           log,
		classifier:    classifier,
		hydratedRepos: map[string]struct{}{},
		layout:        layout.NewLayoutState(0, 0),
		viewStack:     []domain.PrimaryView{domain.PrimaryViewDashboard},
		repoPanel:     dashboard.NewRepoPanelModel(nil),
		prList:        dashboard.NewPRListPanelModel(),
		preview:       dashboard.NewPreviewPanelModel(),
		status:        dashboard.NewStatusBarModel(),
		palette:       overlay.NewModel(deps.Search),
		focus:         domain.FocusRepoPanel,
		previousFocus: domain.FocusRepoPanel,
		state: domain.AppState{
			Session: domain.SessionState{
				ActiveHost: deps.Host,
				StartedAt:  nowFn().UTC(),
			},
			Repos: domain.RepoState{
				SelectedIndex: -1,
				CursorIndex:   0,
			},
			Dashboard: domain.DashboardState{
				ActiveTab:      domain.TabMyPRs,
				PRsByTab:       make(map[domain.DashboardTab][]domain.PullRequestSummary),
				LastRefreshAt:  make(map[domain.DashboardTab]time.Time),
				FreshnessByTab: make(map[domain.DashboardTab]domain.Freshness),
			},
			Search: domain.SearchState{
				Mode: domain.SearchModePRs,
			},
			Jobs: domain.JobState{
				InFlight: make(map[string]bool),
			},
		},
	}

	m.syncPanels()
	m.syncStatus()
	m.syncPaletteStats()
	return m
}

// SetTheme applies a theme to all child panel models.
func (m *Model) SetTheme(th *theme.Theme) {
	m.theme = th
	m.repoPanel.SetTheme(th)
	m.prList.SetTheme(th)
	m.preview.SetTheme(th)
	m.status.SetTheme(th)
	m.palette.SetTheme(th)
}

// SetFocus changes the focused panel.
func (m *Model) SetFocus(f domain.FocusTarget) {
	m.previousFocus = m.focus
	m.focus = f
	m.syncStatus()
}

// Init kicks off startup discovery and auth bootstrap.
func (m *Model) Init() tea.Cmd {
	var cmdsOut []tea.Cmd

	m.logInfo("starting app", "root", m.deps.Root, "host", m.selectedHost())

	if m.deps.Viewer != nil && strings.TrimSpace(m.selectedHost()) != "" {
		cmdsOut = append(cmdsOut, cmds.ResolveViewerCmd(m.deps.Viewer, m.selectedHost()))
	}
	if m.deps.Discovery != nil {
		root := m.deps.Root
		if strings.TrimSpace(root) == "" {
			root = "."
		}
		m.logInfo("starting repo discovery", "root", root)
		cmdsOut = append(cmdsOut, cmds.DiscoverReposCmd(m.deps.Discovery, root))
	}
	if repo, ok := m.selectedRepo(); ok {
		cmdsOut = append(cmdsOut, m.loadRepoCmds(repo, false)...)
	}

	// Start spinner animations.
	cmdsOut = append(cmdsOut, m.preview.Init(), m.status.Init())

	return batch(cmdsOut...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applyWindowSize(msg)
		// Also forward window size to PR detail if active.
		if m.currentView() == domain.PrimaryViewPRDetail && m.prDetail != nil {
			m.prDetail.Width = m.layout.Current.Width
			m.prDetail.Height = m.layout.Current.Height - 2
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case prdetail.BackToDashboard:
		return m, m.handleBackToDashboard()
	case prdetail.OpenBrowserPR:
		return m, openBrowserForPRCmd(m.selectedRepoForURL(msg.Repo), msg.Repo, msg.Number)
	case cmds.PRDetailLoaded:
		// Always route to prDetail if it exists, even when the dashboard is the
		// active view. If the user ESCs before an in-flight response arrives, the
		// data is still stored so same-PR reuse shows it immediately on reopen.
		if m.prDetail != nil {
			next, cmd := m.prDetail.Update(msg)
			m.prDetail = next
			return m, cmd
		}
		return m, nil
	case cmds.DiffLoaded:
		// Same rationale as PRDetailLoaded above.
		if m.prDetail != nil {
			next, cmd := m.prDetail.Update(msg)
			m.prDetail = next
			return m, cmd
		}
		return m, nil
	default:
		// Always route unknown messages through applyMessage so dashboard state
		// (preview debounce, background loads) continues updating regardless of
		// which view is active. PR-detail-specific messages (PRDetailLoaded,
		// DiffLoaded) have explicit cases above.
		return m, m.applyMessage(msg)
	}
}

func (m *Model) View() string {
	if m.state.Search.OverlayOpen {
		return m.palette.View()
	}
	switch m.currentView() {
	case domain.PrimaryViewPRDetail:
		if m.prDetail != nil {
			// PR detail renders content in a rect of width × (height - 2).
			// The "- 2" leaves exactly 2 rows for the shared status bar at the bottom.
			// This matches composeBody's contentH = height - 2.
			body := m.prDetail.View()
			status := m.status.View()
			if strings.TrimSpace(body) == "" {
				return status
			}
			if strings.TrimSpace(status) == "" {
				return body
			}
			return body + "\n" + status
		}
		return m.renderDashboard()
	default:
		return m.renderDashboard()
	}
}

func (m *Model) State() domain.AppState {
	copied := m.state
	copied.Repos.Discovered = append([]domain.Repository(nil), m.state.Repos.Discovered...)
	if m.state.Repos.SelectedRepo != nil {
		repo := *m.state.Repos.SelectedRepo
		copied.Repos.SelectedRepo = &repo
	}
	if m.state.Dashboard.PRsByTab != nil {
		copied.Dashboard.PRsByTab = make(map[domain.DashboardTab][]domain.PullRequestSummary, len(m.state.Dashboard.PRsByTab))
		for tab, prs := range m.state.Dashboard.PRsByTab {
			copied.Dashboard.PRsByTab[tab] = append([]domain.PullRequestSummary(nil), prs...)
		}
	}
	copied.Dashboard.RecentItems = append([]domain.ActivityItem(nil), m.state.Dashboard.RecentItems...)
	if m.state.Dashboard.Preview != nil {
		preview := *m.state.Dashboard.Preview
		copied.Dashboard.Preview = &preview
	}
	if m.state.Dashboard.LastRefreshAt != nil {
		copied.Dashboard.LastRefreshAt = make(map[domain.DashboardTab]time.Time, len(m.state.Dashboard.LastRefreshAt))
		for tab, ts := range m.state.Dashboard.LastRefreshAt {
			copied.Dashboard.LastRefreshAt[tab] = ts
		}
	}
	if m.state.Dashboard.FreshnessByTab != nil {
		copied.Dashboard.FreshnessByTab = make(map[domain.DashboardTab]domain.Freshness, len(m.state.Dashboard.FreshnessByTab))
		for tab, freshness := range m.state.Dashboard.FreshnessByTab {
			copied.Dashboard.FreshnessByTab[tab] = freshness
		}
	}
	copied.Search.Results = append([]domain.SearchResult(nil), m.state.Search.Results...)
	if m.state.Jobs.InFlight != nil {
		copied.Jobs.InFlight = make(map[string]bool, len(m.state.Jobs.InFlight))
		for key, inFlight := range m.state.Jobs.InFlight {
			copied.Jobs.InFlight[key] = inFlight
		}
	}
	copied.Errors.Errors = append([]domain.AppError(nil), m.state.Errors.Errors...)
	if m.state.Errors.RateLimitReset != nil {
		reset := *m.state.Errors.RateLimitReset
		copied.Errors.RateLimitReset = &reset
	}
	return copied
}

func (m *Model) Layout() layout.LayoutState {
	return m.layout
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.logDebug("key", "key", msg.String(), "view", string(m.currentView()), "focus", string(m.focus))

	// keymap.Dispatch is a dashboard-scoped concern. It understands the four
	// dashboard focus targets (repo panel, PR list, preview, command palette)
	// and maps keys to typed Actions that the root model then executes.
	//
	// PR detail is a separate view pushed onto the view stack. It has its own
	// internal focus axis (Files / CI / Content) that does not map to any
	// domain.FocusTarget, and it reuses keys that the dashboard keymap assigns
	// different meanings (e.g. esc → Quit on dashboard, esc → BackToDashboard
	// in PR detail; tab → CycleFocus on dashboard, tab → cycle sub-panels in
	// PR detail). Routing those keys through keymap.Dispatch would produce the
	// wrong actions.
	//
	// PRDetailModel.Update already contains complete, self-contained key
	// handling, so the right move is to forward directly and skip the
	// dashboard dispatcher altogether.
	if m.currentView() == domain.PrimaryViewPRDetail {
		return m, m.forwardKey(msg)
	}

	result := keymap.Dispatch(m.focus, msg)
	if isRootAction(result.Action) {
		return m, m.handleRootAction(result.Action)
	}
	return m, m.forwardKey(msg)
}

func (m *Model) handleRootAction(action keymap.Action) tea.Cmd {
	switch a := action.(type) {
	case keymap.ToggleCmdPalette:
		return m.togglePalette()
	case keymap.CloseCmdPalette:
		return m.closePalette()
	case keymap.CycleFocus:
		return m.cycleFocus(a.Direction)
	case keymap.TriggerRefresh:
		return m.refreshSelectedRepo(true)
	case keymap.OpenBrowser:
		return m.openBrowserForCurrentPR()
	case keymap.SelectPR:
		// Enter on PR list: select the PR and open detail.
		return m.handleSelectPRAndOpenDetail()
	case keymap.OpenPRDetail:
		return m.openPRDetail()
	case keymap.Quit:
		return tea.Quit
	case keymap.OpenDashboardFilter:
		return nil
	default:
		return nil
	}
}

func (m *Model) forwardKey(msg tea.KeyMsg) tea.Cmd {
	if m.currentView() == domain.PrimaryViewPRDetail {
		if m.prDetail != nil {
			next, cmd := m.prDetail.Update(msg)
			m.prDetail = next
			return cmd
		}
		return nil
	}

	switch m.focus {
	case domain.FocusCmdPalette:
		next, cmd := m.palette.Update(msg)
		m.palette = next
		m.syncPaletteState()
		return cmd
	case domain.FocusRepoPanel:
		if next, cmd := m.repoPanel.Update(msg); next != nil {
			if panel, ok := next.(*dashboard.RepoPanelModel); ok {
				m.repoPanel = panel
			}
			m.state.Repos.CursorIndex = m.repoPanel.Cursor
			m.syncStatus()
			return cmd
		}
		return nil
	case domain.FocusPRListPanel:
		if next, cmd := m.prList.Update(msg); next != nil {
			if panel, ok := next.(*dashboard.PRListPanelModel); ok {
				m.prList = panel
			}
			m.state.Dashboard.ActiveTab = m.prList.Active
			m.state.Dashboard.SelectedIndex = m.prList.Cursor
			m.syncStatus()
			return cmd
		}
		return nil
	case domain.FocusPreviewPanel:
		if next, cmd := m.preview.Update(msg); next != nil {
			if panel, ok := next.(*dashboard.PreviewPanelModel); ok {
				m.preview = panel
			}
			m.state.Dashboard.PreviewLoading = m.preview.Loading || m.preview.PendingFetch
			m.syncStatus()
			return cmd
		}
		return nil
	default:
		return nil
	}
}

func (m *Model) applyMessage(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case cmds.ViewerResolved:
		return m.handleViewerResolved(msg)
	case cmds.ReposDiscovered:
		return m.handleReposDiscovered(msg)
	case cmds.DashboardLoaded:
		return m.handleDashboardLoaded(msg)
	case cmds.InvolvingLoaded:
		return m.handleInvolvingLoaded(msg)
	case cmds.RecentLoaded:
		return m.handleRecentLoaded(msg)
	case cmds.PreviewLoaded:
		return m.handlePreviewLoaded(msg)
	case cmds.SearchIndexRebuilt:
		return nil
	case cmds.RefreshStarted:
		m.state.Jobs.InFlight[msg.Key] = true
		m.syncStatus()
		return nil
	case cmds.RefreshFinished:
		delete(m.state.Jobs.InFlight, msg.Key)
		m.syncStatus()
		return nil
	case cmds.RefreshFailed:
		delete(m.state.Jobs.InFlight, msg.Key)
		m.logError("refresh failed", gitlog.FieldJobKey, msg.Key, "err", msg.Err)
		m.recordError(domain.ErrorKindNetwork, msg.Err, "")
		m.syncStatus()
		return nil
	case dashboard.SelectRepoMsg:
		return m.handleSelectRepoMsg(msg)
	case dashboard.SelectPRMsg:
		return m.handleSelectPRMsg(msg)
	case dashboard.ChangeTabMsg:
		return m.handleChangeTabMsg(msg)
	case dashboard.PreviewFetchMsg:
		return m.handlePreviewFetchMsg(msg)
	case dashboard.PreviewLoadedMsg:
		return m.handlePreviewLoadedMsg(msg.Repo, msg.Number, msg.Preview, nil)
	case overlay.DispatchMsg:
		return m.handleOverlayDispatch(msg)
	case overlay.SelectRepo:
		return m.selectRepoByFullName(msg.Repo, false)
	case overlay.OpenPR:
		return openBrowserForPRCmd(m.selectedRepoForURL(msg.Repo), msg.Repo, msg.Number)
	case overlay.CloseCmdPalette:
		return m.closePalette()
	case browserOpenFailed:
		m.recordError(domain.ErrorKindNetwork, msg.Err, "")
		return nil
	case keymap.MoveRepoSelection, keymap.MovePRSelection, keymap.ScrollPreview, keymap.MovePaletteSelection, keymap.SelectRepo, keymap.SelectPR, keymap.ChangeTab, keymap.CycleFocus:
		// These are only expected when tests or future code send them directly.
		return nil
	default:
		return nil
	}
}

func (m *Model) handleViewerResolved(msg cmds.ViewerResolved) tea.Cmd {
	if msg.Err != nil {
		m.logError("viewer resolve failed", "host", m.selectedHost(), "err", msg.Err)
		m.recordError(domain.ErrorKindAuth, msg.Err, m.selectedRepoName())
		return nil
	}
	m.clearErrors()
	m.state.Session.Viewer = msg.Login
	m.logDebug("viewer resolved", "login", msg.Login)
	m.rebuildDashboardTabs()
	if repo, ok := m.selectedRepo(); ok && strings.TrimSpace(msg.Login) != "" && m.deps.Dashboard != nil {
		m.syncStatus()
		return cmds.LoadInvolvingCmd(m.deps.Dashboard, repo, msg.Login, false)
	}
	m.syncStatus()
	return nil
}

func (m *Model) handleReposDiscovered(msg cmds.ReposDiscovered) tea.Cmd {
	if msg.Err != nil {
		m.logError("repo discovery failed", "root", m.deps.Root, "err", msg.Err)
		m.recordError(domain.ErrorKindDiscovery, msg.Err, "")
	}
	if len(msg.Repos) == 0 {
		m.state.Repos.Discovered = nil
		m.state.Repos.SelectedIndex = -1
		m.state.Repos.SelectedRepo = nil
		m.state.Repos.CursorIndex = 0
		m.state.Repos.OrderFrozen = false
		m.repoPanel.SetRepos(nil)
		m.resetRepoSelection()
		m.syncPaletteStats()
		m.syncStatus()
		return nil
	}

	repos := append([]domain.Repository(nil), msg.Repos...)
	m.repoPanel.SetRepos(repos)
	m.state.Repos.Discovered = append([]domain.Repository(nil), m.repoPanel.Repos...)
	m.state.Repos.OrderFrozen = m.repoPanel.OrderFrozen

	selected := m.state.Repos.SelectedRepo
	index := -1
	if selected != nil {
		index = findRepoIndex(m.state.Repos.Discovered, selected.FullName)
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.state.Repos.Discovered) {
		index = len(m.state.Repos.Discovered) - 1
	}

	if index >= 0 {
		cmd := m.selectRepoAt(index, m.repoPanel.Repos[index], false)
		m.state.Repos.LastScanAt = nowPtr(m.now())
		m.syncPaletteStats()
		m.syncStatus()
		var rebuild tea.Cmd
		if m.deps.Search != nil {
			rebuild = cmds.RebuildRepoIndexCmd(m.deps.Search, m.state.Repos.Discovered)
		}
		return batch(cmd, rebuild)
	}

	m.syncPaletteStats()
	m.syncStatus()
	if m.deps.Search != nil {
		return cmds.RebuildRepoIndexCmd(m.deps.Search, m.state.Repos.Discovered)
	}
	return nil
}

func (m *Model) handleDashboardLoaded(msg cmds.DashboardLoaded) tea.Cmd {
	repo, ok := m.selectedRepo()
	if !ok || !sameRepo(repo.FullName, msg.Repo) {
		return nil
	}

	if msg.Err != nil {
		m.logError("dashboard load failed", "repo", msg.Repo, "err", msg.Err)
		m.recordError(domain.ErrorKindNetwork, msg.Err, msg.Repo)
	}
	if isZeroDashboardSnapshot(msg.Snapshot) {
		return nil
	}

	if msg.Err == nil {
		m.clearErrors()
	}
	m.currentDashboard = msg.Snapshot
	m.hydratedRepos[msg.Repo] = struct{}{}
	m.state.Dashboard.LastRefreshAt[domain.TabMyPRs] = msg.Snapshot.FetchedAt
	m.state.Dashboard.FreshnessByTab[domain.TabMyPRs] = freshnessFor(msg.Err)
	m.state.Dashboard.FreshnessByTab[domain.TabNeedsReview] = freshnessFor(msg.Err)
	m.rebuildDashboardTabs()
	m.syncPaletteStats()
	m.syncStatus()

	var rebuild tea.Cmd
	if m.deps.Search != nil {
		rebuild = cmds.RebuildPRIndexCmd(m.deps.Search, repo, msg.Snapshot)
	}
	return batch(m.syncCurrentSelection(), rebuild)
}

func (m *Model) handleInvolvingLoaded(msg cmds.InvolvingLoaded) tea.Cmd {
	repo, ok := m.selectedRepo()
	if !ok || !sameRepo(repo.FullName, msg.Repo) {
		return nil
	}
	if msg.Err != nil {
		m.logError("involving load failed", "repo", msg.Repo, "err", msg.Err)
		m.recordError(domain.ErrorKindNetwork, msg.Err, msg.Repo)
	}
	if isZeroInvolvingSnapshot(msg.Snapshot) {
		return nil
	}

	if msg.Err == nil {
		m.clearErrors()
	}
	m.currentInvolving = msg.Snapshot
	m.hydratedRepos[msg.Repo] = struct{}{}
	m.state.Dashboard.PRsByTab[domain.TabInvolving] = append([]domain.PullRequestSummary(nil), msg.Snapshot.PRs...)
	m.state.Dashboard.LastRefreshAt[domain.TabInvolving] = msg.Snapshot.FetchedAt
	m.state.Dashboard.FreshnessByTab[domain.TabInvolving] = freshnessFor(msg.Err)
	m.prList.SetTabSnapshot(domain.TabInvolving, msg.Snapshot.PRs, msg.Snapshot.TotalCount, msg.Snapshot.Truncated)
	m.prList.Active = m.state.Dashboard.ActiveTab
	m.prList.Cursor = clampIndex(m.prList.Cursor, len(m.currentPRsForTab(m.prList.Active)))
	m.state.Dashboard.SelectedIndex = m.prList.Cursor
	m.syncPaletteStats()
	m.syncStatus()
	return m.syncCurrentSelection()
}

func (m *Model) handleRecentLoaded(msg cmds.RecentLoaded) tea.Cmd {
	repo, ok := m.selectedRepo()
	if !ok || !sameRepo(repo.FullName, msg.Repo) {
		return nil
	}
	if msg.Err != nil {
		m.logError("recent load failed", "repo", msg.Repo, "err", msg.Err)
		m.recordError(domain.ErrorKindNetwork, msg.Err, msg.Repo)
	}
	if isZeroRecentSnapshot(msg.Snapshot) {
		return nil
	}

	if msg.Err == nil {
		m.clearErrors()
	}
	m.currentRecent = msg.Snapshot
	m.hydratedRepos[msg.Repo] = struct{}{}
	m.state.Dashboard.RecentItems = append([]domain.ActivityItem(nil), msg.Snapshot.Items...)
	m.state.Dashboard.LastRefreshAt[domain.TabRecent] = msg.Snapshot.FetchedAt
	m.state.Dashboard.FreshnessByTab[domain.TabRecent] = freshnessFor(msg.Err)
	m.syncPaletteStats()
	m.syncStatus()
	return nil
}

func (m *Model) handlePreviewLoaded(msg cmds.PreviewLoaded) tea.Cmd {
	if msg.Err != nil {
		m.logError("preview load failed", "pr", m.prSlug(msg.Repo, msg.Number), "err", msg.Err)
	} else {
		m.logDebug("preview loaded from service", "pr", m.prSlug(msg.Repo, msg.Number))
	}
	repo, ok := m.selectedRepo()
	if !ok {
		m.logDebug("preview dropped: no selected repo", "pr", m.prSlug(msg.Repo, msg.Number))
		return nil
	}
	if !sameRepo(repo.FullName, msg.Repo) {
		m.logDebug("preview dropped: repo mismatch", "selected", repo.FullName, "loaded", msg.Repo)
		return nil
	}
	current, ok := m.currentSelectedPR()
	if !ok {
		m.logDebug("preview dropped: no current PR", "loaded", m.prSlug(msg.Repo, msg.Number))
		return nil
	}
	if current.Number != msg.Number || !sameRepo(current.Repo, msg.Repo) {
		m.logDebug("preview dropped: PR mismatch", "current", m.prSlug(current.Repo, current.Number), "loaded", m.prSlug(msg.Repo, msg.Number))
		return nil
	}
	return m.handlePreviewLoadedMsg(msg.Repo, msg.Number, msg.Preview, msg.Err)
}

func (m *Model) handleSelectRepoMsg(msg dashboard.SelectRepoMsg) tea.Cmd {
	if msg.Index < 0 || msg.Index >= len(m.repoPanel.Repos) {
		return nil
	}
	selected := m.repoPanel.Repos[msg.Index]
	cmd := m.selectRepoAt(msg.Index, selected, false)
	// Move focus to the PR list panel after selecting a repo.
	m.focus = domain.FocusPRListPanel
	m.previousFocus = domain.FocusRepoPanel
	m.syncStatus()
	return cmd
}

func (m *Model) handleSelectPRAndOpenDetail() tea.Cmd {
	// openPRDetail now handles all state updates (preview, cursor, etc.).
	return m.openPRDetail()
}

func (m *Model) handleSelectPRMsg(msg dashboard.SelectPRMsg) tea.Cmd {
	if msg.Index < 0 {
		return nil
	}
	m.state.Dashboard.ActiveTab = msg.Tab
	m.state.Dashboard.SelectedIndex = msg.Index
	m.prList.Active = msg.Tab
	m.prList.Cursor = msg.Index
	m.prList.Scroll = clampScroll(msg.Index, m.prList.Scroll, len(m.currentPRsForTab(msg.Tab)))
	m.state.Dashboard.PreviewLoading = true
	derived := derivedPreview(msg.Summary)
	m.state.Dashboard.Preview = &derived
	next, cmd := m.preview.Update(msg)
	if panel, ok := next.(*dashboard.PreviewPanelModel); ok {
		m.preview = panel
	}
	m.state.Dashboard.PreviewLoading = m.preview.Loading || m.preview.PendingFetch
	m.syncStatus()
	return cmd
}

func (m *Model) handleChangeTabMsg(msg dashboard.ChangeTabMsg) tea.Cmd {
	m.state.Dashboard.ActiveTab = msg.Tab
	m.prList.Active = msg.Tab
	m.prList.Cursor = 0
	m.prList.Scroll = 0
	m.state.Dashboard.SelectedIndex = 0
	m.state.Dashboard.Preview = nil
	m.state.Dashboard.PreviewLoading = false
	m.preview = dashboard.NewPreviewPanelModel()
	m.preview.SetTheme(m.theme)
	m.preview.SetRect(m.layout.Current.Preview, m.bodyHeight()-2)
	m.syncStatus()
	return m.syncCurrentSelection()
}

func (m *Model) handlePreviewFetchMsg(msg dashboard.PreviewFetchMsg) tea.Cmd {
	// Always forward to preview panel to clear PendingFetch, regardless of whether
	// we proceed with the fetch. Without this, PendingFetch stays true permanently
	// after the debounce timer fires, and future SelectPRMsgs never start a new timer.
	next, _ := m.preview.Update(msg)
	if panel, ok := next.(*dashboard.PreviewPanelModel); ok {
		m.preview = panel
	}

	repo, ok := m.selectedRepo()
	if !ok {
		m.logDebug("preview fetch skipped: no selected repo", "fetch_repo", msg.Repo, "fetch_number", msg.Number)
		return nil
	}
	if !sameRepo(repo.FullName, msg.Repo) {
		m.logDebug("preview fetch skipped: repo mismatch", "selected", repo.FullName, "fetch_repo", msg.Repo)
		return nil
	}
	current, ok := m.currentSelectedPR()
	if !ok {
		m.logDebug("preview fetch skipped: no current PR", "fetch_repo", msg.Repo, "fetch_number", msg.Number)
		return nil
	}
	if current.Number != msg.Number || !sameRepo(current.Repo, msg.Repo) {
		m.logDebug("preview fetch skipped: PR mismatch", "current", m.prSlug(current.Repo, current.Number), "fetch", m.prSlug(msg.Repo, msg.Number))
		return nil
	}
	if m.deps.Dashboard == nil {
		m.logDebug("preview fetch skipped: no dashboard service")
		return nil
	}
	m.logDebug("preview fetch starting", "pr", m.prSlug(msg.Repo, msg.Number))
	key := jobKey(msg.Repo, "preview")
	m.state.Jobs.InFlight[key] = true
	m.syncStatus()
	return cmds.LoadPreviewCmd(m.deps.Dashboard, msg.Repo, msg.Number)
}

func (m *Model) handlePreviewLoadedMsg(repo string, number int, preview domain.PRPreviewSnapshot, err error) tea.Cmd {
	currentRepo, ok := m.selectedRepo()
	if !ok {
		m.logDebug("preview msg dropped: no selected repo", "pr", m.prSlug(repo, number))
		return nil
	}
	if !sameRepo(currentRepo.FullName, repo) {
		m.logDebug("preview msg dropped: repo mismatch", "selected", currentRepo.FullName, "loaded", repo)
		return nil
	}
	current, ok := m.currentSelectedPR()
	if !ok {
		m.logDebug("preview msg dropped: no current PR", "loaded", m.prSlug(repo, number))
		return nil
	}
	if current.Number != number || !sameRepo(current.Repo, repo) {
		m.logDebug("preview msg dropped: PR mismatch", "current", m.prSlug(current.Repo, current.Number), "loaded", m.prSlug(repo, number))
		return nil
	}
	delete(m.state.Jobs.InFlight, jobKey(repo, "preview"))
	m.state.Dashboard.PreviewLoading = false
	if err != nil {
		m.logError("preview load error", "pr", m.prSlug(repo, number), "err", err)
		m.recordError(domain.ErrorKindNetwork, err, repo)
	}
	if isZeroPreviewSnapshot(preview) {
		m.logDebug("preview snapshot is empty", "pr", m.prSlug(repo, number))
		return nil
	}
	m.logDebug("applying preview snapshot", "pr", m.prSlug(repo, number))
	if err == nil {
		m.clearErrors()
	}
	next, cmd := m.preview.Update(dashboard.PreviewLoadedMsg{
		Repo:    repo,
		Number:  number,
		Preview: preview,
	})
	if panel, ok := next.(*dashboard.PreviewPanelModel); ok {
		m.preview = panel
	}
	m.state.Dashboard.PreviewLoading = m.preview.Loading || m.preview.PendingFetch
	previewCopy := preview
	m.state.Dashboard.Preview = &previewCopy
	m.state.Dashboard.FreshnessByTab[m.state.Dashboard.ActiveTab] = freshnessFor(err)
	m.syncStatus()
	return cmd
}

func (m *Model) handleOverlayDispatch(msg overlay.DispatchMsg) tea.Cmd {
	var out []tea.Cmd
	for _, item := range msg.Messages {
		if item == nil {
			continue
		}
		if cmd := m.applyMessage(item); cmd != nil {
			out = append(out, cmd)
		}
	}
	return batch(out...)
}

func (m *Model) togglePalette() tea.Cmd {
	if m.state.Search.OverlayOpen {
		return m.closePalette()
	}
	m.previousFocus = m.focus
	m.state.Search.OverlayOpen = true
	m.focus = domain.FocusCmdPalette
	m.palette = overlay.NewModel(m.deps.Search)
	m.palette.SetActiveRepo(m.selectedRepoName())
	m.syncPaletteStats()
	m.syncStatus()
	return nil
}

func (m *Model) closePalette() tea.Cmd {
	m.state.Search.OverlayOpen = false
	if m.previousFocus == "" {
		m.focus = domain.FocusRepoPanel
	} else {
		m.focus = m.previousFocus
	}
	m.syncStatus()
	return nil
}

func (m *Model) cycleFocus(direction keymap.CycleDirection) tea.Cmd {
	if m.state.Search.OverlayOpen {
		return nil
	}
	idx := max(indexOfFocus(m.focus), 0)
	switch direction {
	case keymap.FocusPrev:
		idx--
	default:
		idx++
	}
	if idx < 0 {
		idx = len(dashboardFocusCycle) - 1
	}
	if idx >= len(dashboardFocusCycle) {
		idx = 0
	}
	m.focus = dashboardFocusCycle[idx]
	m.syncStatus()
	return nil
}

func (m *Model) refreshSelectedRepo(force bool) tea.Cmd {
	repo, ok := m.selectedRepo()
	if !ok {
		return nil
	}
	return batch(m.loadRepoCmds(repo, force)...)
}

func (m *Model) selectRepoByFullName(fullName string, force bool) tea.Cmd {
	index := findRepoIndex(m.state.Repos.Discovered, fullName)
	if index < 0 {
		return nil
	}
	return m.selectRepoAt(index, m.repoPanel.Repos[index], force)
}

func (m *Model) selectRepoAt(index int, repo domain.Repository, force bool) tea.Cmd {
	if index < 0 {
		return nil
	}
	current, ok := m.selectedRepo()
	if ok && sameRepo(current.FullName, repo.FullName) && m.state.Repos.SelectedIndex == index {
		return nil
	}

	m.logDebug("selecting repo", "index", index, "repo", repo.FullName, "path", repo.LocalPath)

	selected := repo
	m.state.Repos.SelectedIndex = index
	m.state.Repos.SelectedRepo = &selected
	m.state.Repos.CursorIndex = index
	m.repoPanel.ActiveIndex = index
	m.repoPanel.Cursor = index
	m.state.Session.ActiveHost = repoHost(repo)

	m.state.Dashboard.ActiveTab = currentTabOrDefault(m.state.Dashboard.ActiveTab)
	m.prList.Active = m.state.Dashboard.ActiveTab
	m.prList.Cursor = 0
	m.prList.Scroll = 0
	m.state.Dashboard.SelectedIndex = 0
	m.resetDashboardsForRepo()
	m.resetPreviewState()
	m.palette.SetActiveRepo(repo.FullName)
	m.syncPaletteStats()
	m.syncStatus()

	cmdsOut := []tea.Cmd{}
	if m.deps.Viewer != nil && strings.TrimSpace(m.state.Session.ActiveHost) != "" {
		cmdsOut = append(cmdsOut, cmds.ResolveViewerCmd(m.deps.Viewer, m.state.Session.ActiveHost))
	}
	// Always use cache first (force=false). The stale-while-revalidate
	// mechanism will schedule a background refresh if data is stale.
	cmdsOut = append(cmdsOut, m.loadRepoCmds(repo, false)...)
	return batch(cmdsOut...)
}

func (m *Model) loadRepoCmds(repo domain.Repository, force bool) []tea.Cmd {
	if m.deps.Dashboard == nil {
		return nil
	}
	out := []tea.Cmd{cmds.LoadDashboardCmd(m.deps.Dashboard, repo, force), cmds.LoadRecentCmd(m.deps.Dashboard, repo, force)}
	if strings.TrimSpace(m.state.Session.Viewer) != "" {
		out = append(out, cmds.LoadInvolvingCmd(m.deps.Dashboard, repo, m.state.Session.Viewer, force))
	}
	return out
}

func (m *Model) syncCurrentSelection() tea.Cmd {
	current, ok := m.currentSelectedPR()
	if !ok {
		m.logDebug("no PR selected for preview", "active_tab", string(m.prList.Active), "my_prs_count", len(m.currentPRsForTab(domain.TabMyPRs)), "needs_review_count", len(m.currentPRsForTab(domain.TabNeedsReview)))
		m.resetPreviewState()
		m.syncStatus()
		return nil
	}
	m.logDebug("sync preview", "pr", m.prSlug(current.Repo, current.Number), "tab", string(m.prList.Active))
	m.state.Dashboard.SelectedIndex = m.prList.Cursor
	m.state.Dashboard.PreviewLoading = true
	derived := derivedPreview(current)
	m.state.Dashboard.Preview = &derived
	next, cmd := m.preview.Update(dashboard.SelectPRMsg{
		Tab:     m.prList.Active,
		Index:   m.prList.Cursor,
		Repo:    current.Repo,
		Number:  current.Number,
		Summary: current,
	})
	if panel, ok := next.(*dashboard.PreviewPanelModel); ok {
		m.preview = panel
	}
	m.state.Dashboard.PreviewLoading = m.preview.Loading || m.preview.PendingFetch
	m.syncStatus()
	return cmd
}

func (m *Model) applyWindowSize(msg tea.WindowSizeMsg) {
	m.layout = m.layout.Update(msg)
	bodyH := m.bodyHeight()
	m.repoPanel.SetRect(m.layout.Current.Repo, bodyH)
	m.prList.SetRect(m.layout.Current.PR, bodyH)
	m.preview.SetRect(m.layout.Current.Preview, bodyH-2)
	m.status.SetRect(m.layout.Current.Width)
	m.palette, _ = m.palette.Update(msg)
	m.syncPaletteStats()
	m.syncStatus()
}

func (m *Model) syncPanels() {
	if m.repoPanel == nil {
		m.repoPanel = dashboard.NewRepoPanelModel(nil)
	}
	if m.prList == nil {
		m.prList = dashboard.NewPRListPanelModel()
	}
	if m.preview == nil {
		m.preview = dashboard.NewPreviewPanelModel()
	}
	if m.status == nil {
		m.status = dashboard.NewStatusBarModel()
	}
}

func (m *Model) syncStatus() {
	m.status.Focus = m.focus
	m.status.Loading = len(m.state.Jobs.InFlight) > 0 || m.state.Dashboard.PreviewLoading
	m.status.Freshness = m.state.Dashboard.FreshnessByTab[m.state.Dashboard.ActiveTab]
	m.status.Errors = m.state.Errors
	m.status.CurrentTab = m.state.Dashboard.ActiveTab
	if repo, ok := m.selectedRepo(); ok {
		m.status.SelectedRepo = repo.FullName
	} else {
		m.status.SelectedRepo = ""
	}
}

// ViewStack methods

func (m *Model) pushView(v domain.PrimaryView) {
	// Guard: prevent double-push of the same view.
	if m.currentView() == v {
		return
	}
	m.viewStack = append(m.viewStack, v)
}

func (m *Model) popView() domain.PrimaryView {
	if len(m.viewStack) <= 1 {
		return m.viewStack[0]
	}
	m.viewStack = m.viewStack[:len(m.viewStack)-1]
	return m.viewStack[len(m.viewStack)-1]
}

func (m *Model) currentView() domain.PrimaryView {
	return m.viewStack[len(m.viewStack)-1]
}

func (m *Model) syncPaletteStats() {
	m.palette.SetActiveRepo(m.selectedRepoName())
	total := len(m.state.Repos.Discovered)
	hydrated := len(m.hydratedRepos)
	if hydrated > total {
		hydrated = total
	}
	m.palette.SetRepoHydrationStats(total, hydrated)
}

func (m *Model) syncPaletteState() {
	m.state.Search.OverlayOpen = m.focus == domain.FocusCmdPalette
	m.syncPaletteStats()
}

func (m *Model) resetDashboardsForRepo() {
	// Do NOT clear PR data — stale cache should remain visible while fresh data loads.
	// Only reset per-repo transient state.
	m.state.Dashboard.SelectedIndex = 0
	m.state.Dashboard.Preview = nil
	m.state.Dashboard.PreviewLoading = false
	// Clear tab counts and freshness — these are per-repo and will be
	// repopulated when DashboardLoaded messages arrive from the cache.
	m.state.Dashboard.TotalCount = 0
	m.state.Dashboard.Truncated = false
}

func (m *Model) resetPreviewState() {
	m.preview = dashboard.NewPreviewPanelModel()
	m.preview.SetTheme(m.theme)
	m.preview.SetRect(m.layout.Current.Preview, m.bodyHeight()-2)
	m.state.Dashboard.Preview = nil
	m.state.Dashboard.PreviewLoading = false
}

func (m *Model) resetRepoSelection() {
	m.repoPanel.ActiveIndex = -1
	m.repoPanel.Cursor = 0
	m.state.Repos.SelectedIndex = -1
	m.state.Repos.SelectedRepo = nil
	m.state.Repos.CursorIndex = 0
	m.resetDashboardsForRepo()
	m.resetPreviewState()
	m.syncStatus()
}

func (m *Model) selectedRepo() (domain.Repository, bool) {
	if m.state.Repos.SelectedRepo == nil {
		return domain.Repository{}, false
	}
	return *m.state.Repos.SelectedRepo, true
}

func (m *Model) selectedRepoName() string {
	if repo, ok := m.selectedRepo(); ok {
		return repo.FullName
	}
	return ""
}

func (m *Model) selectedRepoForURL(fallback string) domain.Repository {
	if repo, ok := m.selectedRepo(); ok {
		if fallback != "" && !sameRepo(repo.FullName, fallback) {
			return domain.Repository{Host: repoHost(repo), FullName: fallback}
		}
		return repo
	}
	if fallback != "" {
		return domain.Repository{Host: m.selectedHost(), FullName: fallback}
	}
	return domain.Repository{Host: m.selectedHost()}
}

func (m *Model) selectedHost() string {
	host := strings.TrimSpace(m.state.Session.ActiveHost)
	if host != "" {
		return host
	}
	if host = strings.TrimSpace(m.deps.Host); host != "" {
		return host
	}
	return "github.com"
}

func (m *Model) currentPRsForTab(tab domain.DashboardTab) []domain.PullRequestSummary {
	return append([]domain.PullRequestSummary(nil), m.state.Dashboard.PRsByTab[tab]...)
}

func (m *Model) rebuildDashboardTabs() {
	if isZeroDashboardSnapshot(m.currentDashboard) {
		return
	}
	classified := m.classifier.Classify(m.state.Session.Viewer, m.currentDashboard.PRs)
	m.state.Dashboard.PRsByTab[domain.TabMyPRs] = append([]domain.PullRequestSummary(nil), classified[domain.TabMyPRs]...)
	m.state.Dashboard.PRsByTab[domain.TabNeedsReview] = append([]domain.PullRequestSummary(nil), classified[domain.TabNeedsReview]...)
	m.state.Dashboard.TotalCount = m.currentDashboard.TotalCount
	m.state.Dashboard.Truncated = m.currentDashboard.Truncated
	m.state.Dashboard.LastRefreshAt[domain.TabMyPRs] = m.currentDashboard.FetchedAt
	m.state.Dashboard.LastRefreshAt[domain.TabNeedsReview] = m.currentDashboard.FetchedAt
	m.prList.SetTabSnapshot(domain.TabMyPRs, m.state.Dashboard.PRsByTab[domain.TabMyPRs], m.currentDashboard.TotalCount, m.currentDashboard.Truncated)
	m.prList.SetTabSnapshot(domain.TabNeedsReview, m.state.Dashboard.PRsByTab[domain.TabNeedsReview], m.currentDashboard.TotalCount, m.currentDashboard.Truncated)
	m.prList.Active = m.state.Dashboard.ActiveTab
	m.prList.Cursor = clampIndex(m.prList.Cursor, len(m.currentPRsForTab(m.prList.Active)))
	m.state.Dashboard.SelectedIndex = m.prList.Cursor
}

func (m *Model) currentSelectedPR() (domain.PullRequestSummary, bool) {
	prs := m.currentPRsForTab(m.prList.Active)
	if len(prs) == 0 {
		return domain.PullRequestSummary{}, false
	}
	if m.prList.Cursor < 0 || m.prList.Cursor >= len(prs) {
		return domain.PullRequestSummary{}, false
	}
	return prs[m.prList.Cursor], true
}

func (m *Model) now() time.Time {
	if m.deps.Now != nil {
		return m.deps.Now()
	}
	return time.Now()
}

func (m *Model) recordError(kind domain.ErrorKind, err error, repo string) {
	if err == nil {
		return
	}
	m.state.Errors.Errors = []domain.AppError{{
		Kind:    kind,
		Message: err.Error(),
		Repo:    repo,
	}}
	m.syncStatus()
}

func (m *Model) clearErrors() {
	m.state.Errors.Errors = nil
	m.state.Errors.RateLimitReset = nil
}

func (m *Model) bodyHeight() int {
	if m.layout.Current.Height <= 2 {
		return 0
	}
	return m.layout.Current.Height - 2
}

func nowPtr(t time.Time) *time.Time {
	return &t
}

func batch(cmdsOut ...tea.Cmd) tea.Cmd {
	filtered := make([]tea.Cmd, 0, len(cmdsOut))
	for _, cmd := range cmdsOut {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return tea.Batch(filtered...)
}

func isRootAction(action keymap.Action) bool {
	switch action.(type) {
	case keymap.ToggleCmdPalette, keymap.CloseCmdPalette, keymap.CycleFocus, keymap.TriggerRefresh, keymap.OpenBrowser, keymap.OpenPRDetail, keymap.SelectPR, keymap.Quit, keymap.OpenDashboardFilter:
		return true
	default:
		return false
	}
}

func sameRepo(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func findRepoIndex(repos []domain.Repository, fullName string) int {
	for i := range repos {
		if sameRepo(repos[i].FullName, fullName) {
			return i
		}
	}
	return -1
}

func indexOfFocus(focus domain.FocusTarget) int {
	for i, candidate := range dashboardFocusCycle {
		if candidate == focus {
			return i
		}
	}
	return -1
}

func currentTabOrDefault(tab domain.DashboardTab) domain.DashboardTab {
	if strings.TrimSpace(string(tab)) == "" {
		return domain.TabMyPRs
	}
	return tab
}

func clampIndex(index, size int) int {
	if size <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= size {
		return size - 1
	}
	return index
}

func clampScroll(index, current, size int) int {
	if size <= 0 {
		return 0
	}
	if current < 0 {
		current = 0
	}
	if index < current {
		return index
	}
	if index >= current {
		return current
	}
	return current
}

func freshnessFor(err error) domain.Freshness {
	if err == nil {
		return domain.FreshnessFresh
	}
	return domain.FreshnessErrorStale
}

func repoHost(repo domain.Repository) string {
	if host := strings.TrimSpace(repo.Host); host != "" {
		return host
	}
	return "github.com"
}

func jobKey(repo, kind string) string {
	return strings.TrimSpace(repo) + ":" + kind
}

func isZeroDashboardSnapshot(s domain.DashboardSnapshot) bool {
	return strings.TrimSpace(s.Repo.FullName) == "" && len(s.PRs) == 0 && s.TotalCount == 0 && !s.Truncated
}

func isZeroInvolvingSnapshot(s domain.InvolvingSnapshot) bool {
	return strings.TrimSpace(s.Repo.FullName) == "" && len(s.PRs) == 0 && s.TotalCount == 0 && !s.Truncated
}

func isZeroRecentSnapshot(s domain.RecentSnapshot) bool {
	return strings.TrimSpace(s.Repo.FullName) == "" && len(s.Items) == 0
}

func isZeroPreviewSnapshot(s domain.PRPreviewSnapshot) bool {
	return strings.TrimSpace(s.Repo) == "" && s.Number == 0 && strings.TrimSpace(s.Title) == "" && strings.TrimSpace(s.BodyExcerpt) == ""
}

func derivedPreview(summary domain.PullRequestSummary) domain.PRPreviewSnapshot {
	return domain.PRPreviewSnapshot{
		Repo:           summary.Repo,
		Number:         summary.Number,
		Title:          summary.Title,
		BodyExcerpt:    "",
		Author:         summary.Author,
		State:          summary.State,
		IsDraft:        summary.IsDraft,
		CIStatus:       summary.CIStatus,
		ReviewDecision: summary.ReviewDecision,
		CreatedAt:      summary.CreatedAt,
		UpdatedAt:      summary.UpdatedAt,
	}
}

func (m *Model) openPRDetail() tea.Cmd {
	current, ok := m.currentSelectedPR()
	if !ok {
		return nil
	}
	repo, ok := m.selectedRepo()
	if !ok {
		return nil
	}

	m.logDebug("opening pr detail", "pr", m.prSlug(current.Repo, current.Number), "repo", repo.FullName)

	// Same PR reuse: if prDetail exists and matches repo+number, reuse it without
	// re-init — Init() would re-fire network requests and overwrite scroll state.
	// Preview state is left untouched; it was already set by the j/k SelectPRMsg path.
	if m.prDetail != nil && m.prDetail.Summary.Repo == current.Repo && m.prDetail.Summary.Number == current.Number {
		m.logDebug("reusing existing pr detail model", "pr", m.prSlug(current.Repo, current.Number))
		m.pushView(domain.PrimaryViewPRDetail)
		return nil
	}

	// Different PR or nil — construct fresh model.
	// PR service may be nil (not wired yet) — model still renders from summary.
	m.prDetail = prdetail.NewModel(current, repo, m.deps.PR)
	m.prDetail.SetTheme(m.theme)
	m.prDetail.Width = m.layout.Current.Width
	m.prDetail.Height = m.layout.Current.Height - 2 // minus status bar
	m.pushView(domain.PrimaryViewPRDetail)

	return m.prDetail.Init()
}

func (m *Model) openBrowserForCurrentPR() tea.Cmd {
	current, ok := m.currentSelectedPR()
	if !ok {
		return nil
	}
	repo := m.selectedRepoForURL(current.Repo)
	m.logDebug("opening browser", "pr", m.prSlug(current.Repo, current.Number))
	return openBrowserForPRCmd(repo, current.Repo, current.Number)
}

// handleBackToDashboard pops the PR detail view and restores dashboard focus.
// Preview state is untouched: it was set when the PR was selected via j/k navigation
// and continues updating in the background via applyMessage while detail was open.
func (m *Model) handleBackToDashboard() tea.Cmd {
	if m.prDetail != nil {
		m.logDebug("pr detail closed", "pr", m.prSlug(m.prDetail.Summary.Repo, m.prDetail.Summary.Number))
	}
	m.popView()
	m.focus = domain.FocusPRListPanel
	m.previousFocus = domain.FocusPreviewPanel
	m.syncStatus()
	return nil
}

func openBrowserForPRCmd(repo domain.Repository, fallbackRepo string, number int) tea.Cmd {
	return func() tea.Msg {
		url := browserURL(repo, fallbackRepo, number)
		if err := openURL(url); err != nil {
			return browserOpenFailed{URL: url, Err: err}
		}
		return nil
	}
}

type browserOpenFailed struct {
	URL string
	Err error
}

func browserURL(repo domain.Repository, fallbackRepo string, number int) string {
	fullName := strings.TrimSpace(repo.FullName)
	if fullName == "" {
		fullName = strings.TrimSpace(fallbackRepo)
	}
	host := repoHost(repo)
	if fullName == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/pull/%d", host, fullName, number)
}

func openURL(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("missing browser URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func (m *Model) slug() string {
	if repo, ok := m.selectedRepo(); ok {
		return repo.FullName
	}
	return ""
}

func (m *Model) prSlug(repo string, number int) string {
	return fmt.Sprintf("%s#%d", repo, number)
}

func (m *Model) logInfo(msg string, attrs ...any) {
	pairs := append([]any{gitlog.FieldRepo, m.slug()}, attrs...)
	m.log.Info(msg, pairs...)
}

func (m *Model) logWarn(msg string, attrs ...any) {
	pairs := append([]any{gitlog.FieldRepo, m.slug()}, attrs...)
	m.log.Warn(msg, pairs...)
}

func (m *Model) logError(msg string, attrs ...any) {
	pairs := append([]any{gitlog.FieldRepo, m.slug()}, attrs...)
	m.log.Error(msg, pairs...)
}

func (m *Model) logDebug(msg string, attrs ...any) {
	pairs := append([]any{gitlog.FieldRepo, m.slug()}, attrs...)
	m.log.Debug(msg, pairs...)
}
