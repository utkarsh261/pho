package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestKeymapOverlayDirectKeys checks the keys marked Direct in the help
// overlay close it and run, and that other keys keep their old behaviour.
func TestKeymapOverlayDirectKeys(t *testing.T) {
	keyHelp := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}

	t.Run("T on the dashboard opens the theme picker", func(t *testing.T) {
		m := ansiDashboard(t, 120, 40)
		ansiKeys(m, "?")
		key(m, keyT)
		if m.keymapOverlay.Visible || !m.themePicking() {
			t.Errorf("overlay visible %v, picking %v; want closed and picking", m.keymapOverlay.Visible, m.themePicking())
		}
	})

	t.Run("ctrl+p on the dashboard opens the palette", func(t *testing.T) {
		m := ansiDashboard(t, 120, 40)
		ansiKeys(m, "?")
		key(m, tea.KeyMsg{Type: tea.KeyCtrlP})
		if m.keymapOverlay.Visible || !m.state.Search.OverlayOpen || m.themePicking() {
			t.Error("ctrl+p from the overlay did not open the jump palette")
		}
	})

	t.Run("T in PR detail opens the theme picker", func(t *testing.T) {
		m := ansiPRDetail(t, "2")
		drainCmds(m, key(m, keyHelp))
		if !m.keymapOverlay.Visible {
			t.Fatal("? did not open the help overlay")
		}
		key(m, keyT)
		if m.keymapOverlay.Visible || !m.themePicking() {
			t.Error("T from the PR detail overlay did not open the picker")
		}
	})

	t.Run("ctrl+p in PR detail is not bound there", func(t *testing.T) {
		m := ansiPRDetail(t, "2")
		drainCmds(m, key(m, keyHelp))
		key(m, tea.KeyMsg{Type: tea.KeyCtrlP})
		if !m.keymapOverlay.Visible || m.state.Search.OverlayOpen {
			t.Error("ctrl+p acted from the PR detail overlay, where it has no binding")
		}
	})

	t.Run("q still only closes", func(t *testing.T) {
		m := ansiDashboard(t, 120, 40)
		ansiKeys(m, "?")
		if cmd := key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatal("q in the help overlay quit pho")
			}
		}
		if m.keymapOverlay.Visible {
			t.Error("q did not close the overlay")
		}
	})
}
