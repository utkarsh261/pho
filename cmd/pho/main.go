/*
Copyright (C) 2026 Utkarsh Pandey

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/utkarsh261/pho/internal/application/dashboard"
	"github.com/utkarsh261/pho/internal/application/discovery"
	apppr "github.com/utkarsh261/pho/internal/application/pr"
	"github.com/utkarsh261/pho/internal/application/search"
	"github.com/utkarsh261/pho/internal/cache"
	"github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
	githubpkg "github.com/utkarsh261/pho/internal/github"
	"github.com/utkarsh261/pho/internal/github/auth"
	"github.com/utkarsh261/pho/internal/github/graphql"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
	"github.com/utkarsh261/pho/internal/release"
	"github.com/utkarsh261/pho/internal/state"
	"github.com/utkarsh261/pho/internal/ui/app"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

var version = "dev"

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
}

func clearCaches() error {
	cacheDir := xdgDir("XDG_CACHE_HOME", ".cache")
	sqliteDB := filepath.Join(cacheDir, "pho", "cache.db")
	if err := os.Remove(sqliteDB); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove sqlite cache %s: %w", sqliteDB, err)
	}

	discDir := filepath.Join(os.TempDir(), "pho-discovery")
	if err := os.RemoveAll(discDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove discovery cache %s: %w", discDir, err)
	}

	return nil
}

func xdgDir(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fallback
	}
	return filepath.Join(home, fallback)
}

type invocation struct {
	Version    bool
	Changelog  bool
	Debug      bool
	Reset      bool
	ConfigPath string
	RootDir    string
	Theme      string
	PRNumber   int
}

func registerFlags(fs *flag.FlagSet, inv *invocation, hidden map[string]bool) {
	fs.BoolVar(&inv.Version, "version", false, "print version and exit")
	fs.BoolVar(&inv.Changelog, "changelog", false, "print recent release notes and exit")
	fs.BoolVar(&inv.Debug, "debug", false, "enable debug logging (also set by PHO_DEBUG=1)")
	fs.BoolVar(&inv.Reset, "reset", false, "clear all caches (SQLite + discovery) and exit")
	fs.StringVar(&inv.RootDir, "root", ".", "root directory to scan for git repos")
	fs.StringVar(&inv.ConfigPath, "config", "", "path to config file (default: XDG config dir)")
	fs.StringVar(&inv.Theme, "theme", "", "colour theme, overriding the config file: "+theme.NameList())
	hidden["config"] = true
}

// newFlagSet builds a quiet FlagSet: flag's own error/usage output is
// suppressed so main is the single place that prints errors and usage.
func newFlagSet(name string, inv *invocation, hidden map[string]bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	registerFlags(fs, inv, hidden)
	return fs
}

// parseInvocation parses pho's CLI: optional global flags, an optional
// `pr <number>` subcommand, and flags again after the subcommand — both
// `pho -debug pr 12` and `pho pr 12 -debug` work.
func parseInvocation(args []string) (invocation, error) {
	var inv invocation
	hidden := map[string]bool{}

	fs := newFlagSet("pho", &inv, hidden)
	if err := fs.Parse(args); err != nil {
		return inv, err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return inv, nil
	}
	if rest[0] != "pr" {
		return inv, fmt.Errorf("unknown command %q (expected `pho pr <number>`)", rest[0])
	}
	rest = rest[1:]
	if len(rest) == 0 {
		return inv, fmt.Errorf("`pho pr` requires a pull request number, e.g. `pho pr 123`")
	}
	numberStr := strings.TrimPrefix(rest[0], "#")
	number, err := strconv.Atoi(numberStr)
	if err != nil || number <= 0 {
		return inv, fmt.Errorf("invalid pull request number %q", rest[0])
	}
	inv.PRNumber = number
	rest = rest[1:]

	// Flags after the subcommand are parsed into a scratch invocation and
	// merged only if the user actually set them, so defaults don't clobber
	// values parsed before the subcommand.
	var tail invocation
	tailFS := newFlagSet("pho pr", &tail, hidden)
	if err := tailFS.Parse(rest); err != nil {
		return inv, err
	}
	if extra := tailFS.Args(); len(extra) > 0 {
		return inv, fmt.Errorf("unexpected argument %q after `pho pr <number>`", extra[0])
	}
	tailFS.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "version":
			inv.Version = tail.Version
		case "changelog":
			inv.Changelog = tail.Changelog
		case "debug":
			inv.Debug = tail.Debug
		case "reset":
			inv.Reset = tail.Reset
		case "root":
			inv.RootDir = tail.RootDir
		case "config":
			inv.ConfigPath = tail.ConfigPath
		case "theme":
			inv.Theme = tail.Theme
		}
	})
	return inv, nil
}

// selectTheme picks the --theme flag over the configured theme. An unknown
// name returns the default theme with an error, so a typo never stops pho
// from starting.
func selectTheme(configured, flagValue string) (*theme.Theme, error) {
	name := configured
	if flagValue != "" {
		name = flagValue
	}
	if th, ok := theme.Get(name); ok {
		return th, nil
	}
	return theme.Default(), unknownTheme(name)
}

func unknownTheme(name string) error {
	return fmt.Errorf("unknown theme %q (available: %s)", name, theme.NameList())
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "pho — terminal UI for GitHub pull requests\n\nUsage:\n  pho [flags]\n  pho pr <number> [flags]\n      open a pull request by number, straight to its detail view\n\nFlags:\n")
	hidden := map[string]bool{}
	fs := newFlagSet("pho", &invocation{}, hidden)
	fs.VisitAll(func(f *flag.Flag) {
		if hidden[f.Name] {
			return
		}
		if f.DefValue == "" || f.DefValue == "false" || f.DefValue == "0" {
			fmt.Fprintf(w, "  -%s\n\t%s\n", f.Name, f.Usage)
		} else {
			fmt.Fprintf(w, "  -%s\n\t%s (default %s)\n", f.Name, f.Usage, f.DefValue)
		}
	})
}

func main() {
	inv, err := parseInvocation(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(os.Stderr)
			return
		}
		fmt.Fprintf(os.Stderr, "pho: %v\n\n", err)
		printUsage(os.Stderr)
		os.Exit(1)
	}

	var (
		showVersion = inv.Version
		debug       = inv.Debug
		reset       = inv.Reset
		configPath  = inv.ConfigPath
		rootDir     = inv.RootDir
	)

	if showVersion {
		fmt.Println("pho", version)
		return
	}
	if inv.Changelog {
		printChangelog(os.Stdout, &release.Client{}, version, stdoutWidth())
		return
	}

	if len(rootDir) >= 2 && rootDir[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			rootDir = filepath.Join(home, rootDir[2:])
		}
	}

	// matches log.IsDebug()
	if os.Getenv("PHO_DEBUG") == "1" {
		debug = true
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pho: failed to load config: %v\n  [config]\n", err)
		os.Exit(1)
	}
	// A --theme typo stops pho straight away; one in the config only gets
	// reported, once the UI is up.
	if inv.Theme != "" && !theme.Exists(inv.Theme) {
		fmt.Fprintf(os.Stderr, "pho: %v\n", unknownTheme(inv.Theme))
		os.Exit(2)
	}

	level := cfg.Logging.Level
	if debug {
		level = "debug"
	}
	logger := pholog.New(cfg.Logging.File, level)

	// existingUser must run before the cache below is created.
	existing := existingUser(cfg.Paths.ConfigFile, cfg.Cache.Dir)
	stateStore := state.NewStore(cfg.Paths.StateFile)

	if reset {
		if err := clearCaches(); err != nil {
			fmt.Fprintf(os.Stderr, "pho: failed to clear caches: %v\n", err)
			os.Exit(1)
		}
		logger.Info("caches cleared on startup")
	}

	authSvc := auth.NewAuthService()
	authDone := logger.Timer("auth resolution")
	profiles, err := authSvc.ResolveHosts(context.Background())
	authDone()
	if err != nil {
		logger.Error("auth failed", "err", err)
		fmt.Fprintf(os.Stderr, "pho: authentication error: %v\n  [auth]\nRun 'gh auth login' to authenticate.\n", err)
		os.Exit(1)
	}
	if len(profiles) == 0 {
		logger.Error("no authenticated hosts found")
		fmt.Fprintf(os.Stderr, "pho: no authenticated GitHub hosts found.\n  [auth]\nRun 'gh auth login' to authenticate.\n")
		os.Exit(1)
	}

	// Only a launch that opens on the dashboard can show the announcement.
	var (
		announce state.Announcement
		saved    state.State
	)
	if inv.PRNumber == 0 {
		announce, saved = planLaunch(stateStore, version, existing,
			cfg.Updates.WhatsNew && !updatesOptedOut(), logger.Warn)
	} else if saved, err = stateStore.Load(); err != nil {
		logger.Warn("could not read state", "path", stateStore.Path(), "err", err)
	}

	lipgloss.SetColorProfile(termenv.NewOutput(os.Stderr).Profile)
	model := buildModel(launch{
		cfg:        cfg,
		inv:        inv,
		configPath: configPath,
		rootDir:    rootDir,
		profiles:   profiles,
		logger:     logger,
		state:      stateStore,
		announce:   announce,
		saved:      saved,
		stdoutTTY:  term.IsTerminal(int(os.Stdout.Fd())),
	})

	defer logger.Timer("total session")()
	stopProfiling := startProfiling(logger.Warn)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	stopProfiling()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// launch is everything buildModel needs that main resolves first.
type launch struct {
	cfg        config.Config
	inv        invocation
	configPath string
	rootDir    string
	profiles   []githubpkg.GitHubHostProfile
	logger     *pholog.Logger
	state      *state.Store
	announce   state.Announcement
	saved      state.State
	stdoutTTY  bool
}

// buildModel wires caches, clients and services into the root model.
func buildModel(l launch) *app.Model {
	// Ensure cache directory exists
	if err := os.MkdirAll(l.cfg.Cache.Dir, 0o700); err != nil {
		l.logger.Warn("failed to create cache directory", "dir", l.cfg.Cache.Dir, "err", err)
	}

	cacheDone := l.logger.Timer("cache init")
	l1 := memory.NewJSONStore(l.cfg.Cache.MaxMemoryMB * 1024 * 1024)

	l2, err := sqlitecache.New(filepath.Join(l.cfg.Cache.Dir, "cache.db"), 1)
	var l2Store cache.Store
	var viewedHistoryStore domain.ViewedHistoryStore
	if err != nil {
		l.logger.Warn("sqlite cache unavailable, using memory-only cache", "err", err)
		l2Store = l1
	} else {
		l2Store = l2
		viewedHistoryStore = l2
	}

	coordinator := cache.NewCoordinator(l1, l2Store, l.logger)

	ghClient := graphql.NewClient(l.profiles, &http.Client{Timeout: 30 * time.Second}, l.logger)
	cacheDone()

	wireDone := l.logger.Timer("service wiring")
	discoverySvc := discovery.New(discovery.Config{
		Pin:     l.cfg.Repos.Pin,
		Exclude: l.cfg.Repos.Exclude,
	})
	discoverySvc.Log = l.logger
	dashboardSvc := dashboard.NewService(coordinator, ghClient)
	dashboardSvc.Log = l.logger
	searchSvc := search.New()
	searchSvc.Log = l.logger

	// REST client for raw diff fetching (one per primary host).
	restClient := rest.NewClient(l.profiles[0].RESTURL, l.profiles[0].Token, l.logger)
	restClient.HTTPClient = &http.Client{Timeout: 30 * time.Second}

	// Per-host REST clients so mutations route to the right GitHub host
	// (e.g. a GHES PR's update-branch lands on the GHES instance, not github.com).
	restByHost := make(map[string]*rest.Client, len(l.profiles))
	for _, p := range l.profiles {
		c := rest.NewClient(p.RESTURL, p.Token, l.logger)
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
		restByHost[p.Host] = c
	}

	// PR detail service: loads PR metadata (GraphQL) and diffs (REST).
	prSvc := apppr.NewService(coordinator, ghClient, restClient, restByHost)
	prSvc.Log = l.logger

	deps := app.Dependencies{
		Viewer:          ghClient,
		Discovery:       discoverySvc,
		Dashboard:       dashboardSvc,
		Search:          searchSvc,
		PR:              prSvc,
		ViewedHistory:   viewedHistoryStore,
		Root:            l.rootDir,
		Host:            l.profiles[0].Host,
		MaxJumpPRs:      l.cfg.Palette.MaxPRs,
		MaxDashboardPRs: l.cfg.Dashboard.MaxPRs,
		NoStackGrouping: !l.cfg.Dashboard.GroupStacks,
		DiffLimits: prdetail.DiffLimits{
			CollapseLines:     l.cfg.Diff.CollapseLines,
			CollapseLineWidth: l.cfg.Diff.CollapseLineWidth,
			MaxLines:          l.cfg.Diff.MaxLines,
			MaxLineWidth:      l.cfg.Diff.MaxLineWidth,
			RowBudget:         l.cfg.Diff.RowBudget,
		},
		InitialPRNumber:    l.inv.PRNumber,
		Logger:             l.logger,
		TerminalBackground: l.cfg.UI.Background == "terminal",
		SaveTheme: func(name string) error {
			return config.SaveTheme(l.configPath, name)
		},
		ConfiguredTheme: l.cfg.UI.Theme,
		Version:         version,
		AnnounceVersion: l.announce.Version,
		WhatsNewSince:   l.announce.Since,
		FetchReleases:   (&release.Client{}).List,
		State:           l.state,
		CheckForUpdates: updateCheckEnabled(l.cfg.Updates.Check, version, l.stdoutTTY),
		LatestKnown:     l.saved.LatestKnown,
		LastUpdateCheck: l.saved.LastUpdateCheck,
		FetchLatest:     (&release.Client{}).Latest,
		RunUpdate:       brewUpdater(),
	}
	model := app.NewModel(deps)

	th, err := selectTheme(l.cfg.UI.Theme, l.inv.Theme)
	model.SetTheme(th)
	if err != nil {
		l.logger.Warn("unknown theme in config, using the default", "err", err)
		model.ReportUnknownTheme(l.cfg.UI.Theme)
	}
	if bg := l.cfg.UI.Background; bg != "theme" && bg != "terminal" {
		l.logger.Warn("unknown [ui] background, painting the theme's", "background", bg)
		model.ReportConfigProblem(fmt.Errorf("unknown [ui] background %q, painting the theme's", bg))
	}
	wireDone()
	return model
}
