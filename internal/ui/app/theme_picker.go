package app

import (
	"errors"
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const themePickHint = "↑/↓: Preview | Enter: Keep | Esc: Revert"

// ThemeSaved reports a theme picker save. Only the latest save's result is
// shown; Seq tells them apart.
type ThemeSaved struct {
	Seq  int
	Name string
	Err  error
}

// themeSaver writes the chosen theme in order: saves run as commands, which
// may finish out of order, so a save older than the last one written is
// dropped.
type themeSaver struct {
	mu   sync.Mutex
	last int
	save func(name string) error
}

func (s *themeSaver) run(seq int, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq < s.last {
		return nil
	}
	s.last = seq
	return s.save(name)
}

// themePicking reports whether the theme picker is open.
func (m *Model) themePicking() bool {
	return m.state.Search.OverlayOpen && m.palette.IsThemePick()
}

// openThemePicker opens the palette as a theme chooser over the current
// view. Moving the selection applies each theme straight away.
func (m *Model) openThemePicker() tea.Cmd {
	if m.state.Search.OverlayOpen || m.theme == nil {
		return nil
	}
	m.themePickFrom = m.theme.Name
	m.previousFocus = m.focus
	m.state.Search.OverlayOpen = true
	m.focus = domain.FocusCmdPalette
	m.palette = overlay.NewModel(m.deps.Search)
	m.palette.SetTheme(m.theme)
	m.palette.OpenThemePick(theme.Names(), m.theme.Name)
	m.palette, _ = m.palette.Update(tea.WindowSizeMsg{Width: m.layout.Width, Height: m.layout.Height})
	m.syncStatus()
	return nil
}

// handleThemePickKey runs the picker synchronously: previews, Esc and Enter
// all take effect in this Update, so no queued message can reorder them.
func (m *Model) handleThemePickKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		m.applyTheme(m.themePickFrom)
		m.closeThemePicker()
		return nil
	case tea.KeyEnter:
		name := m.palette.PreviewTheme()
		if name == "" {
			return nil // nothing matches the filter; stay open
		}
		m.applyTheme(name)
		m.closeThemePicker()
		return m.saveThemeCmd(name)
	}
	m.palette, _ = m.palette.Update(msg)
	name := m.palette.PreviewTheme()
	if name == "" {
		name = m.themePickFrom
	}
	m.applyTheme(name)
	return nil
}

func (m *Model) closeThemePicker() {
	m.themePickFrom = ""
	m.closePalette()
}

// applyTheme switches every view to the named theme, if it differs.
func (m *Model) applyTheme(name string) {
	if m.theme != nil && m.theme.Name == name {
		return
	}
	if th, ok := theme.Get(name); ok {
		m.SetTheme(th)
	}
}

func (m *Model) saveThemeCmd(name string) tea.Cmd {
	if m.themeSaver == nil {
		return nil
	}
	m.themeSaveSeq++
	seq, saver := m.themeSaveSeq, m.themeSaver
	return func() tea.Msg {
		return ThemeSaved{Seq: seq, Name: name, Err: saver.run(seq, name)}
	}
}

// handleThemeSaved reports a failed save of the latest choice. The theme
// stays applied for the session either way.
func (m *Model) handleThemeSaved(msg ThemeSaved) tea.Cmd {
	if msg.Seq != m.themeSaveSeq {
		return nil
	}
	if msg.Err == nil {
		m.logDebug("theme saved", "theme", msg.Name)
		return nil
	}
	m.log.Warn("theme save failed", "theme", msg.Name, "err", msg.Err)
	text := fmt.Sprintf("theme set for this session · couldn't save: %v", msg.Err)
	if errors.Is(msg.Err, config.ErrManualEdit) {
		text = fmt.Sprintf("theme set for this session · add theme = %q under [ui] in your config", msg.Name)
	}
	m.recordError(domain.ErrorKindConfig, errors.New(text), "")
	return nil
}
