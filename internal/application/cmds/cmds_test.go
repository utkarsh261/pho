package cmds_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

func TestResolveViewerCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &viewerService{fn: func(ctx context.Context, host string) (string, error) {
			if host != "github.com" {
				t.Fatalf("host = %q, want github.com", host)
			}
			return "octocat", nil
		}}

		msg := run(t, cmds.ResolveViewerCmd(svc, "github.com"))
		got, ok := msg.(cmds.ViewerResolved)
		if !ok {
			t.Fatalf("message type = %T, want cmds.ViewerResolved", msg)
		}
		if got.Login != "octocat" || got.Err != nil || got.Host != "github.com" {
			t.Fatalf("message = %#v, want login=octocat host=github.com and no error", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := &viewerService{fn: func(ctx context.Context, host string) (string, error) {
			return "", wantErr
		}}

		msg := run(t, cmds.ResolveViewerCmd(svc, "github.com"))
		got := msg.(cmds.ViewerResolved)
		if !errors.Is(got.Err, wantErr) || got.Login != "" {
			t.Fatalf("message = %#v, want error and empty login", got)
		}
	})
}

func TestDiscoverReposCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repoA := repo("org/a")
		repoB := repo("org/b")
		svc := &discoveryService{fn: func(ctx context.Context, root string) ([]domain.Repository, error) {
			if root != "/tmp/workspace" {
				t.Fatalf("root = %q, want /tmp/workspace", root)
			}
			return []domain.Repository{repoA, repoB}, nil
		}}

		msg := run(t, cmds.DiscoverReposCmd(svc, "/tmp/workspace"))
		got := msg.(cmds.ReposDiscovered)
		if got.Err != nil {
			t.Fatalf("message = %#v, want no error", got)
		}
		if len(got.Repos) != 2 || got.Repos[0].FullName != repoA.FullName || got.Repos[1].FullName != repoB.FullName {
			t.Fatalf("message = %#v, want repos returned intact", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("disk failed")
		svc := &discoveryService{fn: func(ctx context.Context, root string) ([]domain.Repository, error) {
			return nil, wantErr
		}}

		msg := run(t, cmds.DiscoverReposCmd(svc, "/tmp/workspace"))
		got := msg.(cmds.ReposDiscovered)
		if !errors.Is(got.Err, wantErr) || got.Repos != nil {
			t.Fatalf("message = %#v, want error and nil repos", got)
		}
	})
}

func TestLoadDashboardCmd(t *testing.T) {
	repo := repo("org/a")
	snap := domain.DashboardSnapshot{Repo: repo, FetchedAt: time.Unix(100, 0)}

	t.Run("success", func(t *testing.T) {
		svc := &dashboardService{loadRepoFn: func(ctx context.Context, gotRepo domain.Repository, force bool) (domain.DashboardSnapshot, error) {
			if gotRepo.FullName != repo.FullName || !force {
				t.Fatalf("load args = %#v, force=%v", gotRepo, force)
			}
			return snap, nil
		}}

		msg := run(t, cmds.LoadDashboardCmd(svc, repo, true))
		got := msg.(cmds.DashboardLoaded)
		if got.Repo != repo.FullName || got.Err != nil || got.Snapshot.Repo.FullName != repo.FullName {
			t.Fatalf("message = %#v, want dashboard snapshot", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("network")
		svc := &dashboardService{loadRepoFn: func(ctx context.Context, gotRepo domain.Repository, force bool) (domain.DashboardSnapshot, error) {
			return domain.DashboardSnapshot{}, wantErr
		}}

		msg := run(t, cmds.LoadDashboardCmd(svc, repo, false))
		got := msg.(cmds.DashboardLoaded)
		if !errors.Is(got.Err, wantErr) || got.Repo != repo.FullName {
			t.Fatalf("message = %#v, want error and repo identity", got)
		}
	})
}

func TestLoadInvolvingCmd(t *testing.T) {
	repo := repo("org/a")
	snap := domain.InvolvingSnapshot{Repo: repo, FetchedAt: time.Unix(100, 0)}

	t.Run("success", func(t *testing.T) {
		svc := &dashboardService{loadInvolvingFn: func(ctx context.Context, gotRepo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error) {
			if gotRepo.FullName != repo.FullName || viewer != "octocat" || !force {
				t.Fatalf("load args = %#v viewer=%q force=%v", gotRepo, viewer, force)
			}
			return snap, nil
		}}

		msg := run(t, cmds.LoadInvolvingCmd(svc, repo, "octocat", true))
		got := msg.(cmds.InvolvingLoaded)
		if got.Repo != repo.FullName || got.Err != nil || got.Snapshot.Repo.FullName != repo.FullName {
			t.Fatalf("message = %#v, want involving snapshot", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := &dashboardService{loadInvolvingFn: func(ctx context.Context, gotRepo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error) {
			return domain.InvolvingSnapshot{}, wantErr
		}}

		msg := run(t, cmds.LoadInvolvingCmd(svc, repo, "octocat", false))
		got := msg.(cmds.InvolvingLoaded)
		if !errors.Is(got.Err, wantErr) || got.Repo != repo.FullName {
			t.Fatalf("message = %#v, want error and repo identity", got)
		}
	})
}

func TestLoadRecentCmd(t *testing.T) {
	repo := repo("org/a")
	snap := domain.RecentSnapshot{Repo: repo, FetchedAt: time.Unix(100, 0)}

	t.Run("success", func(t *testing.T) {
		svc := &dashboardService{loadRecentFn: func(ctx context.Context, gotRepo domain.Repository, force bool) (domain.RecentSnapshot, error) {
			if gotRepo.FullName != repo.FullName || force {
				t.Fatalf("load args = %#v force=%v", gotRepo, force)
			}
			return snap, nil
		}}

		msg := run(t, cmds.LoadRecentCmd(svc, repo, false))
		got := msg.(cmds.RecentLoaded)
		if got.Repo != repo.FullName || got.Err != nil || got.Snapshot.Repo.FullName != repo.FullName {
			t.Fatalf("message = %#v, want recent snapshot", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := &dashboardService{loadRecentFn: func(ctx context.Context, gotRepo domain.Repository, force bool) (domain.RecentSnapshot, error) {
			return domain.RecentSnapshot{}, wantErr
		}}

		msg := run(t, cmds.LoadRecentCmd(svc, repo, true))
		got := msg.(cmds.RecentLoaded)
		if !errors.Is(got.Err, wantErr) || got.Repo != repo.FullName {
			t.Fatalf("message = %#v, want error and repo identity", got)
		}
	})
}

func TestLoadPreviewCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &dashboardService{loadPreviewFn: func(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error) {
			if repo != "org/a" || number != 99 {
				t.Fatalf("load args = %q #%d", repo, number)
			}
			return domain.PRPreviewSnapshot{Repo: repo, Number: number}, nil
		}}

		msg := run(t, cmds.LoadPreviewCmd(svc, "org/a", 99, ""))
		got := msg.(cmds.PreviewLoaded)
		if got.Repo != "org/a" || got.Number != 99 || got.Err != nil {
			t.Fatalf("message = %#v, want preview snapshot", got)
		}
	})

	t.Run("with host", func(t *testing.T) {
		svc := &dashboardService{loadPreviewFn: func(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error) {
			// Should receive "host/org/a" format
			if repo != "github.com/org/a" || number != 42 {
				t.Fatalf("load args = %q #%d, want github.com/org/a #42", repo, number)
			}
			return domain.PRPreviewSnapshot{Repo: repo, Number: number}, nil
		}}

		msg := run(t, cmds.LoadPreviewCmd(svc, "org/a", 42, "github.com"))
		got := msg.(cmds.PreviewLoaded)
		// Message should contain clean repo "org/a", not "github.com/org/a"
		if got.Repo != "org/a" || got.Number != 42 || got.Err != nil {
			t.Fatalf("message = %#v, want org/a #42", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("not found")
		svc := &dashboardService{loadPreviewFn: func(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error) {
			return domain.PRPreviewSnapshot{}, wantErr
		}}

		msg := run(t, cmds.LoadPreviewCmd(svc, "org/a", 99, ""))
		got := msg.(cmds.PreviewLoaded)
		if !errors.Is(got.Err, wantErr) || got.Repo != "org/a" || got.Number != 99 {
			t.Fatalf("message = %#v, want error and repo identity", got)
		}
	})
}

func TestRebuildPRIndexCmd(t *testing.T) {
	repo := repo("org/a")
	snap := domain.DashboardSnapshot{Repo: repo, FetchedAt: time.Unix(100, 0)}

	t.Run("success", func(t *testing.T) {
		svc := &searchService{buildPRIndexFn: func(gotRepo domain.Repository, gotSnap domain.DashboardSnapshot) error {
			if gotRepo.FullName != repo.FullName || gotSnap.Repo.FullName != repo.FullName {
				t.Fatalf("build args = %#v snap=%#v", gotRepo, gotSnap)
			}
			return nil
		}}

		msg := run(t, cmds.RebuildPRIndexCmd(svc, repo, snap))
		got := msg.(cmds.SearchIndexRebuilt)
		if got.Repo != repo.FullName || got.Err != nil {
			t.Fatalf("message = %#v, want rebuilt repo identity", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := &searchService{buildPRIndexFn: func(gotRepo domain.Repository, gotSnap domain.DashboardSnapshot) error {
			return wantErr
		}}

		msg := run(t, cmds.RebuildPRIndexCmd(svc, repo, snap))
		got := msg.(cmds.SearchIndexRebuilt)
		if !errors.Is(got.Err, wantErr) || got.Repo != repo.FullName {
			t.Fatalf("message = %#v, want error and repo identity", got)
		}
	})
}

func TestRebuildRepoIndexCmd(t *testing.T) {
	repos := []domain.Repository{repo("org/a"), repo("org/b")}

	t.Run("success", func(t *testing.T) {
		svc := &searchService{buildRepoIndexFn: func(gotRepos []domain.Repository) error {
			if len(gotRepos) != len(repos) || gotRepos[0].FullName != repos[0].FullName || gotRepos[1].FullName != repos[1].FullName {
				t.Fatalf("build repos = %#v", gotRepos)
			}
			return nil
		}}

		msg := run(t, cmds.RebuildRepoIndexCmd(svc, repos))
		got := msg.(cmds.SearchIndexRebuilt)
		if got.Err != nil || got.Repo != "" {
			t.Fatalf("message = %#v, want success with empty repo identity", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := &searchService{buildRepoIndexFn: func(gotRepos []domain.Repository) error {
			return wantErr
		}}

		msg := run(t, cmds.RebuildRepoIndexCmd(svc, repos))
		got := msg.(cmds.SearchIndexRebuilt)
		if !errors.Is(got.Err, wantErr) {
			t.Fatalf("message = %#v, want error", got)
		}
	})
}

func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	return cmd()
}

type viewerService struct {
	fn func(context.Context, string) (string, error)
}

func (s *viewerService) FetchViewer(ctx context.Context, host string) (string, error) {
	return s.fn(ctx, host)
}

type discoveryService struct {
	fn func(context.Context, string) ([]domain.Repository, error)
}

func (s *discoveryService) Discover(ctx context.Context, root string) ([]domain.Repository, error) {
	return s.fn(ctx, root)
}

type dashboardService struct {
	loadRepoFn      func(context.Context, domain.Repository, bool) (domain.DashboardSnapshot, error)
	loadInvolvingFn func(context.Context, domain.Repository, string, bool) (domain.InvolvingSnapshot, error)
	loadRecentFn    func(context.Context, domain.Repository, bool) (domain.RecentSnapshot, error)
	loadPreviewFn   func(context.Context, string, int) (domain.PRPreviewSnapshot, error)
}

func (s *dashboardService) LoadRepo(ctx context.Context, repo domain.Repository, force bool) (domain.DashboardSnapshot, error) {
	return s.loadRepoFn(ctx, repo, force)
}

func (s *dashboardService) LoadInvolving(ctx context.Context, repo domain.Repository, viewer string, force bool) (domain.InvolvingSnapshot, error) {
	return s.loadInvolvingFn(ctx, repo, viewer, force)
}

func (s *dashboardService) LoadRecent(ctx context.Context, repo domain.Repository, force bool) (domain.RecentSnapshot, error) {
	return s.loadRecentFn(ctx, repo, force)
}

func (s *dashboardService) LoadPreview(ctx context.Context, repo string, number int) (domain.PRPreviewSnapshot, error) {
	return s.loadPreviewFn(ctx, repo, number)
}

type searchService struct {
	buildPRIndexFn   func(domain.Repository, domain.DashboardSnapshot) error
	buildRepoIndexFn func([]domain.Repository) error
}

func (s *searchService) BuildPRIndex(repo domain.Repository, snap domain.DashboardSnapshot) error {
	return s.buildPRIndexFn(repo, snap)
}

func (s *searchService) BuildRepoIndex(repos []domain.Repository) error {
	return s.buildRepoIndexFn(repos)
}

func repo(full string) domain.Repository {
	owner, name, _ := splitRepo(full)
	return domain.Repository{FullName: full, Owner: owner, Name: name, LocalPath: "/tmp/" + name}
}

func splitRepo(full string) (string, string, bool) {
	for i := 0; i < len(full); i++ {
		if full[i] == '/' {
			return full[:i], full[i+1:], true
		}
	}
	return "", full, false
}
