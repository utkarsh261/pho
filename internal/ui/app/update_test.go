package app

import (
	"context"
	"errors"
	"os/exec"
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

var keyU = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}}

type updateSetup struct {
	version     string
	latestKnown string
	lastCheck   time.Time
	fetch       func(context.Context) (release.Release, error)
	brew        bool
	runErr      error
	announce    string
}

// newUpdateModel builds a model with the update check on. runs counts the
// upgrades U started.
func newUpdateModel(t *testing.T, s updateSetup) (*Model, *state.Store, *int) {
	t.Helper()
	store := state.NewStore(filepath.Join(t.TempDir(), "pho", "state.json"))
	m := newTestModel(nil, nil)
	runs := new(int)
	m.deps.Version = s.version
	m.deps.State = store
	m.deps.CheckForUpdates = true
	m.deps.LatestKnown = s.latestKnown
	m.deps.LastUpdateCheck = s.lastCheck
	m.deps.FetchLatest = s.fetch
	if s.brew {
		m.deps.RunUpdate = func() tea.Cmd {
			*runs++
			return func() tea.Msg { return UpdateFinished{Err: s.runErr} }
		}
	}
	m.latestKnown = s.latestKnown
	m.announce = s.announce
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	return m, store, runs
}

func latest(v string) func(context.Context) (release.Release, error) {
	return func(context.Context) (release.Release, error) { return release.Release{Version: v}, nil }
}

func statusText(m *Model) string {
	return ansi.Strip(m.status.View())
}

func TestUpdateCheck_FindsNewerRelease(t *testing.T) {
	m, store, _ := newUpdateModel(t, updateSetup{version: "1.2.0", fetch: latest("1.3.0")})
	drainCmds(m, m.checkForUpdatesCmd())

	if got := statusText(m); !strings.Contains(got, "pho 1.3.0 available") || !strings.Contains(got, "github.com/utkarsh261/pho/releases") {
		t.Errorf("status bar = %q, want the notice with the releases link", got)
	}
	if strings.Contains(statusText(m), "U: Update") {
		t.Error("non-brew install offers U")
	}
	st, _ := store.Load()
	if st.LatestKnown != "1.3.0" || !st.LastUpdateCheck.Equal(fixedNow()) {
		t.Errorf("state = %+v, want latest 1.3.0 checked at %v", st, fixedNow())
	}
}

func TestUpdateCheck_SkippedWithinADay(t *testing.T) {
	called := false
	fetch := func(context.Context) (release.Release, error) { called = true; return release.Release{}, nil }
	m, _, _ := newUpdateModel(t, updateSetup{
		version: "1.2.0", latestKnown: "1.3.0", lastCheck: fixedNow().Add(-time.Hour), fetch: fetch,
	})
	if cmd := m.checkForUpdatesCmd(); cmd != nil {
		drainCmds(m, cmd)
	}
	if called {
		t.Error("checked again within a day")
	}
	if !strings.Contains(statusText(m), "pho 1.3.0 available") {
		t.Errorf("cached result not shown: %q", statusText(m))
	}
}

func TestUpdateCheck_DueAfterADay(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", lastCheck: fixedNow().Add(-25 * time.Hour), fetch: latest("1.3.0")})
	if m.checkForUpdatesCmd() == nil {
		t.Error("no check after more than a day")
	}
}

func TestUpdateCheck_FailuresKeepWhatWasKnown(t *testing.T) {
	old := updateTimeout
	updateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { updateTimeout = old })

	fetches := map[string]func(context.Context) (release.Release, error){
		"error": func(context.Context) (release.Release, error) { return release.Release{}, errors.New("offline") },
		"panic": func(context.Context) (release.Release, error) { panic("boom") },
		"hangs until cancelled": func(ctx context.Context) (release.Release, error) {
			<-ctx.Done()
			return release.Release{}, ctx.Err()
		},
	}
	for name, fetch := range fetches {
		t.Run(name, func(t *testing.T) {
			m, store, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", fetch: fetch})
			store.Update(func(s *state.State) { s.LatestKnown = "1.3.0" })
			drainCmds(m, m.checkForUpdatesCmd())

			if len(m.state.Errors.Errors) != 0 {
				t.Errorf("check failure reached the status bar: %+v", m.state.Errors.Errors)
			}
			if !strings.Contains(statusText(m), "pho 1.3.0 available") {
				t.Errorf("failed check erased the notice: %q", statusText(m))
			}
			st, _ := store.Load()
			if st.LatestKnown != "1.3.0" {
				t.Errorf("LatestKnown = %q, want 1.3.0 kept", st.LatestKnown)
			}
			if !st.LastUpdateCheck.Equal(fixedNow()) {
				t.Errorf("LastUpdateCheck = %v, want it stamped so offline machines aren't retried", st.LastUpdateCheck)
			}
		})
	}
}

func TestUpdateCheck_Off(t *testing.T) {
	for name, s := range map[string]updateSetup{
		"dev build":  {version: "dev", fetch: latest("1.3.0")},
		"prerelease": {version: "1.3.0-rc.1", fetch: latest("1.3.0")},
	} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := newUpdateModel(t, s)
			if m.checkForUpdatesCmd() != nil {
				t.Error("check would run")
			}
		})
	}
	t.Run("turned off", func(t *testing.T) {
		m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", fetch: latest("1.3.0")})
		m.deps.CheckForUpdates = false
		m.syncStatus()
		if m.checkForUpdatesCmd() != nil || m.status.Notice != "" {
			t.Errorf("check off but cmd or notice %q present", m.status.Notice)
		}
	})
}

func TestUpdateNotice_NotForSameOrOlder(t *testing.T) {
	for _, v := range []string{"1.2.0", "1.1.0"} {
		m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: v})
		if m.status.Notice != "" {
			t.Errorf("latest %s: notice %q", v, m.status.Notice)
		}
	}
}

func TestUpdateNotice_AnnouncementWins(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", announce: "1.2.0"})
	if got := statusText(m); !strings.Contains(got, "Updated to 1.2.0") || strings.Contains(got, "1.3.0 available") {
		t.Errorf("status bar = %q, want only the announcement", got)
	}
	m.announce = ""
	m.syncStatus()
	if !strings.Contains(statusText(m), "1.3.0 available") {
		t.Error("update notice missing once the announcement is gone")
	}
}

func TestUpdateNotice_OnlyOnDashboard(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", brew: true})
	m.pushView(domain.PrimaryViewPRDetail)
	m.syncStatus()
	if m.status.Notice != "" {
		t.Errorf("notice %q in PR detail", m.status.Notice)
	}
}

func TestUpdate_BrewConfirmAndRun(t *testing.T) {
	m, _, runs := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", brew: true})
	if !strings.Contains(statusText(m), "U: Update") {
		t.Fatalf("brew install should offer U: %q", statusText(m))
	}

	// U then Esc cancels.
	key(m, keyU)
	if got := statusText(m); !strings.Contains(got, "brew upgrade utkarsh261/tap/pho?") {
		t.Fatalf("U didn't ask to confirm: %q", got)
	}
	key(m, keyEsc)
	if *runs != 0 || m.updateConfirm {
		t.Fatalf("Esc ran %d upgrades, confirm %v", *runs, m.updateConfirm)
	}

	// Any other key cancels too, without acting on the dashboard.
	key(m, keyU)
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if *runs != 0 || m.themePicking() {
		t.Fatalf("T after U ran %d upgrades or opened the picker", *runs)
	}

	// U then Enter runs it once.
	key(m, keyU)
	drainCmds(m, key(m, keyEnter))
	if *runs != 1 {
		t.Fatalf("runs = %d, want 1", *runs)
	}
	if got := statusText(m); !strings.Contains(got, "✓ Updated to 1.3.0") || !strings.Contains(got, "restart pho") {
		t.Errorf("status bar after success = %q", got)
	}
	key(m, keyU)
	if m.updateConfirm {
		t.Error("U still offered after a successful upgrade")
	}
}

func TestUpdate_BrewFailureOffersRetry(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	m, _, runs := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", brew: true, runErr: exitErr})
	key(m, keyU)
	drainCmds(m, key(m, keyEnter))
	if got := statusText(m); !strings.Contains(got, "brew upgrade failed (exit 3)") || !strings.Contains(got, "U: Retry") {
		t.Errorf("status bar after failure = %q", got)
	}
	if len(m.state.Errors.Errors) != 0 {
		t.Errorf("failure went to StatusError: %+v", m.state.Errors.Errors)
	}
	key(m, keyU)
	drainCmds(m, key(m, keyEnter))
	if *runs != 2 {
		t.Errorf("retry ran %d times in total, want 2", *runs)
	}
}

func TestUpdate_UNoOpWithoutBrew(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0"})
	key(m, keyU)
	if m.updateConfirm {
		t.Error("U asked to confirm on a non-brew install")
	}
}

func TestUpdate_UNoOpWithoutNewerRelease(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.2.0", brew: true})
	key(m, keyU)
	if m.updateConfirm {
		t.Error("U asked to confirm with nothing to update to")
	}
}
