package createpr

import (
	"io"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// submitButton is a custom huh field that renders a "Create PR" button and
// advances the form when Enter or Tab is pressed.
type submitButton struct {
	focused bool
	width   int
	height  int
	theme   *huh.Theme
	keymap  huh.NoteKeyMap
}

func (s *submitButton) Init() tea.Cmd { return nil }
func (s *submitButton) Focus() tea.Cmd {
	s.focused = true
	return nil
}
func (s *submitButton) Blur() tea.Cmd {
	s.focused = false
	return nil
}
func (s *submitButton) Error() error                             { return nil }
func (s *submitButton) Skip() bool                               { return false }
func (s *submitButton) Zoom() bool                               { return false }
func (s *submitButton) GetKey() string                           { return "submit" }
func (s *submitButton) GetValue() any                            { return nil }
func (s *submitButton) Run() error                               { return nil }
func (s *submitButton) RunAccessible(io.Writer, io.Reader) error { return nil }

func (s *submitButton) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, s.keymap.Prev):
			return s, huh.PrevField
		case key.Matches(msg, s.keymap.Next, s.keymap.Submit):
			return s, huh.NextField
		}
		return s, nil
	}
	return s, nil
}

func (s *submitButton) View() string {
	th := s.theme
	if th == nil {
		th = huh.ThemeBase()
	}
	label := "Create PR"
	var button string
	if s.focused {
		button = th.Focused.FocusedButton.Render(label)
	} else {
		button = th.Blurred.BlurredButton.Render(label)
	}
	if s.width > 0 {
		return lipgloss.PlaceHorizontal(s.width, lipgloss.Center, button)
	}
	return button
}

func (s *submitButton) KeyBinds() []key.Binding {
	return []key.Binding{s.keymap.Prev, s.keymap.Next, s.keymap.Submit}
}

func (s *submitButton) WithTheme(t *huh.Theme) huh.Field {
	s.theme = t
	return s
}
func (s *submitButton) WithAccessible(bool) huh.Field      { return s }
func (s *submitButton) WithKeyMap(k *huh.KeyMap) huh.Field { s.keymap = k.Note; return s }
func (s *submitButton) WithWidth(w int) huh.Field          { s.width = w; return s }
func (s *submitButton) WithHeight(h int) huh.Field         { s.height = h; return s }
func (s *submitButton) WithPosition(p huh.FieldPosition) huh.Field {
	s.keymap.Prev.SetEnabled(!p.IsFirst())
	s.keymap.Next.SetEnabled(!p.IsLast())
	s.keymap.Submit.SetEnabled(p.IsLast())
	return s
}
