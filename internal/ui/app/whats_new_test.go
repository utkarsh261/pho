package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
)

var keyW = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}}

var testReleases = []release.Release{
	{Version: "1.3.0", Body: "## Changelog\n* aaaaaaa: Not out yet for this user (#3)"},
	{Version: "1.2.0", Body: "## Changelog\n* bbbbbbb: Add the W panel (#2) (@u)"},
	{Version: "1.1.0", Body: "## Changelog\n* ccccccc: Older change (#1)"},
}

type whatsNewSetup struct {
	version, announce, since string
	fetch                    func(context.Context, int) ([]release.Release, error)
}

func newWhatsNewModel(t *testing.T, s whatsNewSetup) (*Model, *state.Store) {
	t.Helper()
	store := state.NewStore(filepath.Join(t.TempDir(), "pho", "state.json"))
	m := newTestModel(nil, nil)
	m.deps.Version = s.version
	m.deps.WhatsNewSince = s.since
	m.deps.FetchReleases = s.fetch
	m.deps.State = store
	m.announce = s.announce
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, store
}

func fetchOK(context.Context, int) ([]release.Release, error) {
	return testReleases, nil
}

func TestWhatsNew_AnnouncementThenW(t *testing.T) {
	m, store := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", announce: "1.2.0", since: "1.1.0", fetch: fetchOK})

	if got := ansi.Strip(m.status.View()); !strings.Contains(got, "✦ Updated to 1.2.0") || !strings.Contains(got, "What's new") {
		t.Fatalf("status bar lacks the announcement: %q", got)
	}

	drainCmds(m, key(m, keyW))

	if !m.whatsNew.Visible {
		t.Fatal("W didn't open the panel")
	}
	if strings.Contains(ansi.Strip(m.status.View()), "Updated to") {
		t.Error("announcement still shown after W")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Add the W panel (#2)") {
		t.Errorf("panel doesn't show the 1.2.0 notes:\n%s", view)
	}
	if strings.Contains(view, "Not out yet") || strings.Contains(view, "Older change") {
		t.Errorf("panel shows releases outside 1.1.0..1.2.0:\n%s", view)
	}
	st, err := store.Load()
	if err != nil || st.LastSeenVersion != "1.2.0" {
		t.Errorf("state = %+v, %v; want LastSeenVersion 1.2.0", st, err)
	}

	key(m, keyEsc)
	if m.whatsNew.Visible {
		t.Error("Esc didn't close the panel")
	}
}

func TestWhatsNew_WithoutUpgradeShowsRecent(t *testing.T) {
	m, _ := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", fetch: fetchOK})
	drainCmds(m, key(m, keyW))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Add the W panel") || !strings.Contains(view, "Older change") || strings.Contains(view, "Not out yet") {
		t.Errorf("want 1.2.0 and older, nothing newer:\n%s", view)
	}
}

func TestWhatsNew_NonReleaseBuildLeavesStateAlone(t *testing.T) {
	for _, v := range []string{"dev", "1.2.0-rc.1"} {
		t.Run(v, func(t *testing.T) {
			m, store := newWhatsNewModel(t, whatsNewSetup{version: v, fetch: fetchOK})
			drainCmds(m, key(m, keyW))
			if !m.whatsNew.Visible {
				t.Fatal("W didn't open the panel")
			}
			if store.Exists() {
				st, _ := store.Load()
				t.Errorf("a %s build wrote state: %+v", v, st)
			}
			if !strings.Contains(ansi.Strip(m.View()), "Not out yet") {
				t.Error("dev build should show the newest releases")
			}
		})
	}
}

func TestWhatsNew_FetchFailuresStayInThePanel(t *testing.T) {
	old := whatsNewTimeout
	whatsNewTimeout = 50 * time.Millisecond
	t.Cleanup(func() { whatsNewTimeout = old })

	fetches := map[string]func(context.Context, int) ([]release.Release, error){
		"error": func(context.Context, int) ([]release.Release, error) {
			return nil, errors.New("offline")
		},
		"panic": func(context.Context, int) ([]release.Release, error) {
			panic("boom")
		},
		"hangs until cancelled": func(ctx context.Context, _ int) ([]release.Release, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	for name, fetch := range fetches {
		t.Run(name, func(t *testing.T) {
			m, store := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", announce: "1.2.0", since: "1.1.0", fetch: fetch})
			drainCmds(m, key(m, keyW))

			if len(m.state.Errors.Errors) != 0 {
				t.Errorf("fetch failure reached the status bar: %+v", m.state.Errors.Errors)
			}
			if !m.whatsNew.Failed() {
				t.Error("panel isn't in its error state")
			}
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "Couldn't load release notes.") || !strings.Contains(view, "github.com/utkarsh261/pho/releases") {
				t.Errorf("error panel missing message or link:\n%s", view)
			}
			// Seeing the panel still counts, even without notes.
			if st, _ := store.Load(); st.LastSeenVersion != "1.2.0" {
				t.Errorf("LastSeenVersion = %q, want 1.2.0", st.LastSeenVersion)
			}

			key(m, keyEsc)
			if m.whatsNew.Visible {
				t.Fatal("Esc didn't close the error panel")
			}
			if view := ansi.Strip(m.View()); strings.Contains(view, "Couldn't load") {
				t.Error("error text left on the dashboard after closing")
			}

			// Opening again retries.
			m.deps.FetchReleases = fetchOK
			drainCmds(m, key(m, keyW))
			if m.whatsNew.Failed() || !strings.Contains(ansi.Strip(m.View()), "Add the W panel") {
				t.Error("reopening didn't retry the fetch")
			}
		})
	}
}

func TestWhatsNew_UnwritableStateIsIgnored(t *testing.T) {
	m, _ := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", announce: "1.2.0", fetch: fetchOK})
	m.deps.State = state.NewStore(filepath.Join("/dev/null", "pho", "state.json"))
	drainCmds(m, key(m, keyW))
	if !m.whatsNew.Visible || len(m.state.Errors.Errors) != 0 {
		t.Errorf("visible %v, errors %+v; want the panel and no errors", m.whatsNew.Visible, m.state.Errors.Errors)
	}
}

func TestWhatsNew_AnnouncementOnlyOnDashboard(t *testing.T) {
	m, _ := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", announce: "1.2.0", fetch: fetchOK})
	if m.status.Notice == "" {
		t.Fatal("no announcement on the dashboard")
	}

	m.SetFocus(domain.FocusCmdPalette)
	if m.status.Notice != "" {
		t.Error("announcement shown while the palette has focus")
	}
	m.SetFocus(domain.FocusPRListPanel)

	m.pushView(domain.PrimaryViewPRDetail)
	m.syncStatus()
	if m.status.Notice != "" {
		t.Error("announcement shown in PR detail")
	}
	m.popView()
	m.syncStatus()
	if m.status.Notice == "" {
		t.Error("announcement gone after returning to the dashboard")
	}
}

func TestWhatsNew_NoAnnouncementByDefault(t *testing.T) {
	m, _ := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", fetch: fetchOK})
	if m.status.Notice != "" {
		t.Errorf("unexpected notice %q", m.status.Notice)
	}
}

func TestWhatsNew_DirectFromKeymapOverlay(t *testing.T) {
	m, _ := newWhatsNewModel(t, whatsNewSetup{version: "1.2.0", fetch: fetchOK})
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.keymapOverlay.Visible {
		t.Fatal("? didn't open the keymap overlay")
	}
	drainCmds(m, key(m, keyW))
	if m.keymapOverlay.Visible || !m.whatsNew.Visible {
		t.Errorf("keymap %v, whats new %v; want W to swap them", m.keymapOverlay.Visible, m.whatsNew.Visible)
	}
	// Keys go to the panel, not the dashboard, while it's open.
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if m.themePicking() {
		t.Error("T opened the theme picker under the panel")
	}
}
