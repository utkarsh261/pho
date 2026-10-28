package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
)

func TestPrintChangelog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[
			{"tag_name":"v1.3.0","body":"## Changelog\n* aaaaaaa: Newer (#3)"},
			{"tag_name":"v1.2.0","body":"## Changelog\n* bbbbbbb: Current (#2) (@u)"}
		]`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	printChangelog(&out, &release.Client{BaseURL: srv.URL}, "1.2.0", 80)
	got := ansi.Strip(out.String())
	if !strings.Contains(got, "v1.2.0") || !strings.Contains(got, "Current (#2)") {
		t.Errorf("changelog missing 1.2.0 notes:\n%s", got)
	}
	if strings.Contains(got, "Newer") {
		t.Errorf("changelog shows a release newer than the running one:\n%s", got)
	}

	out.Reset()
	printChangelog(&out, &release.Client{BaseURL: srv.URL}, "dev", 80)
	if got := ansi.Strip(out.String()); !strings.Contains(got, "Newer") {
		t.Errorf("dev build should print the newest releases:\n%s", got)
	}
}

func TestPrintChangelog_FailurePrintsLink(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"server error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"bad json":     func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("nope")) },
		"no releases":  func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("[]")) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			var out bytes.Buffer
			printChangelog(&out, &release.Client{BaseURL: srv.URL}, "1.2.0", 80)
			if !strings.Contains(out.String(), release.ReleasesURL) {
				t.Errorf("failure output lacks the releases link: %q", out.String())
			}
		})
	}
}

func TestExistingUser(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	cache := filepath.Join(dir, "cache")
	if existingUser(cfg, cache) {
		t.Fatal("fresh machine counted as an existing user")
	}
	os.MkdirAll(cache, 0o700)
	os.WriteFile(filepath.Join(cache, "cache.db"), nil, 0o600)
	if !existingUser(cfg, cache) {
		t.Error("an existing cache.db should mark an existing user")
	}
	os.Remove(filepath.Join(cache, "cache.db"))
	os.WriteFile(cfg, nil, 0o600)
	if !existingUser(cfg, cache) {
		t.Error("an existing config should mark an existing user")
	}
}

func TestPlanLaunch_UnwritableStateAnnouncesNothing(t *testing.T) {
	store := state.NewStore(filepath.Join("/dev/null", "pho", "state.json"))
	var warned bool
	a := planLaunch(store, "1.2.0", true, true, func(string, ...any) { warned = true })
	if a.Version != "" || !warned {
		t.Errorf("announcement %+v, warned %v; want none and a warning", a, warned)
	}
}

func TestUpdatesOptedOut(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "false": false, "1": true, "yes": true} {
		t.Setenv("PHO_NO_UPDATE_CHECK", v)
		if got := updatesOptedOut(); got != want {
			t.Errorf("PHO_NO_UPDATE_CHECK=%q: got %v, want %v", v, got, want)
		}
	}
}

func TestUpdateCheckEnabled(t *testing.T) {
	t.Setenv("PHO_NO_UPDATE_CHECK", "")
	if !updateCheckEnabled(true, "1.2.0", true) {
		t.Error("default release build in a terminal should check")
	}
	for name, got := range map[string]bool{
		"config off":     updateCheckEnabled(false, "1.2.0", true),
		"dev build":      updateCheckEnabled(true, "dev", true),
		"prerelease":     updateCheckEnabled(true, "1.3.0-rc.1", true),
		"not a terminal": updateCheckEnabled(true, "1.2.0", false),
	} {
		if got {
			t.Errorf("%s: check enabled, want off", name)
		}
	}
	t.Setenv("PHO_NO_UPDATE_CHECK", "1")
	if updateCheckEnabled(true, "1.2.0", true) {
		t.Error("PHO_NO_UPDATE_CHECK=1 should turn the check off")
	}
}
