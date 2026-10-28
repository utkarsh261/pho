package app

import (
	"github.com/utkarsh261/pho/internal/application/stack"
	"github.com/utkarsh261/pho/internal/domain"
)

// rebuildStacks finds stacks among the selected repo's loaded open PRs and
// the Involving tab's, then hands them to the dashboard panels.
func (m *Model) rebuildStacks(open []domain.PullRequestSummary) {
	prs := append([]domain.PullRequestSummary(nil), open...)
	if inv, ok := m.selectedInvolving(); ok {
		seen := make(map[int]bool, len(open))
		for _, pr := range open {
			seen[pr.Number] = true
		}
		for _, pr := range inv.PRs {
			if !seen[pr.Number] {
				prs = append(prs, pr)
			}
		}
	}
	m.stacks = stack.Build(prs)
	m.prList.SetStacks(m.stacks)
	m.preview.SetStacks(m.stacks)
}

// selectedInvolving is the Involving snapshot when it belongs to the
// selected repo; after a repo switch it still holds the old repo's.
func (m *Model) selectedInvolving() (domain.InvolvingSnapshot, bool) {
	inv := m.currentInvolving
	if isZeroInvolvingSnapshot(inv) {
		return inv, false
	}
	repo := inv.Repo.FullName
	if repo == "" && len(inv.PRs) > 0 {
		repo = inv.PRs[0].Repo
	}
	return inv, sameRepo(repo, m.selectedRepoName())
}

// groupStacks lists each stack's PRs together, top of the stack first. The
// result is always a fresh slice, safe to keep as a tab's list.
func (m *Model) groupStacks(prs []domain.PullRequestSummary) []domain.PullRequestSummary {
	out := append([]domain.PullRequestSummary(nil), prs...)
	if m.deps.NoStackGrouping {
		return out
	}
	return stack.Group(out, m.stacks)
}
