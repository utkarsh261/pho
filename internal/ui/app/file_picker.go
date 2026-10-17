package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

const filePickHint = "↑/↓: Move | Enter: Go to file | Esc: Close"

// filePicking reports whether the file picker is open.
func (m *Model) filePicking() bool {
	return m.state.Search.OverlayOpen && m.palette.IsFilePick()
}

// diffView is the detail model on screen: the PR, or a commit opened from
// it. nil on the dashboard.
func (m *Model) diffView() *prdetail.PRDetailModel {
	switch m.currentView() {
	case domain.PrimaryViewPRDetail:
		return m.prDetail
	case domain.PrimaryViewCommitDetail:
		if m.commitDetail != nil {
			return m.commitDetail.Inner()
		}
	}
	return nil
}

// diffViewBody renders the detail view on screen, without the status bar.
func (m *Model) diffViewBody() string {
	switch m.currentView() {
	case domain.PrimaryViewCommitDetail:
		if m.commitDetail != nil {
			return m.commitDetail.View()
		}
	case domain.PrimaryViewPRDetail:
		if m.prDetail != nil {
			return m.prDetail.View()
		}
	}
	return ""
}

// openFilePicker opens the palette as a chooser of the diff's files, starting
// on the current one. Like T, ctrl+p reaches here as a message one step after
// the key, so it stays shut if another overlay opened in between, or if the
// diff hasn't loaded.
func (m *Model) openFilePicker() tea.Cmd {
	if m.state.Search.OverlayOpen || m.keymapOverlay.Visible ||
		(m.createPR != nil && m.createPR.Active()) {
		return nil
	}
	d := m.diffView()
	if d == nil || d.Diff == nil || len(d.Diff.Files) == 0 {
		return nil
	}
	files := make([]overlay.FileItem, 0, len(d.Diff.Files))
	for i := range d.Diff.Files {
		f := &d.Diff.Files[i]
		files = append(files, overlay.FileItem{Path: prdetail.FilePath(f), Status: f.Status, Additions: f.Additions, Deletions: f.Deletions})
	}
	m.previousFocus = m.focus
	m.state.Search.OverlayOpen = true
	m.focus = domain.FocusCmdPalette
	m.palette = overlay.NewModel(m.deps.Search)
	m.palette.SetTheme(m.theme)
	m.palette.OpenFilePick(files, d.CurrentFile())
	m.palette, _ = m.palette.Update(tea.WindowSizeMsg{Width: m.layout.Width, Height: m.layout.Height})
	m.syncStatus()
	return nil
}

// handleFilePickKey runs the picker: Enter jumps to the selected file, Esc
// and ctrl+p close it, and other keys edit the filter or move the selection.
func (m *Model) handleFilePickKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case msg.Type == tea.KeyEsc || msg.String() == "ctrl+p":
		m.closeOverlay()
		return nil
	case msg.Type == tea.KeyEnter:
		path := m.palette.SelectedFile()
		if path == "" {
			return nil // nothing matches the filter; stay open
		}
		m.closeOverlay()
		if d := m.diffView(); d != nil {
			d.GoToFile(path)
		}
		m.syncStatus()
		return nil
	}
	m.palette, _ = m.palette.Update(msg)
	return nil
}
