package app

import (
	"github.com/utkarsh261/pho/internal/application/stack"
	"github.com/utkarsh261/pho/internal/domain"
)

// rebuildStacks finds stacks among the selected repo's loaded open PRs and
// the Involving tab's, then hands them to the dashboard panels.
func (m *Model) rebuildStacks() {
	open := m.allOpenPRs()
	seen := make(map[int]bool, len(open))
	for _, pr := range open {
		seen[pr.Number] = true
	}
	for _, pr := range m.currentInvolving.PRs {
		if !seen[pr.Number] && sameRepo(pr.Repo, m.selectedRepoName()) {
			open = append(open, pr)
		}
	}
	m.stacks = stack.Build(open)
	m.prList.SetStacks(m.stacks)
	m.preview.SetStacks(m.stacks)
}

// groupStacks lists each stack's PRs together, top of the stack first.
func (m *Model) groupStacks(prs []domain.PullRequestSummary) []domain.PullRequestSummary {
	if m.deps.NoStackGrouping {
		return prs
	}
	return stack.Group(prs, m.stacks)
}
