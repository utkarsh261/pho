package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
	"github.com/utkarsh261/pho/internal/ui/markdown"
)

// changelogReleases is how many releases `pho --changelog` prints.
const changelogReleases = 5

// updatesOptedOut reports whether PHO_NO_UPDATE_CHECK turns off pho's
// automatic release features.
func updatesOptedOut() bool {
	v := strings.TrimSpace(os.Getenv("PHO_NO_UPDATE_CHECK"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// existingUser reports whether pho has run here before, judged by files an
// earlier version leaves behind. It tells an upgrade from a version that
// kept no state apart from a fresh install.
func existingUser(configFile, cacheDir string) bool {
	for _, p := range []string{configFile, filepath.Join(cacheDir, "cache.db")} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// planLaunch decides whether this launch announces an upgrade, and records
// the decision. It never fails: a state file that can't be read or written
// only means the announcement may repeat.
func planLaunch(store *state.Store, version string, existing, enabled bool, warn func(string, ...any)) state.Announcement {
	var a state.Announcement
	if err := store.Update(func(s *state.State) {
		a = s.PlanLaunch(version, existing, enabled)
	}); err != nil {
		warn("could not save state", "path", store.Path(), "err", err)
	}
	return a
}

// printChangelog prints the newest release notes up to version, falling
// back to the releases link when they can't be fetched.
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

// stdoutWidth is the terminal width for --changelog, 80 when unknown.
func stdoutWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return min(w, 100)
	}
	return 80
}
