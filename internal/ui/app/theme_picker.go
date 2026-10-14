package app

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/components/overlay"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const themePickHint = "↑/↓: Preview | Enter: Keep | Esc: Revert"

// ThemeSaved reports a theme picker save.
type ThemeSaved struct {
	Name string
	Err  error
}

// themeSaves keeps theme saves in order with one rule: at most one save runs
// at a time, and a choice made meanwhile waits as pending, replacing any
// earlier pending one. The config therefore always ends up with the last
// choice, and only that save's result is reported.
type themeSaves struct {
	save     func(name string) error
	saved    string // the theme the config holds; "" if unknown
	inFlight string // the save running now, if any
	pending  string // the choice to save once it finishes
}

// target is the theme the config will hold once queued saves finish.
func (s *themeSaves) target() string {
	switch {
	case s.pending != "":
		return s.pending
	case s.inFlight != "":
		return s.inFlight
	}
	return s.saved
}

// themePicking reports whether the theme picker is open.
func (m *Model) themePicking() bool {
	return m.state.Search.OverlayOpen && m.palette.IsThemePick()
}

// openThemePicker opens the palette as a theme chooser over the current
// view. Moving the selection applies each theme straight away.
//
// From PR detail, T reaches here as a message one step after the key, so
// another overlay may have opened in between: the picker then stays shut
// rather than open underneath it.
func (m *Model) openThemePicker() tea.Cmd {
	if m.state.Search.OverlayOpen || m.keymapOverlay.Visible || m.theme == nil ||
		(m.createPR != nil && m.createPR.Active()) {
		return nil
	}
	m.pickThemes = map[string]*theme.Theme{m.theme.Name: m.theme}
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
		m.cancelThemePicker()
		return nil
	case tea.KeyEnter:
		name := m.palette.PreviewTheme()
		if name == "" {
			return nil // nothing matches the filter; stay open
		}
		m.applyTheme(name)
		m.closeThemePicker()
		return m.saveTheme(name)
	}
	m.palette, _ = m.palette.Update(msg)
	name := m.palette.PreviewTheme()
	if name == "" {
		name = m.themePickFrom
	}
	m.applyTheme(name)
	return nil
}

// cancelThemePicker restores the theme the picker opened with and closes it.
func (m *Model) cancelThemePicker() {
	m.applyTheme(m.themePickFrom)
	m.closeThemePicker()
}

// closeThemePicker closes the picker. It leaves a pending `pho pr <n>` deep
// link alone: the picker has nothing to do with it.
func (m *Model) closeThemePicker() {
	m.themePickFrom = ""
	m.pickThemes = nil
	m.closeOverlay()
}

// applyTheme switches every view to the named theme, if it differs. While
// the picker is open each theme is built once, the first time it's shown.
func (m *Model) applyTheme(name string) {
	if m.theme != nil && m.theme.Name == name {
		return
	}
	th := m.pickThemes[name]
	if th == nil {
		th, _ = theme.Get(name)
		if th != nil && m.pickThemes != nil {
			m.pickThemes[name] = th
		}
	}
	if th != nil {
		m.SetTheme(th)
	}
}

// saveTheme writes name to the config unless the config already holds it
// (or will, once queued saves finish). It compares with the config rather
// than the theme on screen, which --theme or an unknown configured theme
// can make differ.
func (m *Model) saveTheme(name string) tea.Cmd {
	s := m.themeSaves
	if s == nil || name == s.target() {
		return nil
	}
	if s.inFlight != "" {
		s.pending = name
		return nil
	}
	s.inFlight = name
	save := s.save
	return func() tea.Msg {
		return ThemeSaved{Name: name, Err: save(name)}
	}
}

// handleThemeSaved records a finished save and starts the pending one, if
// any; only the last save's result is reported. The theme stays applied for
// the session either way.
func (m *Model) handleThemeSaved(msg ThemeSaved) tea.Cmd {
	s := m.themeSaves
	if s == nil || msg.Name != s.inFlight {
		return nil
	}
	s.inFlight = ""
	if msg.Err == nil {
		s.saved = msg.Name
	}
	if next := s.pending; next != "" {
		s.pending = ""
		return m.saveTheme(next)
	}
	if msg.Err == nil {
		m.logDebug("theme saved", "theme", msg.Name)
		m.clearConfigErrors()
		return nil
	}
	m.log.Warn("theme save failed", "theme", msg.Name, "err", msg.Err)
	// The status bar already labels the error "config".
	reason := strings.TrimPrefix(msg.Err.Error(), "config: ")
	text := fmt.Sprintf("theme set for this session · couldn't save: %s", reason)
	if errors.Is(msg.Err, config.ErrManualEdit) {
		text = fmt.Sprintf("theme set for this session · add theme = %q under [ui] in your config", msg.Name)
	}
	m.recordError(domain.ErrorKindConfig, errors.New(text), "")
	return nil
}

// ReportUnknownTheme shows that the configured theme doesn't exist. Saving a
// theme from the picker fixes the config, which clears the message.
func (m *Model) ReportUnknownTheme(name string) {
	err := fmt.Errorf("unknown theme %q, using the default (pho -h lists themes)", name)
	m.unknownTheme = err.Error()
	m.ReportConfigProblem(err)
}

// clearConfigErrors runs after a successful save: it drops an earlier failed
// save's error and the unknown-theme problem, since the config now names a
// real theme. Other config problems stay.
func (m *Model) clearConfigErrors() {
	if m.unknownTheme != "" {
		problems := m.configProblems[:0:0]
		for _, p := range m.configProblems {
			if p.Message != m.unknownTheme {
				problems = append(problems, p)
			}
		}
		m.configProblems, m.unknownTheme = problems, ""
	}
	isProblem := func(e domain.AppError) bool {
		for _, p := range m.configProblems {
			if p == e {
				return true
			}
		}
		return false
	}
	errs := m.state.Errors.Errors[:0:0]
	for _, e := range m.state.Errors.Errors {
		if e.Kind != domain.ErrorKindConfig || isProblem(e) {
			errs = append(errs, e)
		}
	}
	if len(errs) != len(m.state.Errors.Errors) {
		m.state.Errors.Errors = errs
		m.syncStatus()
	}
}
