package search

import (
	"fmt"
	"testing"

	"github.com/utkarsh261/pho/internal/bench"
	"github.com/utkarsh261/pho/internal/domain"
)

func BenchmarkSearchPRs(b *testing.B) {
	repos := bench.Repos(20)
	for _, total := range []int{1_000, 5_000, 20_000} {
		s := New()
		per := total / len(repos)
		for i, r := range repos {
			prs := bench.PRs(r.FullName, per, uint64(i+1))
			if err := s.BuildPRIndex(r, domain.DashboardSnapshot{Repo: r, PRs: prs[:per/2], FetchedAt: bench.Epoch}); err != nil {
				b.Fatal(err)
			}
			s.AppendJumpPRs(r.FullName, prs[per/2:])
		}
		s.SetCurrentRepo(repos[0].FullName)
		for _, q := range []string{"c", "pars", "refactor cac"} {
			b.Run(fmt.Sprintf("%dprs/q%d", total, len(q)), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					s.SearchPRs(q, 50)
				}
			})
		}
	}
}

func BenchmarkSearchRepos(b *testing.B) {
	s := New()
	if err := s.BuildRepoIndex(bench.Repos(200)); err != nil {
		b.Fatal(err)
	}
	for _, q := range []string{"r", "repo-1", "acme/repo-1"} {
		b.Run(fmt.Sprintf("200repos/q%d", len(q)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s.SearchRepos(q, 50)
			}
		})
	}
}
