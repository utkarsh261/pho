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

	"github.com/utkarsh261/pho/internal/application/dashboard"
	"github.com/utkarsh261/pho/internal/application/discovery"
	apppr "github.com/utkarsh261/pho/internal/application/pr"
	"github.com/utkarsh261/pho/internal/application/search"
	"github.com/utkarsh261/pho/internal/cache"
	"github.com/utkarsh261/pho/internal/cache/memory"
	sqlitecache "github.com/utkarsh261/pho/internal/cache/sqlite"
	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/auth"
	"github.com/utkarsh261/pho/internal/github/graphql"
	"github.com/utkarsh261/pho/internal/github/rest"
	pholog "github.com/utkarsh261/pho/internal/log"
	"github.com/utkarsh261/pho/internal/ui/app"
	"github.com/utkarsh261/pho/internal/ui/theme"
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

// invocation holds everything parseInvocation can extract from the command line.
type invocation struct {
	Version    bool
	Debug      bool
	Reset      bool
	ConfigPath string
	RootDir    string
	PRNumber   int
}

func registerFlags(fs *flag.FlagSet, inv *invocation, hidden map[string]bool) {
	fs.BoolVar(&inv.Version, "version", false, "print version and exit")
	fs.BoolVar(&inv.Debug, "debug", false, "enable debug logging (also set by PHO_DEBUG=1)")
	fs.BoolVar(&inv.Reset, "reset", false, "clear all caches (SQLite + discovery) and exit")
	fs.StringVar(&inv.RootDir, "root", ".", "root directory to scan for git repos")
	fs.StringVar(&inv.ConfigPath, "config", "", "path to config file (default: XDG config dir)")
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
		case "debug":
			inv.Debug = tail.Debug
		case "reset":
			inv.Reset = tail.Reset
		case "root":
			inv.RootDir = tail.RootDir
		case "config":
			inv.ConfigPath = tail.ConfigPath
		}
	})
	return inv, nil
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

	level := cfg.Logging.Level
	if debug {
		level = "debug"
	}
	logger := pholog.New(cfg.Logging.File, level)

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

	// Ensure cache directory exists
	if err := os.MkdirAll(cfg.Cache.Dir, 0o700); err != nil {
		logger.Warn("failed to create cache directory", "dir", cfg.Cache.Dir, "err", err)
	}

	cacheDone := logger.Timer("cache init")
	l1 := memory.NewJSONStore(cfg.Cache.MaxMemoryMB * 1024 * 1024)

	l2, err := sqlitecache.New(filepath.Join(cfg.Cache.Dir, "cache.db"), 1)
	var l2Store cache.Store
	var viewedHistoryStore domain.ViewedHistoryStore
	if err != nil {
		logger.Warn("sqlite cache unavailable, using memory-only cache", "err", err)
		l2Store = l1
	} else {
		l2Store = l2
		viewedHistoryStore = l2
	}

	coordinator := cache.NewCoordinator(l1, l2Store, logger)

	ghClient := graphql.NewClient(profiles, &http.Client{Timeout: 30 * time.Second}, logger)
	cacheDone()

	wireDone := logger.Timer("service wiring")
	discoverySvc := discovery.New(discovery.Config{
		Pin:     cfg.Repos.Pin,
		Exclude: cfg.Repos.Exclude,
	})
	discoverySvc.Log = logger
	dashboardSvc := dashboard.NewService(coordinator, ghClient)
	dashboardSvc.Log = logger
	searchSvc := search.New()
	searchSvc.Log = logger

	// REST client for raw diff fetching (one per primary host).
	restClient := rest.NewClient(profiles[0].RESTURL, profiles[0].Token, logger)
	restClient.HTTPClient = &http.Client{Timeout: 30 * time.Second}

	// Per-host REST clients so mutations route to the right GitHub host
	// (e.g. a GHES PR's update-branch lands on the GHES instance, not github.com).
	restByHost := make(map[string]*rest.Client, len(profiles))
	for _, p := range profiles {
		c := rest.NewClient(p.RESTURL, p.Token, logger)
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
		restByHost[p.Host] = c
	}

	// PR detail service: loads PR metadata (GraphQL) and diffs (REST).
	prSvc := apppr.NewService(coordinator, ghClient, restClient, restByHost)
	prSvc.Log = logger

	deps := app.Dependencies{
		Viewer:          ghClient,
		Discovery:       discoverySvc,
		Dashboard:       dashboardSvc,
		Search:          searchSvc,
		PR:              prSvc,
		ViewedHistory:   viewedHistoryStore,
		Root:            rootDir,
		Host:            profiles[0].Host,
		MaxJumpPRs:      cfg.Palette.MaxPRs,
		InitialPRNumber: inv.PRNumber,
		Logger:          logger,
	}
	model := app.NewModel(deps)

	lipgloss.SetColorProfile(termenv.NewOutput(os.Stderr).Profile)
	th := theme.Default()
	model.SetTheme(th)
	wireDone()

	defer logger.Timer("total session")()
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
