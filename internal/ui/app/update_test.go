package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
	pholog "github.com/utkarsh261/pho/internal/log"
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
// upgrades U started; scheduled checks are recorded in m's test hook.
func newUpdateModel(t *testing.T, s updateSetup) (*Model, *state.Store, *int) {
	t.Helper()
	recordSchedules(t)
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
	m.lastUpdateCheck = s.lastCheck
	m.announce = s.announce
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	return m, store, runs
}

var scheduled []time.Duration

// recordSchedules stops checks from sleeping in tests and records their delays.
func recordSchedules(t *testing.T) {
	old := scheduleUpdateCheck
	scheduled = nil
	scheduleUpdateCheck = func(d time.Duration) tea.Cmd {
		scheduled = append(scheduled, d)
		return nil
	}
	t.Cleanup(func() { scheduleUpdateCheck = old })
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
	if len(scheduled) != 1 || scheduled[0] != updateCheckEvery {
		t.Errorf("scheduled %v, want the next check in a day", scheduled)
	}

	// A long session checks again when that comes due.
	m.deps.Now = func() time.Time { return fixedNow().Add(updateCheckEvery) }
	m.deps.FetchLatest = latest("1.4.0")
	drainCmds(m, func() tea.Msg { return updateCheckDue{} })
	if !strings.Contains(statusText(m), "pho 1.4.0 available") {
		t.Errorf("status bar = %q, want the second check's result", statusText(m))
	}
}

func TestUpdateCheck_SkippedWithinADay(t *testing.T) {
	called := false
	fetch := func(context.Context) (release.Release, error) { called = true; return release.Release{}, nil }
	m, _, _ := newUpdateModel(t, updateSetup{
		version: "1.2.0", latestKnown: "1.3.0", lastCheck: fixedNow().Add(-time.Hour), fetch: fetch,
	})
	drainCmds(m, m.checkForUpdatesCmd())
	if called {
		t.Error("checked again within a day")
	}
	if len(scheduled) != 1 || scheduled[0] != 23*time.Hour {
		t.Errorf("scheduled %v, want the check when the day is up", scheduled)
	}
	if !strings.Contains(statusText(m), "pho 1.3.0 available") {
		t.Errorf("cached result not shown: %q", statusText(m))
	}
}

func TestUpdateCheck_Due(t *testing.T) {
	for name, last := range map[string]time.Time{
		"after a day":          fixedNow().Add(-25 * time.Hour),
		"clock moved back":     fixedNow().Add(48 * time.Hour),
		"never checked before": {},
	} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", lastCheck: last, fetch: latest("1.3.0")})
			drainCmds(m, m.checkForUpdatesCmd())
			if !strings.Contains(statusText(m), "pho 1.3.0 available") {
				t.Errorf("didn't check: %q", statusText(m))
			}
		})
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
			if !st.LastUpdateCheck.IsZero() {
				t.Errorf("LastUpdateCheck = %v, want a failed check left unrecorded", st.LastUpdateCheck)
			}
			if len(scheduled) != 1 || scheduled[0] != updateRetryEvery {
				t.Errorf("scheduled %v, want a retry in an hour", scheduled)
			}
		})
	}
}

func TestUpdateCheck_Off(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", fetch: latest("1.3.0")})
	m.deps.CheckForUpdates = false
	m.syncStatus()
	if m.checkForUpdatesCmd() != nil || m.status.Notice != "" {
		t.Errorf("check off but cmd or notice %q present", m.status.Notice)
	}
}

func TestUpdateNotice_NotForSameOrOlder(t *testing.T) {
	for _, v := range []string{"1.2.0", "1.1.0"} {
		m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: v})
		if m.status.Notice != "" {
			t.Errorf("latest %s: notice %q", v, m.status.Notice)
		}
	}
}

func TestUpdateNotice_ShownWithAnnouncement(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", announce: "1.2.0", brew: true})
	if got := statusText(m); !strings.Contains(got, "Updated to 1.2.0") || !strings.Contains(got, "1.3.0 available") {
		t.Errorf("status bar = %q, want the announcement and the update notice", got)
	}

	key(m, keyU)
	drainCmds(m, key(m, keyEnter))
	got := statusText(m)
	if !strings.Contains(got, "✓ Updated to 1.3.0") {
		t.Errorf("status bar = %q, want the brew result next to the announcement", got)
	}
	if strings.Index(got, "✓ Updated to 1.3.0") > strings.Index(got, "Updated to 1.2.0") {
		t.Errorf("status bar = %q, want the brew result first", got)
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

func TestUpdate_FromKeymapOverlay(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", brew: true})
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.keymapOverlay.Visible {
		t.Fatal("? didn't open the keymap overlay")
	}
	key(m, keyU)
	if m.keymapOverlay.Visible || !m.updateConfirm {
		t.Errorf("overlay %v, confirm %v; want U to close the overlay and ask", m.keymapOverlay.Visible, m.updateConfirm)
	}
}

func TestUpdate_NotOverOtherOverlays(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0", latestKnown: "1.3.0", brew: true})
	m.whatsNew.Open()
	m.startUpdate()
	if m.updateConfirm {
		t.Error("U asked to confirm over the What's new panel")
	}
}

func TestBrewUpgradeScript_CtrlCAtPromptKeepsStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	// The fake brew succeeds, then interrupts the shell once it waits at
	// the prompt.
	brew := filepath.Join(t.TempDir(), "brew")
	os.WriteFile(brew, []byte("#!/bin/sh\n(sleep 0.3; kill -INT $PPID) &\nexit 0\n"), 0o755)
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	c := exec.Command("sh", "-c", brewUpgradeScript, "sh", brew, release.BrewFormula)
	c.Stdin = r
	if err := c.Run(); err != nil {
		t.Errorf("Ctrl-C at the prompt after a good upgrade: %v, want success", err)
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

func TestUpdateCheck_WarnsOncePerFailingStreak(t *testing.T) {
	m, _, _ := newUpdateModel(t, updateSetup{version: "1.2.0"})
	logFile := filepath.Join(t.TempDir(), "debug.log")
	m.log = pholog.New(logFile, "warn")

	offline := UpdateChecked{Err: errors.New("offline"), At: fixedNow()}
	for range 3 {
		m.Update(offline)
	}
	m.Update(UpdateChecked{Latest: "1.2.0", At: fixedNow()})
	m.Update(offline)

	data, _ := os.ReadFile(logFile)
	if n := strings.Count(string(data), "update check failed"); n != 2 {
		t.Errorf("%d warnings, want one per failing streak:\n%s", n, data)
	}
}
