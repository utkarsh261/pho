// Package prdetail implements the PR detail view model for Phase 2.
// It manages the PR detail state and handles keyboard routing within
// the PR detail view.
package prdetail

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utk/git-term/internal/application/cmds"
	"github.com/utk/git-term/internal/diff/model"
	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/ui/theme"
)

// PRDetailModel is the Bubble Tea model for the PR detail view.
type PRDetailModel struct {
	// Summary is the cached PullRequestSummary that opened this view.
	Summary domain.PullRequestSummary

	// Detail is the extended PR metadata from GraphQL.
	Detail *domain.PRPreviewSnapshot

	// Diff is the parsed unified diff model.
	Diff *model.DiffModel

	// Loading tracks whether data is still being fetched.
	DetailLoading bool
	DiffLoading   bool

	// FromCache tracks whether the detail was served from cache.
	DetailFromCache bool

	// Width and height of the view rect.
	Width  int
	Height int

	// PR service for loading data.
	PRService cmds.PRService
	Repo      domain.Repository

	// contentScroll tracks the vertical scroll position.
	ContentScroll int

	// lastKey is used for double-g (gg) detection.
	LastKey string
}

// NewModel creates a new PRDetailModel for the given PR.
func NewModel(summary domain.PullRequestSummary, repo domain.Repository, prService cmds.PRService) *PRDetailModel {
	loading := prService != nil
	return &PRDetailModel{
		Summary:       summary,
		PRService:     prService,
		Repo:          repo,
		DetailLoading: loading,
		DiffLoading:   loading,
	}
}

// SetTheme applies a theme to the PR detail model (currently unused in Phase 2).
func (m *PRDetailModel) SetTheme(th *theme.Theme) {
	_ = th // reserved for Phase 3 styling
}

// Init fires the parallel load commands for PR detail and diff.
func (m *PRDetailModel) Init() tea.Cmd {
	if m.PRService == nil {
		return nil
	}
	headSHA := m.Summary.HeadRefOID
	return tea.Batch(
		cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, false),
		cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, false),
	)
}

// Update handles messages and key events within the PR detail view.
func (m *PRDetailModel) Update(msg tea.Msg) (*PRDetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case cmds.PRDetailLoaded:
		m.DetailLoading = false
		if msg.Err != nil {
			// If we have no detail at all, keep loading state for retry.
			if m.Detail == nil {
				m.DetailLoading = true
			}
			return m, nil
		}
		m.Detail = &msg.Detail
		m.DetailFromCache = msg.FromCache

		var cmdsOut []tea.Cmd
		// If data came from stale cache, schedule background revalidation.
		if msg.FromCache {
			cmdsOut = append(cmdsOut, cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true))
		}
		return m, tea.Batch(cmdsOut...)

	case cmds.DiffLoaded:
		m.DiffLoading = false
		if msg.Err != nil {
			if m.Diff == nil {
				m.DiffLoading = true
			}
			return m, nil
		}
		// Validate SHA if HeadRefOID is available.
		if m.Summary.HeadRefOID != "" && msg.Diff.HeadSHA != "" && msg.Diff.HeadSHA != m.Summary.HeadRefOID {
			// SHA mismatch — discard and refetch.
			m.DiffLoading = true
			return m, cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, m.Summary.HeadRefOID, true)
		}
		m.Diff = &msg.Diff
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	default:
		return m, nil
	}
}

// View renders the PR detail view.
// Header renders immediately from the cached summary.
// Diff area shows a spinner until DiffLoaded arrives.
// Returns exactly m.Height rows of content (status bar is composed separately).
func (m *PRDetailModel) View() string {
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}

	var lines []string

	// Header row: PR title, number, state, author, repo.
	lines = append(lines, m.renderHeader())

	// Section buttons (scroll-spy indicators).
	lines = append(lines, m.renderSectionButtons())

	// Content area. m.Height is already terminal_height - 2 (status bar space reserved).
	contentH := m.Height - len(lines)
	if contentH < 1 {
		contentH = 1
	}

	// Description section header.
	lines = append(lines, m.sectionDivider("Description"))

	// If detail is loaded, show body text.
	if m.Detail != nil && strings.TrimSpace(m.Detail.BodyExcerpt) != "" {
		bodyLines := strings.Split(m.Detail.BodyExcerpt, "\n")
		for i := 0; i < contentH && i < len(bodyLines); i++ {
			lines = append(lines, "  "+bodyLines[i])
		}
	} else if !m.DetailLoading {
		lines = append(lines, "  No description provided.")
	} else {
		lines = append(lines, "  Loading...")
	}

	// Diff section header.
	lines = append(lines, m.sectionDivider("Diff"))

	// Diff content or spinner.
	if m.DiffLoading {
		lines = append(lines, "  ⠋ Loading diff…")
	} else if m.Diff == nil || len(m.Diff.Files) == 0 {
		lines = append(lines, "  No changes")
	} else {
		// Render diff stats summary.
		stats := m.Diff.Stats
		lines = append(lines, fmt.Sprintf("  %d file(s), +%d -%d", stats.TotalFiles, stats.TotalAdditions, stats.TotalDeletions))
	}

	// Pad remaining content lines to exactly m.Height.
	for len(lines) < m.Height {
		lines = append(lines, "")
	}

	return strings.Join(lines[:m.Height], "\n")
}

// renderHeader builds the header row with PR title, number, state, author, repo.
func (m *PRDetailModel) renderHeader() string {
	s := m.Summary
	state := string(s.State)
	if s.IsDraft {
		state = "Draft"
	}
	title := s.Title
	if title == "" {
		title = "(no title)"
	}
	// Truncate title to fit.
	maxTitle := m.Width - 40
	if maxTitle < 10 {
		maxTitle = 10
	}
	if len(title) > maxTitle {
		title = title[:maxTitle-1] + "…"
	}

	return fmt.Sprintf("%s #%d  [%s]  %s  %s", title, s.Number, state, s.Author, s.Repo)
}

// renderSectionButtons renders the section navigation buttons.
func (m *PRDetailModel) renderSectionButtons() string {
	return "  Desc | Diff | Comments"
}

// sectionDivider renders a section header with a divider line.
func (m *PRDetailModel) sectionDivider(name string) string {
	return "  ── " + name + " ──────────────────────────────────────────────"
}

// handleKey routes keyboard input within the PR detail view.
func (m *PRDetailModel) handleKey(msg tea.KeyMsg) (*PRDetailModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, m.emitBackToDashboard()
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'q':
				return m, m.emitBackToDashboard()
			case 'r':
				return m.handleRefresh()
			case 'o':
				return m, m.emitOpenBrowser()
			}
		}
	case tea.KeyCtrlO:
		return m, m.emitOpenBrowser()
	}
	m.LastKey = msg.String()
	return m, nil
}

// handleRefresh refires both load commands with force=true.
func (m *PRDetailModel) handleRefresh() (*PRDetailModel, tea.Cmd) {
	if m.PRService == nil {
		return m, nil
	}
	m.DetailLoading = true
	m.DiffLoading = true
	headSHA := m.Summary.HeadRefOID
	return m, tea.Batch(
		cmds.LoadPRDetailCmd(m.PRService, m.Repo, m.Summary.Number, true),
		cmds.LoadDiffCmd(m.PRService, m.Repo, m.Summary.Number, headSHA, true),
	)
}

// emitBackToDashboard emits the BackToDashboard action.
func (m *PRDetailModel) emitBackToDashboard() tea.Cmd {
	return func() tea.Msg {
		return BackToDashboard{}
	}
}

// emitOpenBrowser emits the OpenBrowser action.
func (m *PRDetailModel) emitOpenBrowser() tea.Cmd {
	return func() tea.Msg {
		return OpenBrowserPR{
			Repo:   m.Summary.Repo,
			Number: m.Summary.Number,
		}
	}
}

// BackToDashboard is emitted when the user presses Esc in PR detail.
type BackToDashboard struct{}

// OpenBrowserPR is emitted when the user presses 'o' in PR detail.
type OpenBrowserPR struct {
	Repo   string
	Number int
}
