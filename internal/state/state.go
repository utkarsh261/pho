// Package state keeps the small amount of state pho remembers between runs
// that isn't configuration: which version the user last saw, and what the
// update check last found. It lives in $XDG_STATE_HOME/pho/state.json.
//
// Nothing here is essential. A missing, unreadable or corrupt file reads as
// empty state, and callers log failed writes and carry on.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/utkarsh261/pho/internal/fsutil"
	"github.com/utkarsh261/pho/internal/release"
)

// AnnounceLaunches is how many launches of a new version announce it in the
// status bar before pho stops mentioning it.
const AnnounceLaunches = 3

// State is everything state.json holds.
type State struct {
	// LastSeenVersion is the newest stable version the user has seen the
	// notes for, or been told about long enough. Only stable versions are
	// ever stored.
	LastSeenVersion string `json:"last_seen_version,omitempty"`
	// AnnouncedVersion and AnnouncedLaunches count the launches that
	// announced AnnouncedVersion.
	AnnouncedVersion  string `json:"announced_version,omitempty"`
	AnnouncedLaunches int    `json:"announced_launches,omitempty"`
	// LastUpdateCheck and LatestKnown record the last update check.
	LastUpdateCheck time.Time `json:"last_update_check,omitzero"`
	LatestKnown     string    `json:"latest_known,omitempty"`
}

// Store reads and writes state.json. Its methods are safe for concurrent
// use, so background commands can update different fields without losing
// each other's writes.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore returns a store for the file at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the file the store reads and writes.
func (s *Store) Path() string {
	return s.path
}

// Load reads the state. On any failure it returns empty state along with
// the error, which callers only log.
func (s *Store) Load() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Exists reports whether the state file is there, readable or not.
func (s *Store) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Update reads the state, applies fn and writes the result back, all under
// one lock. fn runs even when the file can't be read, against empty state.
func (s *Store) Update(fn func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _ := s.load()
	fn(&st)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if err := fsutil.WriteAtomic(s.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return nil
}

func (s *Store) load() (State, error) {
	var st State
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("state: %w", err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("state: parse %s: %w", s.path, err)
	}
	// Only stable versions make a baseline; anything else would stop a
	// later release from comparing as newer.
	if !release.IsRelease(st.LastSeenVersion) {
		st.LastSeenVersion = ""
	}
	if !release.IsRelease(st.LatestKnown) {
		st.LatestKnown = ""
	}
	return st, nil
}

// Announcement says whether this launch should announce an upgrade, and
// which version the user upgraded from ("" when unknown).
type Announcement struct {
	Version string // the version to announce; "" for none
	Since   string
}

// PlanLaunch decides whether this launch of version announces an upgrade,
// and records the decision in s. existingUser tells a first run apart from
// an upgrade from a version that didn't keep state; enabled is false when
// the user turned "What's new" off. Dev and prerelease builds never change
// anything.
func (s *State) PlanLaunch(version string, existingUser, enabled bool) Announcement {
	if !release.IsRelease(version) {
		return Announcement{}
	}
	version = release.Normalize(version)
	if s.LastSeenVersion == "" && !existingUser {
		s.LastSeenVersion = version
		return Announcement{}
	}
	if s.LastSeenVersion != "" && release.Compare(version, s.LastSeenVersion) <= 0 {
		return Announcement{}
	}
	if !enabled {
		s.LastSeenVersion = version
		return Announcement{}
	}
	if s.AnnouncedVersion != version {
		s.AnnouncedVersion = version
		s.AnnouncedLaunches = 0
	}
	if s.AnnouncedLaunches >= AnnounceLaunches {
		s.LastSeenVersion = version
		return Announcement{}
	}
	s.AnnouncedLaunches++
	return Announcement{Version: version, Since: s.LastSeenVersion}
}

// MarkSeen records that the user has seen version's notes. Only stable
// versions are recorded.
func (s *State) MarkSeen(version string) {
	if !release.IsRelease(version) {
		return
	}
	version = release.Normalize(version)
	if s.LastSeenVersion == "" || release.Compare(version, s.LastSeenVersion) > 0 {
		s.LastSeenVersion = version
	}
}
