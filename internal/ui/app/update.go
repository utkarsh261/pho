package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
)

// updateCheckEvery is how long a check result is trusted before asking
// GitHub again.
const updateCheckEvery = 24 * time.Hour

// updateTimeout bounds the update check; tests shorten it.
var updateTimeout = release.Timeout

// UpdateChecked carries the result of the background update check.
type UpdateChecked struct {
	Latest string
	Err    error
}

// UpdateFinished reports a `brew upgrade` run from U.
type UpdateFinished struct {
	Err error
}

// BrewUpgrade returns the command U runs: brew upgrades pho in the normal
// terminal while the TUI is suspended, then waits for Enter so its output
// can be read.
func BrewUpgrade(brew string) func() tea.Cmd {
	const script = `"$1" upgrade "$2"; s=$?; printf '\nPress Enter to return to pho'; read _; exit $s`
	return func() tea.Cmd {
		c := exec.Command("sh", "-c", script, "sh", brew, release.BrewFormula)
		return tea.ExecProcess(c, func(err error) tea.Msg { return UpdateFinished{Err: err} })
	}
}

// checkForUpdatesCmd asks GitHub for the latest release when the last check
// is more than a day old, and records what it found. A failure still stamps
// the check time, so an offline machine isn't retried every launch, and
// keeps the version learned before.
func (m *Model) checkForUpdatesCmd() tea.Cmd {
	d := m.deps
	if !d.CheckForUpdates || d.FetchLatest == nil || !release.IsRelease(d.Version) {
		return nil
	}
	now := m.now()
	if !d.LastUpdateCheck.IsZero() && now.Sub(d.LastUpdateCheck) < updateCheckEvery {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
		defer cancel()
		latest, err := fetchLatest(ctx, d.FetchLatest)
		if d.State != nil {
			if serr := d.State.Update(func(s *state.State) {
				s.LastUpdateCheck = now
				if err == nil {
					s.LatestKnown = latest.Version
				}
			}); serr != nil {
				m.logWarn("could not save the update check", "err", serr)
			}
		}
		return UpdateChecked{Latest: latest.Version, Err: err}
	}
}

// fetchLatest calls fetch, turning a panic into an error.
func fetchLatest(ctx context.Context, fetch func(context.Context) (release.Release, error)) (r release.Release, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("update check: panic: %v", p)
		}
	}()
	return fetch(ctx)
}

func (m *Model) handleUpdateChecked(msg UpdateChecked) {
	if msg.Err != nil {
		m.logWarn("update check failed", "err", msg.Err)
		return
	}
	m.latestKnown = msg.Latest
	m.syncStatus()
}

// updateAvailable is the newer version to mention, or "".
func (m *Model) updateAvailable() string {
	if !m.deps.CheckForUpdates || m.updated || !release.IsRelease(m.deps.Version) || !release.IsRelease(m.latestKnown) {
		return ""
	}
	if release.Compare(m.latestKnown, m.deps.Version) <= 0 {
		return ""
	}
	return release.Normalize(m.latestKnown)
}

// canUpdate reports whether U can upgrade pho in place.
func (m *Model) canUpdate() bool {
	return m.deps.RunUpdate != nil && m.updateAvailable() != ""
}

// updateNotice is the status-bar line about a newer release, or the
// outcome of an upgrade run from U.
func (m *Model) updateNotice() string {
	if m.updateResult != "" {
		return m.updateResult
	}
	v := m.updateAvailable()
	switch {
	case v == "":
		return ""
	case m.canUpdate():
		return fmt.Sprintf("pho %s available | U: Update", v)
	default:
		return fmt.Sprintf("pho %s available · %s", v, strings.TrimPrefix(release.ReleasesURL, "https://"))
	}
}

func (m *Model) updateConfirmHint() string {
	return fmt.Sprintf("Update pho to %s with brew upgrade %s? | Enter: Update | Esc: Cancel",
		m.updateAvailable(), release.BrewFormula)
}

// startUpdate asks to confirm the upgrade in the status bar.
func (m *Model) startUpdate() tea.Cmd {
	if !m.canUpdate() || m.currentView() != domain.PrimaryViewDashboard {
		return nil
	}
	m.updateConfirm = true
	m.syncStatus()
	return nil
}

// handleUpdateConfirmKey runs the upgrade on Enter; any other key cancels.
func (m *Model) handleUpdateConfirmKey(msg tea.KeyMsg) tea.Cmd {
	m.updateConfirm = false
	defer m.syncStatus()
	if msg.Type != tea.KeyEnter || !m.canUpdate() {
		return nil
	}
	m.updateTarget = m.updateAvailable()
	m.updateResult = ""
	return m.deps.RunUpdate()
}

func (m *Model) handleUpdateFinished(msg UpdateFinished) {
	if msg.Err != nil {
		m.logWarn("brew upgrade failed", "err", msg.Err)
		code := -1
		var exitErr *exec.ExitError
		if errors.As(msg.Err, &exitErr) {
			code = exitErr.ExitCode()
		}
		if code >= 0 {
			m.updateResult = fmt.Sprintf("brew upgrade failed (exit %d) | U: Retry", code)
		} else {
			m.updateResult = "brew upgrade failed | U: Retry"
		}
	} else {
		// The new version announces itself on the next launch, since
		// the last version seen is still this one.
		m.updated = true
		m.updateResult = fmt.Sprintf("✓ Updated to %s · restart pho to use it", m.updateTarget)
	}
	m.syncStatus()
}
