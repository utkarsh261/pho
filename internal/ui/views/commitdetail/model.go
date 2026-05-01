package commitdetail

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

// Model wraps a PRDetailModel configured for commit viewing.
// All rendering, key handling, and navigation are delegated to
// the embedded PRDetailModel in CommitMode.
type Model struct {
	inner *prdetail.PRDetailModel
}

// NewModel creates a commit detail view by delegating to PRDetailModel in CommitMode.
func NewModel(repo domain.Repository, commit domain.Commit, svc cmds.PRService) *Model {
	inner := prdetail.NewCommitModel(repo, commit, svc)
	return &Model{inner: inner}
}

// SetTheme applies a theme to the commit detail view.
func (m *Model) SetTheme(th *theme.Theme) {
	m.inner.SetTheme(th)
}

// Init fires the load commands for the commit diff.
func (m *Model) Init() tea.Cmd {
	return m.inner.Init()
}

// Update delegates to the inner PRDetailModel.
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	next, cmd := m.inner.Update(msg)
	m.inner = next
	return m, cmd
}

// View delegates to the inner PRDetailModel.
func (m *Model) View() string {
	return m.inner.View()
}

// StatusHint delegates to the inner PRDetailModel.
func (m *Model) StatusHint() string {
	return m.inner.StatusHint()
}

// Inner returns the underlying PRDetailModel for message type routing in app.go.
func (m *Model) Inner() *prdetail.PRDetailModel {
	return m.inner
}

// SetDiff sets the diff data on the inner model (for testing).
func (m *Model) SetDiff(diff *diffmodel.DiffModel) {
	m.inner.Diff = diff
	m.inner.DiffLoading = false
}
