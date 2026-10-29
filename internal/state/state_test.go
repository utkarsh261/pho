package state

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestStore_MissingFile(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "pho", "state.json"))
	st, err := s.Load()
	if err != nil || st != (State{}) {
		t.Fatalf("Load = %+v, %v; want empty state, nil", st, err)
	}
}

func TestStore_RoundTrip(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "pho", "state.json"))
	if err := s.Update(func(st *State) {
		st.LastSeenVersion = "1.2.0"
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSeenVersion != "1.2.0" {
		t.Errorf("Load = %+v", st)
	}
}

func TestStore_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	s := NewStore(path)
	st, err := s.Load()
	if err == nil || st != (State{}) {
		t.Fatalf("Load = %+v, %v; want empty state and an error", st, err)
	}
	// Update starts over from empty state and repairs the file.
	if err := s.Update(func(st *State) { st.LastSeenVersion = "1.0.0" }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if st, err := s.Load(); err != nil || st.LastSeenVersion != "1.0.0" {
		t.Errorf("after repair Load = %+v, %v", st, err)
	}
}

func TestStore_NonReleaseVersionsDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{"last_seen_version":"dev"}`), 0o600)
	st, err := NewStore(path).Load()
	if err != nil || st.LastSeenVersion != "" {
		t.Errorf("Load = %+v, %v; want the version dropped", st, err)
	}
}

func TestStore_Unwritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permissions not enforced")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o500)
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	s := NewStore(filepath.Join(dir, "pho", "state.json"))
	if err := s.Update(func(st *State) { st.LastSeenVersion = "1.0.0" }); err == nil {
		t.Error("Update into a read-only directory succeeded, want error")
	}
	if _, err := s.Load(); err != nil {
		t.Errorf("Load after failed write: %v", err)
	}
}

func TestStore_Unreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permissions not enforced")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{"last_seen_version":"1.0.0"}`), 0o000)
	st, err := NewStore(path).Load()
	if err == nil || st != (State{}) {
		t.Errorf("Load = %+v, %v; want empty state and an error", st, err)
	}
}

func TestStore_ConcurrentUpdates(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "state.json"))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Update(func(st *State) { st.AnnouncedLaunches++ })
		}()
	}
	wg.Wait()
	st, _ := s.Load()
	if st.AnnouncedLaunches != 20 {
		t.Errorf("AnnouncedLaunches = %d, want 20 (lost updates)", st.AnnouncedLaunches)
	}
}

func TestPlanLaunch(t *testing.T) {
	t.Run("fresh install records silently", func(t *testing.T) {
		var st State
		if a := st.PlanLaunch("1.2.0", false, true); a.Version != "" {
			t.Errorf("announced %+v", a)
		}
		if st.LastSeenVersion != "1.2.0" {
			t.Errorf("LastSeenVersion = %q", st.LastSeenVersion)
		}
	})

	t.Run("upgrade announces three launches then stops", func(t *testing.T) {
		st := State{LastSeenVersion: "1.1.0"}
		for i := 1; i <= AnnounceLaunches; i++ {
			a := st.PlanLaunch("v1.2.0", true, true)
			if a.Version != "1.2.0" || a.Since != "1.1.0" {
				t.Fatalf("launch %d: %+v", i, a)
			}
		}
		if a := st.PlanLaunch("1.2.0", true, true); a.Version != "" {
			t.Errorf("launch 4 announced %+v", a)
		}
		if st.LastSeenVersion != "1.2.0" {
			t.Errorf("LastSeenVersion = %q after the announcements ran out", st.LastSeenVersion)
		}
		if a := st.PlanLaunch("1.2.0", true, true); a.Version != "" {
			t.Errorf("launch 5 announced %+v", a)
		}
	})

	t.Run("a newer upgrade restarts the count", func(t *testing.T) {
		st := State{LastSeenVersion: "1.1.0", AnnouncedVersion: "1.2.0", AnnouncedLaunches: 2}
		if a := st.PlanLaunch("1.3.0", true, true); a.Version != "1.3.0" || st.AnnouncedLaunches != 1 {
			t.Errorf("got %+v, launches %d", a, st.AnnouncedLaunches)
		}
	})

	t.Run("existing user without state", func(t *testing.T) {
		var st State
		a := st.PlanLaunch("1.2.0", true, true)
		if a.Version != "1.2.0" || a.Since != "" {
			t.Errorf("got %+v", a)
		}
	})

	t.Run("same version and downgrade", func(t *testing.T) {
		for _, v := range []string{"1.2.0", "1.1.0"} {
			st := State{LastSeenVersion: "1.2.0"}
			if a := st.PlanLaunch(v, true, true); a.Version != "" || st.LastSeenVersion != "1.2.0" {
				t.Errorf("%s: got %+v, state %+v", v, a, st)
			}
		}
	})

	t.Run("dev and prerelease change nothing", func(t *testing.T) {
		for _, v := range []string{"dev", "1.3.0-rc.1", "v0.0.0-20260101120000-abcdef123456"} {
			st := State{LastSeenVersion: "1.2.0"}
			if a := st.PlanLaunch(v, true, true); a.Version != "" || st != (State{LastSeenVersion: "1.2.0"}) {
				t.Errorf("%s: got %+v, state %+v", v, a, st)
			}
		}
	})

	t.Run("disabled records silently", func(t *testing.T) {
		st := State{LastSeenVersion: "1.1.0"}
		if a := st.PlanLaunch("1.2.0", true, false); a.Version != "" || st.LastSeenVersion != "1.2.0" {
			t.Errorf("got %+v, state %+v", a, st)
		}
	})
}

func TestMarkSeen(t *testing.T) {
	st := State{LastSeenVersion: "1.1.0"}
	st.MarkSeen("dev")
	st.MarkSeen("1.3.0-rc.1")
	if st.LastSeenVersion != "1.1.0" {
		t.Errorf("non-release changed LastSeenVersion to %q", st.LastSeenVersion)
	}
	st.MarkSeen("v1.2.0")
	if st.LastSeenVersion != "1.2.0" {
		t.Errorf("LastSeenVersion = %q, want 1.2.0", st.LastSeenVersion)
	}
	st.MarkSeen("1.0.0")
	if st.LastSeenVersion != "1.2.0" {
		t.Errorf("older version lowered LastSeenVersion to %q", st.LastSeenVersion)
	}
}
