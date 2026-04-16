package discovery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

func TestParseRemoteURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		host  string
		owner string
		repo  string
		ok    bool
	}{
		{
			name:  "scp",
			raw:   "git@github.com:org/repo.git",
			host:  "github.com",
			owner: "org",
			repo:  "repo",
			ok:    true,
		},
		{
			name:  "https",
			raw:   "https://github.com/org/repo.git",
			host:  "github.com",
			owner: "org",
			repo:  "repo",
			ok:    true,
		},
		{
			name:  "ssh",
			raw:   "ssh://git@github.com/org/repo.git",
			host:  "github.com",
			owner: "org",
			repo:  "repo",
			ok:    true,
		},
		{
			name:  "git protocol",
			raw:   "git://github.com/org/repo.git",
			host:  "github.com",
			owner: "org",
			repo:  "repo",
			ok:    true,
		},
		{
			name:  "enterprise host",
			raw:   "https://github.example.com/acme/project.git",
			host:  "github.example.com",
			owner: "acme",
			repo:  "project",
			ok:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, owner, repo, ok := parseRemoteURL(tt.raw)
			if ok != tt.ok || host != tt.host || owner != tt.owner || repo != tt.repo {
				t.Fatalf("parseRemoteURL(%q) = %q,%q,%q,%v; want %q,%q,%q,%v",
					tt.raw, host, owner, repo, ok, tt.host, tt.owner, tt.repo, tt.ok)
			}
		})
	}
}

func TestDiscover_ScansRootAndDirectChildren(t *testing.T) {
	root := t.TempDir()

	writeRepo(t, root, "https://github.com/org/cwd.git", time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC))
	writeRepo(t, filepath.Join(root, "pinned"), "git@github.com:org/pinned.git", time.Date(2024, 1, 13, 10, 0, 0, 0, time.UTC))
	writeRepo(t, filepath.Join(root, "recent"), "ssh://git@github.com/org/recent.git", time.Date(2024, 1, 14, 10, 0, 0, 0, time.UTC))
	writeRepo(t, filepath.Join(root, "alpha"), "git://github.com/org/alpha.git", time.Date(2024, 1, 12, 10, 0, 0, 0, time.UTC))
	writeRepo(t, filepath.Join(root, "excluded"), "https://github.com/org/excluded.git", time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))

	svc := New(Config{
		Pin:     []string{"org/pinned"},
		Exclude: []string{"org/excluded"},
	})

	repos, err := svc.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	got := visibleNames(repos)
	want := []string{"org/pinned", "org/cwd", "org/recent", "org/alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible order = %v, want %v", got, want)
	}

	if repos[0].Host != "github.com" || repos[0].Owner != "org" || repos[0].Name != "pinned" {
		t.Fatalf("unexpected first repo: %+v", repos[0])
	}
	if repos[1].LocalPath != root {
		t.Fatalf("cwd repo should be rooted at %q, got %q", root, repos[1].LocalPath)
	}
}

func TestDiscover_DeduplicatesByVisibleIdentity(t *testing.T) {
	root := t.TempDir()

	writeRepo(t, root, "https://github.com/org/dup.git", time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC))
	writeRepo(t, filepath.Join(root, "alt"), "git@github.com:org/dup.git", time.Date(2024, 2, 2, 10, 0, 0, 0, time.UTC))

	svc := New(Config{})
	repos, err := svc.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo after dedupe, got %d: %v", len(repos), visibleNames(repos))
	}
	if repos[0].LocalPath != root {
		t.Fatalf("cwd repo should win collision, got path %q", repos[0].LocalPath)
	}
}

func TestDiscover_UsesCachedSnapshotOnLaterFailure(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, root, "https://github.com/org/cached.git", time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC))

	svc := New(Config{})
	repos, err := svc.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("initial Discover() error = %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo initially, got %d", len(repos))
	}

	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove root: %v", err)
	}

	cached, err := svc.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("cached Discover() error = %v", err)
	}
	if !reflect.DeepEqual(visibleNames(cached), []string{"org/cached"}) {
		t.Fatalf("cached repos = %v", visibleNames(cached))
	}
}

// TestDiscover_SkipsRepoWithNoRemote ensures a git repo that has no remote
// configured does not abort the scan — other repos in the same root must
// still be returned.
func TestDiscover_SkipsRepoWithNoRemote(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)

	// A proper repo with a remote.
	writeRepo(t, filepath.Join(root, "with-remote"), "git@github.com:org/myrepo.git", now)

	// A git repo with an empty .git/config (no remote section).
	noRemoteDir := filepath.Join(root, "no-remote")
	if err := os.MkdirAll(filepath.Join(noRemoteDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noRemoteDir, ".git", "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := New(Config{})
	repos, err := svc.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := visibleNames(repos)
	if len(names) != 1 || names[0] != "org/myrepo" {
		t.Fatalf("expected [org/myrepo], got %v", names)
	}
}

func visibleNames(repos []domain.Repository) []string {
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repo.FullName)
	}
	return out
}

func writeRepo(t *testing.T, path, remote string, modTime time.Time) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, ".git", "config"), []byte("[remote \"origin\"]\n\turl = "+remote+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes repo dir: %v", err)
	}
	if err := os.Chtimes(filepath.Join(path, ".git"), modTime, modTime); err != nil {
		t.Fatalf("chtimes git dir: %v", err)
	}
	if err := os.Chtimes(filepath.Join(path, ".git", "config"), modTime, modTime); err != nil {
		t.Fatalf("chtimes config: %v", err)
	}
}
