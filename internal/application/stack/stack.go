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
	// Each PR reports the stack's first page of layers plus itself, so the
	// layers of a long stack are spread over its PRs: combine them all.
	type stackID struct {
		repo   string
		number int
	}
	type native struct {
		trunk  string
		layers map[int]domain.NativeStackLayer // by position
	}
	stacks := map[stackID]*native{}
	var order []stackID
	for _, pr := range prs {
		ns := pr.NativeStack
		if ns == nil {
			continue
		}
		id := stackID{strings.ToLower(pr.Repo), ns.Number}
		n, ok := stacks[id]
		if !ok {
			n = &native{trunk: ns.Trunk, layers: map[int]domain.NativeStackLayer{}}
			stacks[id] = n
			order = append(order, id)
		}
		for _, l := range ns.Layers {
			if _, seen := n.layers[l.Position]; !seen {
				n.layers[l.Position] = l
			}
		}
	}
	for _, id := range order {
		n := stacks[id]
		if len(n.layers) < 2 {
			continue
		}
		layers := make([]domain.NativeStackLayer, 0, len(n.layers))
		for _, l := range n.layers {
			layers = append(layers, l)
		}
		sort.Slice(layers, func(i, j int) bool { return layers[i].Position > layers[j].Position })
		repo := id.repo
		st := &domain.Stack{Trunk: n.trunk, Native: true}
		for i, l := range layers {
			m := domain.StackMember{
				Number:  l.Number,
				Title:   l.Title,
				State:   l.State,
				IsDraft: l.IsDraft,
				Layer:   l.Position,
			}
			// Layers no loaded PR reported leave gaps; link across them.
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
		if !eligible(pr, idx) || pr.IsCrossRepository || pr.HeadRefName == "" || longLived(pr.HeadRefName) {
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
	for p, kids := range children {
		if len(kids) > maxChildren {
			// So many PRs on one branch make it a shared trunk, not a stack.
			for _, kid := range kids {
				delete(parent, kid)
			}
			delete(children, p)
			continue
		}
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

// maxChildren is the most PRs one PR can carry before its branch counts as
// a shared trunk rather than the base of a stack.
const maxChildren = 5

// longLived reports whether branch is one teams merge into rather than
// stack on, as in git-flow: a PR from develop is a release, not a stack.
func longLived(branch string) bool {
	switch strings.ToLower(branch) {
	case "main", "master", "trunk", "develop", "development", "dev", "next", "staging", "production":
		return true
	}
	return strings.HasPrefix(branch, "release/") || strings.HasPrefix(branch, "releases/")
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
