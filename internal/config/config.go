// Package config handles loading and resolving configuration for git-term.
// It reads a TOML file (defaulting to the XDG config path) and applies
// hardcoded defaults for any fields that are zero-valued or absent.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// duration is a time.Duration that can be decoded from a TOML string like "5m".
type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("config: invalid duration %q: %w", string(text), err)
	}
	d.Duration = v
	return nil
}

// rawConfig mirrors Config but uses the custom duration type for TTL fields
// so that BurntSushi/toml can decode string values like "5m".
type rawConfig struct {
	Discovery struct {
		MaxRepos    int      `toml:"max_repos"`
		ExcludeDirs []string `toml:"exclude_dirs"`
	} `toml:"discovery"`
	Repos struct {
		Pin     []string `toml:"pin"`
		Exclude []string `toml:"exclude"`
	} `toml:"repos"`
	Dashboard struct {
		DefaultTab  string `toml:"default_tab"`
		RecentHours int    `toml:"recent_hours"`
	} `toml:"dashboard"`
	Cache struct {
		Dir            string   `toml:"dir"`
		MaxMemoryMB    int      `toml:"max_memory_mb"`
		DashboardTTL   duration `toml:"dashboard_ttl"`
		DiscoveryTTL   duration `toml:"discovery_ttl"`
		SearchIndexTTL duration `toml:"search_index_ttl"`
	} `toml:"cache"`
	Logging struct {
		File  string `toml:"file"`
		Level string `toml:"level"`
	} `toml:"logging"`
}

// Config is the fully-resolved application configuration.
type Config struct {
	Discovery struct {
		MaxRepos    int
		ExcludeDirs []string
	}
	Repos struct {
		Pin     []string
		Exclude []string
	}
	Dashboard struct {
		DefaultTab  string
		RecentHours int
	}
	Cache struct {
		Dir            string
		MaxMemoryMB    int
		DashboardTTL   time.Duration
		DiscoveryTTL   time.Duration
		SearchIndexTTL time.Duration
	}
	Logging struct {
		File  string
		Level string
	}
}

// Paths holds fully-resolved filesystem paths used by the application.
type Paths struct {
	ConfigFile string
	CacheDir   string
	LogFile    string
}

func xdgDir(envVar, homeRelDefault string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return homeRelDefault
	}
	return filepath.Join(home, homeRelDefault)
}

// expandTilde replaces a leading "~" with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// ResolvePaths derives the three canonical filesystem paths for the given
// (possibly empty) config file path.
func ResolvePaths(configPath string) Paths {
	if configPath == "" {
		cfgDir := xdgDir("XDG_CONFIG_HOME", ".config")
		configPath = filepath.Join(cfgDir, "git-term", "config.toml")
	}
	cacheDir := filepath.Join(xdgDir("XDG_CACHE_HOME", ".cache"), "git-term")
	logFile := filepath.Join(xdgDir("XDG_STATE_HOME", ".local/state"), "git-term", "debug.log")
	return Paths{
		ConfigFile: configPath,
		CacheDir:   cacheDir,
		LogFile:    logFile,
	}
}

// defaults returns a Config pre-populated with all hardcoded default values.
func defaults() Config {
	var cfg Config
	cfg.Discovery.MaxRepos = 50
	cfg.Discovery.ExcludeDirs = []string{}
	cfg.Dashboard.DefaultTab = "my_prs"
	cfg.Dashboard.RecentHours = 24
	cfg.Cache.MaxMemoryMB = 16
	cfg.Cache.DashboardTTL = 2 * time.Minute
	cfg.Cache.DiscoveryTTL = 1 * time.Hour
	cfg.Cache.SearchIndexTTL = 2 * time.Minute
	cfg.Logging.Level = "info"
	return cfg
}

// applyDefaults fills zero-value fields in cfg with values from def.
func applyDefaults(cfg *Config, def Config) {
	if cfg.Discovery.MaxRepos == 0 {
		cfg.Discovery.MaxRepos = def.Discovery.MaxRepos
	}
	if cfg.Discovery.ExcludeDirs == nil {
		cfg.Discovery.ExcludeDirs = def.Discovery.ExcludeDirs
	}
	if cfg.Dashboard.DefaultTab == "" {
		cfg.Dashboard.DefaultTab = def.Dashboard.DefaultTab
	}
	if cfg.Dashboard.RecentHours == 0 {
		cfg.Dashboard.RecentHours = def.Dashboard.RecentHours
	}
	if cfg.Cache.MaxMemoryMB == 0 {
		cfg.Cache.MaxMemoryMB = def.Cache.MaxMemoryMB
	}
	if cfg.Cache.DashboardTTL == 0 {
		cfg.Cache.DashboardTTL = def.Cache.DashboardTTL
	}
	if cfg.Cache.DiscoveryTTL == 0 {
		cfg.Cache.DiscoveryTTL = def.Cache.DiscoveryTTL
	}
	if cfg.Cache.SearchIndexTTL == 0 {
		cfg.Cache.SearchIndexTTL = def.Cache.SearchIndexTTL
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = def.Logging.Level
	}
}

// Load reads configuration from path (or the XDG default when path is empty),
// applies hardcoded defaults for missing fields, and resolves XDG-derived
// paths for Cache.Dir and Logging.File when not set in the file.
//
// If the file does not exist, Load returns the defaults and a nil error.
// If the file exists but cannot be parsed, Load returns the defaults and a
// non-nil wrapped error.
func Load(path string) (Config, error) {
	paths := ResolvePaths(path)
	resolved := paths.ConfigFile

	def := defaults()

	data, err := os.ReadFile(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No config file — apply XDG-derived paths and return defaults.
			def.Cache.Dir = paths.CacheDir
			def.Logging.File = paths.LogFile
			return def, nil
		}
		// Unexpected read error — treat like a parse error.
		def.Cache.Dir = paths.CacheDir
		def.Logging.File = paths.LogFile
		return def, fmt.Errorf("config: read %s: %w", resolved, err)
	}

	var raw rawConfig
	if _, err := toml.Decode(string(data), &raw); err != nil {
		def.Cache.Dir = paths.CacheDir
		def.Logging.File = paths.LogFile
		return def, fmt.Errorf("config: parse %s: %w", resolved, err)
	}

	// Map raw → Config.
	var cfg Config
	cfg.Discovery.MaxRepos = raw.Discovery.MaxRepos
	cfg.Discovery.ExcludeDirs = raw.Discovery.ExcludeDirs
	cfg.Repos.Pin = raw.Repos.Pin
	cfg.Repos.Exclude = raw.Repos.Exclude
	cfg.Dashboard.DefaultTab = raw.Dashboard.DefaultTab
	cfg.Dashboard.RecentHours = raw.Dashboard.RecentHours
	cfg.Cache.Dir = raw.Cache.Dir
	cfg.Cache.MaxMemoryMB = raw.Cache.MaxMemoryMB
	cfg.Cache.DashboardTTL = raw.Cache.DashboardTTL.Duration
	cfg.Cache.DiscoveryTTL = raw.Cache.DiscoveryTTL.Duration
	cfg.Cache.SearchIndexTTL = raw.Cache.SearchIndexTTL.Duration
	cfg.Logging.File = raw.Logging.File
	cfg.Logging.Level = raw.Logging.Level

	// Fill zero-value fields with defaults.
	applyDefaults(&cfg, def)

	// Derive XDG paths when not set in the file.
	if cfg.Cache.Dir == "" {
		cfg.Cache.Dir = paths.CacheDir
	}
	if cfg.Logging.File == "" {
		cfg.Logging.File = paths.LogFile
	}

	// Expand ~ in path fields.
	cfg.Cache.Dir = expandTilde(cfg.Cache.Dir)
	cfg.Logging.File = expandTilde(cfg.Logging.File)

	return cfg, nil
}
