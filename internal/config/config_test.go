package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/config"
)

// unsetEnv clears an environment variable and restores the original value
// when the test finishes.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// setEnv sets an environment variable and restores the original value when the
// test finishes.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.toml")

	cfg, err := config.Load(path)

	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg.Discovery.MaxRepos != 50 {
		t.Errorf("Discovery.MaxRepos: got %d, want 50", cfg.Discovery.MaxRepos)
	}
	if cfg.Dashboard.DefaultTab != "my_prs" {
		t.Errorf("Dashboard.DefaultTab: got %q, want %q", cfg.Dashboard.DefaultTab, "my_prs")
	}
	if cfg.Dashboard.RecentHours != 24 {
		t.Errorf("Dashboard.RecentHours: got %d, want 24", cfg.Dashboard.RecentHours)
	}
	if cfg.Cache.MaxMemoryMB != 16 {
		t.Errorf("Cache.MaxMemoryMB: got %d, want 16", cfg.Cache.MaxMemoryMB)
	}
	if cfg.Cache.DashboardTTL != 2*time.Minute {
		t.Errorf("Cache.DashboardTTL: got %v, want 2m", cfg.Cache.DashboardTTL)
	}
	if cfg.Cache.DiscoveryTTL != time.Hour {
		t.Errorf("Cache.DiscoveryTTL: got %v, want 1h", cfg.Cache.DiscoveryTTL)
	}
	if cfg.Cache.SearchIndexTTL != 2*time.Minute {
		t.Errorf("Cache.SearchIndexTTL: got %v, want 2m", cfg.Cache.SearchIndexTTL)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level: got %q, want %q", cfg.Logging.Level, "info")
	}
}

func TestLoad_ValidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	toml := `
[discovery]
max_repos = 100
exclude_dirs = ["/tmp", "/var"]

[repos]
pin = ["owner/repo1"]
exclude = ["owner/repo2"]

[dashboard]
default_tab = "needs_review"
recent_hours = 48

[cache]
max_memory_mb = 32
dashboard_ttl = "5m"
discovery_ttl = "2h"
search_index_ttl = "10m"

[logging]
level = "debug"
`
	if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Discovery.MaxRepos != 100 {
		t.Errorf("Discovery.MaxRepos: got %d, want 100", cfg.Discovery.MaxRepos)
	}
	if len(cfg.Discovery.ExcludeDirs) != 2 || cfg.Discovery.ExcludeDirs[0] != "/tmp" {
		t.Errorf("Discovery.ExcludeDirs: got %v", cfg.Discovery.ExcludeDirs)
	}
	if len(cfg.Repos.Pin) != 1 || cfg.Repos.Pin[0] != "owner/repo1" {
		t.Errorf("Repos.Pin: got %v", cfg.Repos.Pin)
	}
	if len(cfg.Repos.Exclude) != 1 || cfg.Repos.Exclude[0] != "owner/repo2" {
		t.Errorf("Repos.Exclude: got %v", cfg.Repos.Exclude)
	}
	if cfg.Dashboard.DefaultTab != "needs_review" {
		t.Errorf("Dashboard.DefaultTab: got %q, want %q", cfg.Dashboard.DefaultTab, "needs_review")
	}
	if cfg.Dashboard.RecentHours != 48 {
		t.Errorf("Dashboard.RecentHours: got %d, want 48", cfg.Dashboard.RecentHours)
	}
	if cfg.Cache.MaxMemoryMB != 32 {
		t.Errorf("Cache.MaxMemoryMB: got %d, want 32", cfg.Cache.MaxMemoryMB)
	}
	if cfg.Cache.DashboardTTL != 5*time.Minute {
		t.Errorf("Cache.DashboardTTL: got %v, want 5m", cfg.Cache.DashboardTTL)
	}
	if cfg.Cache.DiscoveryTTL != 2*time.Hour {
		t.Errorf("Cache.DiscoveryTTL: got %v, want 2h", cfg.Cache.DiscoveryTTL)
	}
	if cfg.Cache.SearchIndexTTL != 10*time.Minute {
		t.Errorf("Cache.SearchIndexTTL: got %v, want 10m", cfg.Cache.SearchIndexTTL)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level: got %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestLoad_CorruptTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(path, []byte("[[[[not valid toml"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err == nil {
		t.Fatal("expected non-nil error for corrupt TOML")
	}
	// Should still return defaults.
	if cfg.Discovery.MaxRepos != 50 {
		t.Errorf("Discovery.MaxRepos: got %d, want 50 (default)", cfg.Discovery.MaxRepos)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level: got %q, want %q (default)", cfg.Logging.Level, "info")
	}
}

func TestLoad_XDGConfigHome(t *testing.T) {
	cfgDir := t.TempDir()
	setEnv(t, "XDG_CONFIG_HOME", cfgDir)

	// Create config at the expected XDG path.
	gitTermDir := filepath.Join(cfgDir, "pho")
	if err := os.MkdirAll(gitTermDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(gitTermDir, "config.toml")
	toml := `
[dashboard]
default_tab = "involving"
`
	if err := os.WriteFile(cfgFile, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Dashboard.DefaultTab != "involving" {
		t.Errorf("Dashboard.DefaultTab: got %q, want %q", cfg.Dashboard.DefaultTab, "involving")
	}
}

func TestLoad_XDGCacheHome(t *testing.T) {
	cacheDir := t.TempDir()
	setEnv(t, "XDG_CACHE_HOME", cacheDir)

	// No config file — just verify Cache.Dir derives from XDG_CACHE_HOME.
	nonexistent := filepath.Join(t.TempDir(), "config.toml")

	cfg, err := config.Load(nonexistent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(cacheDir, "pho")
	if cfg.Cache.Dir != want {
		t.Errorf("Cache.Dir: got %q, want %q", cfg.Cache.Dir, want)
	}
}

func TestLoad_PartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Only set one field; everything else should fall back to defaults.
	toml := `
[discovery]
max_repos = 200
`
	if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Discovery.MaxRepos != 200 {
		t.Errorf("Discovery.MaxRepos: got %d, want 200", cfg.Discovery.MaxRepos)
	}
	// Everything else should be default.
	if cfg.Dashboard.DefaultTab != "my_prs" {
		t.Errorf("Dashboard.DefaultTab: got %q, want %q (default)", cfg.Dashboard.DefaultTab, "my_prs")
	}
	if cfg.Cache.DashboardTTL != 2*time.Minute {
		t.Errorf("Cache.DashboardTTL: got %v, want 2m (default)", cfg.Cache.DashboardTTL)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level: got %q, want %q (default)", cfg.Logging.Level, "info")
	}
}

func TestLoad_DurationParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	toml := `
[cache]
dashboard_ttl = "5m"
discovery_ttl = "30m"
search_index_ttl = "15s"
`
	if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Cache.DashboardTTL != 5*time.Minute {
		t.Errorf("Cache.DashboardTTL: got %v, want 5m", cfg.Cache.DashboardTTL)
	}
	if cfg.Cache.DiscoveryTTL != 30*time.Minute {
		t.Errorf("Cache.DiscoveryTTL: got %v, want 30m", cfg.Cache.DiscoveryTTL)
	}
	if cfg.Cache.SearchIndexTTL != 15*time.Second {
		t.Errorf("Cache.SearchIndexTTL: got %v, want 15s", cfg.Cache.SearchIndexTTL)
	}
}

func TestResolvePaths_Empty(t *testing.T) {
	// Clear XDG vars so we get predictable home-relative defaults.
	unsetEnv(t, "XDG_CONFIG_HOME")
	unsetEnv(t, "XDG_CACHE_HOME")
	unsetEnv(t, "XDG_STATE_HOME")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	paths := config.ResolvePaths("")

	wantConfig := filepath.Join(home, ".config", "pho", "config.toml")
	wantCache := filepath.Join(home, ".cache", "pho")
	wantLog := filepath.Join(home, ".local", "state", "pho", "debug.log")

	if paths.ConfigFile != wantConfig {
		t.Errorf("ConfigFile: got %q, want %q", paths.ConfigFile, wantConfig)
	}
	if paths.CacheDir != wantCache {
		t.Errorf("CacheDir: got %q, want %q", paths.CacheDir, wantCache)
	}
	if paths.LogFile != wantLog {
		t.Errorf("LogFile: got %q, want %q", paths.LogFile, wantLog)
	}
}

func TestResolvePaths_XDGOverrides(t *testing.T) {
	cfgDir := t.TempDir()
	cacheDir := t.TempDir()
	stateDir := t.TempDir()

	setEnv(t, "XDG_CONFIG_HOME", cfgDir)
	setEnv(t, "XDG_CACHE_HOME", cacheDir)
	setEnv(t, "XDG_STATE_HOME", stateDir)

	paths := config.ResolvePaths("")

	if paths.ConfigFile != filepath.Join(cfgDir, "pho", "config.toml") {
		t.Errorf("ConfigFile: got %q", paths.ConfigFile)
	}
	if paths.CacheDir != filepath.Join(cacheDir, "pho") {
		t.Errorf("CacheDir: got %q", paths.CacheDir)
	}
	if paths.LogFile != filepath.Join(stateDir, "pho", "debug.log") {
		t.Errorf("LogFile: got %q", paths.LogFile)
	}
}
