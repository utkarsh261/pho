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

const (
	updateCheckEvery = 24 * time.Hour
	updateRetryEvery = time.Hour
)

var updateTimeout = release.Timeout

// scheduleUpdateCheck asks for another check after d; tests replace it.
var scheduleUpdateCheck = func(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return updateCheckDue{} })
}

type UpdateChecked struct {
	Latest  string
	Err     error
	SaveErr error
	At      time.Time
}

type updateCheckDue struct{}

type UpdateFinished struct {
	Err error
}

const brewUpgradeScript = `"$1" upgrade "$2"; s=$?; trap 'exit $s' INT; printf '\nPress Enter to return to pho'; read _; exit $s`

// BrewUpgrade runs brew with the TUI suspended, then waits for Enter so its
// output can be read. Ctrl-C at that prompt still reports brew's own status.
func BrewUpgrade(brew string) func() tea.Cmd {
	return func() tea.Cmd {
		c := exec.Command("sh", "-c", brewUpgradeScript, "sh", brew, release.BrewFormula)
		return tea.ExecProcess(c, func(err error) tea.Msg { return UpdateFinished{Err: err} })
	}
}

// checkForUpdatesCmd checks when the last successful check is a day old, or
// in the future because the clock moved back. Only a success is recorded, so
// a failed check is retried.
func (m *Model) checkForUpdatesCmd() tea.Cmd {
	d := m.deps
	if !d.CheckForUpdates || d.FetchLatest == nil || m.updated {
		return nil
	}
	now := m.now()
	if last := m.lastUpdateCheck; !last.IsZero() && !now.Before(last) && now.Sub(last) < updateCheckEvery {
		return scheduleUpdateCheck(updateCheckEvery - now.Sub(last))
	}
	store := d.State
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
		defer cancel()
		latest, err := fetchLatest(ctx, d.FetchLatest)
		msg := UpdateChecked{Latest: latest.Version, Err: err, At: now}
		if err == nil && store != nil {
			msg.SaveErr = store.Update(func(s *state.State) {
				s.LastUpdateCheck = now
				s.LatestKnown = latest.Version
			})
		}
		return msg
	}
}

func fetchLatest(ctx context.Context, fetch func(context.Context) (release.Release, error)) (r release.Release, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("update check: panic: %v", p)
		}
	}()
	return fetch(ctx)
}

func (m *Model) handleUpdateChecked(msg UpdateChecked) tea.Cmd {
	if msg.Err != nil {
		m.logWarn("update check failed", "err", msg.Err)
		return scheduleUpdateCheck(updateRetryEvery)
	}
	if msg.SaveErr != nil {
		m.logWarn("could not save the update check", "err", msg.SaveErr)
	}
	m.lastUpdateCheck = msg.At
	m.latestKnown = msg.Latest
	m.syncStatus()
	return scheduleUpdateCheck(updateCheckEvery)
}

func (m *Model) updateAvailable() string {
	if !m.deps.CheckForUpdates || m.updated {
		return ""
	}
	if release.Compare(m.latestKnown, m.deps.Version) <= 0 {
		return ""
	}
	return release.Normalize(m.latestKnown)
}

func (m *Model) canUpdate() bool {
	return m.deps.RunUpdate != nil && m.updateAvailable() != ""
}

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

// notice joins the announcement and the update notice. An upgrade's result
// goes first, so a narrow status bar cuts the announcement instead.
func (m *Model) notice() string {
	parts := []string{m.announcement(), m.updateNotice()}
	if m.updateResult != "" {
		parts[0], parts[1] = parts[1], parts[0]
	}
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " | ")
}

func (m *Model) updateConfirmHint() string {
	return fmt.Sprintf("Update pho to %s with brew upgrade %s? | Enter: Update | Esc: Cancel",
		m.updateAvailable(), release.BrewFormula)
}

func (m *Model) startUpdate() tea.Cmd {
	if !m.canUpdate() || m.currentView() != domain.PrimaryViewDashboard ||
		m.state.Search.OverlayOpen || m.keymapOverlay.Visible || m.whatsNew.Visible ||
		(m.createPR != nil && m.createPR.Active()) {
		return nil
	}
	m.updateConfirm = true
	m.syncStatus()
	return nil
}

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
		// The next launch announces the new version.
		m.updated = true
		m.updateResult = fmt.Sprintf("✓ Updated to %s · restart pho to use it", m.updateTarget)
	}
	m.syncStatus()
}
