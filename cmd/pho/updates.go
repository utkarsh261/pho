package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
	"github.com/utkarsh261/pho/internal/ui/app"
	"github.com/utkarsh261/pho/internal/ui/markdown"
)

// changelogReleases is how many releases `pho --changelog` prints.
const changelogReleases = 5

func updatesOptedOut() bool {
	v := strings.TrimSpace(os.Getenv("PHO_NO_UPDATE_CHECK"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// existingUser reports whether an earlier pho has run here.
func existingUser(configFile, cacheDir string) bool {
	for _, p := range []string{configFile, filepath.Join(cacheDir, "cache.db")} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// planLaunch never fails, and returns the state it read. A launch it can't
// record announces nothing, so an unwritable state file can't repeat the
// notice forever.
func planLaunch(store *state.Store, version string, existing, enabled bool, warn func(string, ...any)) (state.Announcement, state.State) {
	var (
		a  state.Announcement
		st state.State
	)
	if err := store.Update(func(s *state.State) {
		a = s.PlanLaunch(version, existing, enabled)
		st = *s
	}); err != nil {
		warn("could not save state", "path", store.Path(), "err", err)
		return state.Announcement{}, st
	}
	return a, st
}

// updateCheckEnabled is off for dev builds and when stdout isn't a terminal.
func updateCheckEnabled(configured bool, version string, stdoutTTY bool) bool {
	return configured && !updatesOptedOut() && release.IsRelease(version) && stdoutTTY
}

// brewUpdater is nil unless pho was installed with Homebrew.
func brewUpdater() func() tea.Cmd {
	brew, ok := release.DetectBrew(os.Executable, exec.LookPath)
	if !ok {
		return nil
	}
	return app.BrewUpgrade(brew)
}

// printChangelog falls back to the releases link on any error.
func printChangelog(w io.Writer, c *release.Client, version string, width int) {
	rs, err := c.List(context.Background(), 20)
	if err == nil {
		rs = release.Recent(rs, version, changelogReleases)
	}
	if err != nil || len(rs) == 0 {
		fmt.Fprintf(w, "Couldn't load release notes. Read them at %s\n", release.ReleasesURL)
		return
	}
	lines := markdown.New().Render(release.Markdown(rs), width)
	fmt.Fprintln(w, strings.Join(lines, "\n"))
	fmt.Fprintf(w, "\nAll releases: %s\n", release.ReleasesURL)
}

func stdoutWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return min(w, 100)
	}
	return 80
}
