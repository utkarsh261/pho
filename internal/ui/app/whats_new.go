package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
)

const (
	whatsNewReleases = 20
	whatsNewShown    = 5
)

var whatsNewTimeout = release.Timeout

// stateSaveFailed carries a background state write's error back to the
// update loop, which logs it.
type stateSaveFailed struct {
	what string
	err  error
}

type WhatsNewLoaded struct {
	Releases []release.Release
	Err      error
}

func (m *Model) announcement() string {
	if m.announce == "" {
		return ""
	}
	return fmt.Sprintf("✦ Updated to %s | W: What's new", m.announce)
}

// openWhatsNew counts as seeing the current version's notes.
func (m *Model) openWhatsNew() tea.Cmd {
	if m.state.Search.OverlayOpen || m.keymapOverlay.Visible ||
		(m.createPR != nil && m.createPR.Active()) {
		return nil
	}
	m.whatsNew.Open()
	m.announce = ""
	m.syncStatus()

	var out []tea.Cmd
	if store, version := m.deps.State, m.deps.Version; store != nil && release.IsRelease(version) {
		out = append(out, func() tea.Msg {
			if err := store.Update(func(s *state.State) { s.MarkSeen(version) }); err != nil {
				return stateSaveFailed{what: "version seen", err: err}
			}
			return nil
		})
	}
	if !m.whatsNew.Loaded() && !m.whatsNewLoading && m.deps.FetchReleases != nil {
		m.whatsNewLoading = true
		out = append(out, fetchReleasesCmd(m.deps.FetchReleases))
	} else if !m.whatsNew.Loaded() && m.deps.FetchReleases == nil {
		m.whatsNew.SetError()
	}
	return batch(out...)
}

func fetchReleasesCmd(fetch func(context.Context, int) ([]release.Release, error)) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = WhatsNewLoaded{Err: fmt.Errorf("fetch releases: panic: %v", r)}
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), whatsNewTimeout)
		defer cancel()
		rs, err := fetch(ctx, whatsNewReleases)
		return WhatsNewLoaded{Releases: rs, Err: err}
	}
}

func (m *Model) handleWhatsNewLoaded(msg WhatsNewLoaded) {
	m.whatsNewLoading = false
	if msg.Err != nil {
		m.logWarn("could not load release notes", "err", msg.Err)
		m.whatsNew.SetError()
		return
	}
	shown := release.Between(msg.Releases, m.deps.WhatsNewSince, m.deps.Version, whatsNewShown)
	if len(shown) == 0 {
		shown = release.Recent(msg.Releases, m.deps.Version, whatsNewShown)
	}
	m.whatsNew.SetReleases(shown)
}
