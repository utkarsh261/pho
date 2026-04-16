package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	"github.com/utkarsh261/pho/internal/github/auth"
	"github.com/utkarsh261/pho/internal/github/graphql"
	"github.com/utkarsh261/pho/internal/github/rest"
	gitlog "github.com/utkarsh261/pho/internal/log"
	"github.com/utkarsh261/pho/internal/ui/app"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

var version = "dev"

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

func main() {
	var (
		showVersion bool
		debug       bool
		reset       bool
		configPath  string
		rootDir     string
	)

	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&debug, "debug", false, "enable debug logging (also set by PHO_DEBUG=1)")
	flag.BoolVar(&reset, "reset", false, "clear all caches (SQLite + discovery) and exit")
	flag.StringVar(&configPath, "config", "", "path to config file (default: XDG config dir)")
	flag.StringVar(&rootDir, "root", ".", "root directory to scan for git repos")
	flag.Parse()

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
	logger := gitlog.New(cfg.Logging.File, level)

	if reset {
		if err := clearCaches(); err != nil {
			fmt.Fprintf(os.Stderr, "pho: failed to clear caches: %v\n", err)
			os.Exit(1)
		}
		logger.Info("caches cleared on startup")
	}

	authSvc := auth.NewAuthService()
	profiles, err := authSvc.ResolveHosts(context.Background())
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

	l1 := memory.NewJSONStore(cfg.Cache.MaxMemoryMB * 1024 * 1024)

	l2, err := sqlitecache.New(filepath.Join(cfg.Cache.Dir, "cache.db"), 1)
	var l2Store cache.Store
	if err != nil {
		logger.Warn("sqlite cache unavailable, using memory-only cache", "err", err)
		l2Store = l1
	} else {
		l2Store = l2
	}

	coordinator := cache.NewCoordinator(l1, l2Store, logger)

	ghClient := graphql.NewClient(profiles, &http.Client{Timeout: 30 * time.Second}, logger)

	discoverySvc := discovery.New(discovery.Config{
		Pin:     cfg.Repos.Pin,
		Exclude: cfg.Repos.Exclude,
	})
	dashboardSvc := dashboard.NewService(coordinator, ghClient)
	searchSvc := search.New()

	// REST client for raw diff fetching (one per primary host).
	restClient := &rest.Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		BaseURL:    profiles[0].RESTURL,
		Token:      profiles[0].Token,
	}

	// PR detail service: loads PR metadata (GraphQL) and diffs (REST).
	prSvc := apppr.NewService(coordinator, ghClient, restClient)
	prSvc.Host = profiles[0].Host
	prSvc.Log = logger

	deps := app.Dependencies{
		Viewer:    ghClient,
		Discovery: discoverySvc,
		Dashboard: dashboardSvc,
		Search:    searchSvc,
		PR:        prSvc,
		Root:      rootDir,
		Host:      profiles[0].Host,
		Logger:    logger,
	}
	model := app.NewModel(deps)

	lipgloss.SetColorProfile(termenv.NewOutput(os.Stderr).Profile)
	th := theme.Default()
	model.SetTheme(th)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
