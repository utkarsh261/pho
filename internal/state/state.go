// Package state is pho's state.json. Nothing in it is essential: a bad file
// reads as empty state.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/utkarsh261/pho/internal/fsutil"
	"github.com/utkarsh261/pho/internal/release"
)

// AnnounceLaunches is how many launches announce a new version.
const AnnounceLaunches = 3

type State struct {
	LastSeenVersion   string `json:"last_seen_version,omitempty"`
	AnnouncedVersion  string `json:"announced_version,omitempty"`
	AnnouncedLaunches int    `json:"announced_launches,omitempty"`
}

// Store is safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

// Load returns empty state on any error.
func (s *Store) Load() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Update applies fn under one lock. An unreadable file counts as empty.
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
	// A non-stable baseline would never compare older than a release.
	if !release.IsRelease(st.LastSeenVersion) {
		st.LastSeenVersion = ""
	}
	return st, nil
}

type Announcement struct {
	Version string // the version to announce; "" for none
	Since   string
}

// PlanLaunch decides whether this launch announces an upgrade, recording it
// in s. Dev and prerelease builds change nothing.
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

// MarkSeen only records stable versions, and only forward.
func (s *State) MarkSeen(version string) {
	if !release.IsRelease(version) {
		return
	}
	version = release.Normalize(version)
	if s.LastSeenVersion == "" || release.Compare(version, s.LastSeenVersion) > 0 {
		s.LastSeenVersion = version
	}
}
