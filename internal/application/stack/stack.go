// Package stack finds stacked pull requests: PRs whose base branch is
// another open PR's head branch, or that GitHub reports as a native stack.
package stack

import (
	"sort"
	"strings"

	"github.com/utkarsh261/pho/internal/domain"
)

// Build indexes every stack among prs. Native GitHub stacks come first; the
// remaining PRs are chained by matching base branches to head branches.
func Build(prs []domain.PullRequestSummary) domain.StackIndex {
	prs = append([]domain.PullRequestSummary(nil), prs...) // members point into this copy
	idx := domain.StackIndex{}
	byKey := make(map[string]*domain.PullRequestSummary, len(prs))
	for i := range prs {
		byKey[domain.StackKey(prs[i].Repo, prs[i].Number)] = &prs[i]
	}
	buildNative(prs, byKey, idx)
	buildInferred(prs, idx)
	return idx
}

func buildNative(prs []domain.PullRequestSummary, byKey map[string]*domain.PullRequestSummary, idx domain.StackIndex) {
	// One PR's view of a native stack lists every layer, so take the first
	// PR seen for each stack (preferring the one listing the most layers).
	type stackID struct {
		repo   string
		number int
	}
	best := map[stackID]*domain.NativeStack{}
	var order []stackID
	for _, pr := range prs {
		ns := pr.NativeStack
		if ns == nil || len(ns.Layers) < 2 {
			continue
		}
		id := stackID{strings.ToLower(pr.Repo), ns.Number}
		if cur, ok := best[id]; !ok {
			order = append(order, id)
			best[id] = ns
		} else if len(ns.Layers) > len(cur.Layers) {
			best[id] = ns
		}
	}
	for _, id := range order {
		ns := best[id]
		layers := append([]domain.NativeStackLayer(nil), ns.Layers...)
		sort.Slice(layers, func(i, j int) bool { return layers[i].Position > layers[j].Position })
		repo := id.repo
		st := &domain.Stack{Trunk: ns.Trunk, Native: true}
		for i, l := range layers {
			m := domain.StackMember{
				Number:  l.Number,
				Title:   l.Title,
				State:   l.State,
				IsDraft: l.IsDraft,
				Layer:   len(layers) - i,
			}
			if i+1 < len(layers) {
				m.Parent = layers[i+1].Number
			}
			if i > 0 {
				m.Children = []int{layers[i-1].Number}
			}
			if s, ok := byKey[domain.StackKey(repo, l.Number)]; ok {
				m.Summary = s
				m.Title, m.State, m.IsDraft = s.Title, s.State, s.IsDraft
				st.Repo = s.Repo
			}
			st.Members = append(st.Members, m)
		}
		if st.Repo == "" {
			st.Repo = repo
		}
		for _, m := range st.Members {
			idx[domain.StackKey(repo, m.Number)] = st
		}
	}
}

func buildInferred(prs []domain.PullRequestSummary, idx domain.StackIndex) {
	// The PR that owns each head branch. Fork heads can't be a base here.
	heads := map[string]*domain.PullRequestSummary{}
	for i := range prs {
		pr := &prs[i]
		if !eligible(pr, idx) || pr.IsCrossRepository || pr.HeadRefName == "" {
			continue
		}
		k := branchKey(pr.Repo, pr.HeadRefName)
		if cur, ok := heads[k]; !ok || pr.UpdatedAt.After(cur.UpdatedAt) {
			heads[k] = pr
		}
	}

	parent := map[*domain.PullRequestSummary]*domain.PullRequestSummary{}
	children := map[*domain.PullRequestSummary][]*domain.PullRequestSummary{}
	for i := range prs {
		pr := &prs[i]
		if !eligible(pr, idx) {
			continue
		}
		p, ok := heads[branchKey(pr.Repo, pr.BaseRefName)]
		if !ok || p == pr {
			continue
		}
		parent[pr] = p
		children[p] = append(children[p], pr)
	}
	for _, kids := range children {
		sort.Slice(kids, func(i, j int) bool { return kids[i].Number > kids[j].Number })
	}

	// Roots in list order so stacks are built deterministically. PRs caught
	// in a base/head cycle have no root and are left unstacked.
	for i := range prs {
		root := &prs[i]
		if _, hasParent := parent[root]; hasParent || len(children[root]) == 0 {
			continue
		}
		st := &domain.Stack{Repo: root.Repo, Trunk: root.BaseRefName}
		var walk func(pr *domain.PullRequestSummary, layer, indent int)
		walk = func(pr *domain.PullRequestSummary, layer, indent int) {
			for i, kid := range children[pr] {
				ki := indent
				if i > 0 {
					ki++
				}
				walk(kid, layer+1, ki)
			}
			m := domain.StackMember{
				Number:  pr.Number,
				Title:   pr.Title,
				State:   pr.State,
				IsDraft: pr.IsDraft,
				Layer:   layer,
				Indent:  indent,
				Summary: pr,
			}
			if p, ok := parent[pr]; ok {
				m.Parent = p.Number
			}
			for _, kid := range children[pr] {
				m.Children = append(m.Children, kid.Number)
			}
			st.Members = append(st.Members, m)
		}
		walk(root, 1, 0)
		for _, m := range st.Members {
			idx[domain.StackKey(st.Repo, m.Number)] = st
		}
	}
}

// eligible reports whether pr can join an inferred stack: it must be open
// and not already in a native one.
func eligible(pr *domain.PullRequestSummary, idx domain.StackIndex) bool {
	if pr.State != "" && pr.State != domain.PRStateOpen {
		return false
	}
	return idx.Lookup(pr.Repo, pr.Number) == nil
}

func branchKey(repo, branch string) string {
	return strings.ToLower(repo) + "\x00" + branch
}

// Group reorders prs so the members of each stack sit together, top-first,
// at the place of the stack's first PR in the list. PRs outside any stack,
// and stacks with only one member in the list, keep their places.
func Group(prs []domain.PullRequestSummary, idx domain.StackIndex) []domain.PullRequestSummary {
	if len(idx) == 0 || len(prs) < 2 {
		return prs
	}
	present := map[*domain.Stack]map[int]int{} // stack -> number -> index in prs
	for i, pr := range prs {
		if st := idx.Lookup(pr.Repo, pr.Number); st != nil {
			if present[st] == nil {
				present[st] = map[int]int{}
			}
			present[st][pr.Number] = i
		}
	}
	out := make([]domain.PullRequestSummary, 0, len(prs))
	done := map[*domain.Stack]bool{}
	for _, pr := range prs {
		st := idx.Lookup(pr.Repo, pr.Number)
		if st == nil || len(present[st]) < 2 {
			out = append(out, pr)
			continue
		}
		if done[st] {
			continue
		}
		done[st] = true
		for _, m := range st.Members {
			if i, ok := present[st][m.Number]; ok {
				out = append(out, prs[i])
			}
		}
	}
	return out
}
