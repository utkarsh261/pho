// Package bench holds seeded fixture generators and measurement helpers for
// benchmarks. Only test code should import it.
package bench

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
)

// Epoch is the fixed clock fixtures are dated against.
var Epoch = time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

// SynthDiff builds files with 60-line hunks (20 context, 20 del, 20 add).
func SynthDiff(files, linesPerFile int) string {
	var b strings.Builder
	for f := 0; f < files; f++ {
		p := fmt.Sprintf("pkg/mod%d/file%d.go", f%50, f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1111111..2222222 100644\n--- a/%s\n+++ b/%s\n", p, p, p, p)
		n := 0
		for h := 0; n < linesPerFile; h++ {
			start := h*100 + 1
			fmt.Fprintf(&b, "@@ -%d,40 +%d,40 @@ func something%d() {\n", start, start, h)
			for i := 0; i < 20 && n < linesPerFile; i++ {
				fmt.Fprintf(&b, " \tctx := doTheThing(%d, \"some context line here\", value)\n", i)
				n++
			}
			for i := 0; i < 20 && n < linesPerFile; i++ {
				fmt.Fprintf(&b, "-\tresult := computeOldValue(input%d, options.Legacy, 42)\n", i)
				n++
			}
			for i := 0; i < 20 && n < linesPerFile; i++ {
				fmt.Fprintf(&b, "+\tresult := computeNewValue(input%d, options.Modern, 43)\n", i)
				n++
			}
		}
	}
	return b.String()
}

// SynthSmall builds review-sized hunks: 3 ctx, 3 del, 3 add, 3 ctx.
func SynthSmall(files, linesPerFile int) string {
	var b strings.Builder
	for f := 0; f < files; f++ {
		p := fmt.Sprintf("pkg/mod%d/file%d.go", f%50, f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1111111..2222222 100644\n--- a/%s\n+++ b/%s\n", p, p, p, p)
		for h := 0; h*12 < linesPerFile; h++ {
			start := h*40 + 1
			fmt.Fprintf(&b, "@@ -%d,9 +%d,9 @@ func f%d() {\n", start, start, h)
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, " \tctx := doTheThing(%d, value)\n", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, "-\tresult := computeOldValue(input%d, options.Legacy)\n", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, "+\tresult := computeNewValue(input%d, options.Modern)\n", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&b, " \treturn x%d\n", i)
			}
		}
	}
	return b.String()
}

var (
	words   = strings.Fields("fix add remove refactor cache parser login session cookie diff render theme stack review comment palette search index dashboard preview checks retry timeout budget scroll cursor anchor hunk")
	authors = []string{"octocat", "alice", "bob", "carol", "dave", "erin", "frank", "grace"}
)

func title(r *rand.Rand) string {
	n := 3 + r.IntN(6)
	ws := make([]string, n)
	for i := range ws {
		ws[i] = words[r.IntN(len(words))]
	}
	return strings.Join(ws, " ")
}

// Repos returns n repositories named org/repo-NN.
func Repos(n int) []domain.Repository {
	out := make([]domain.Repository, n)
	for i := range out {
		name := fmt.Sprintf("repo-%02d", i)
		out[i] = domain.Repository{
			Host: "github.com", Owner: "acme", Name: name,
			FullName: "acme/" + name, LocalPath: "/tmp/bench/" + name,
		}
	}
	return out
}

// PRs returns n open PRs for repo with seeded titles, authors and states.
// Every fifth PR is based on the previous one's branch, forming stacks.
func PRs(repo string, n int, seed uint64) []domain.PullRequestSummary {
	r := rand.New(rand.NewPCG(seed, 0x9e3779b9))
	out := make([]domain.PullRequestSummary, n)
	ci := []domain.CIStatus{domain.CIStatusSuccess, domain.CIStatusFailure, domain.CIStatusPending}
	rd := []domain.ReviewDecision{domain.ReviewDecisionApproved, domain.ReviewDecisionChangesRequested, domain.ReviewDecisionReviewRequired}
	for i := range out {
		num := i + 1
		head := fmt.Sprintf("feature/%s-%d", words[r.IntN(len(words))], num)
		base := "main"
		if i > 0 && i%5 != 0 {
			base = out[i-1].HeadRefName
		}
		out[i] = domain.PullRequestSummary{
			ID:                 fmt.Sprintf("PR_%s_%d", repo, num),
			Repo:               repo,
			Number:             num,
			Title:              title(r),
			Author:             authors[r.IntN(len(authors))],
			State:              domain.PRStateOpen,
			IsDraft:            r.IntN(10) == 0,
			ReviewDecision:     rd[r.IntN(len(rd))],
			CIStatus:           ci[r.IntN(len(ci))],
			CreatedAt:          Epoch.Add(-time.Duration(r.IntN(2000)) * time.Hour),
			UpdatedAt:          Epoch.Add(-time.Duration(r.IntN(200)) * time.Hour),
			HeadRefName:        head,
			HeadRefOID:         fmt.Sprintf("%040x", r.Uint64()),
			BaseRefName:        base,
			CommentCount:       r.IntN(40),
			Additions:          r.IntN(800),
			Deletions:          r.IntN(400),
			FileCount:          1 + r.IntN(60),
			RequestedReviewers: []string{authors[r.IntN(len(authors))]},
		}
	}
	return out
}

// Comments returns n top-level PR comments with markdown bodies.
func Comments(n int, seed uint64) []domain.PreviewComment {
	r := rand.New(rand.NewPCG(seed, 1))
	out := make([]domain.PreviewComment, n)
	for i := range out {
		out[i] = domain.PreviewComment{
			ID:        fmt.Sprintf("c%d", i),
			Login:     authors[r.IntN(len(authors))],
			Body:      Markdown(1+r.IntN(3), seed+uint64(i)),
			CreatedAt: Epoch.Add(-time.Duration(n-i) * time.Minute),
		}
	}
	return out
}

// Markdown returns a PR-description-like document with headings, lists,
// code blocks, links and inline code.
func Markdown(sections int, seed uint64) string {
	r := rand.New(rand.NewPCG(seed, 2))
	var b strings.Builder
	for s := 0; s < sections; s++ {
		fmt.Fprintf(&b, "## %s\n\n", title(r))
		for p := 0; p < 2; p++ {
			for w := 0; w < 40; w++ {
				switch r.IntN(12) {
				case 0:
					fmt.Fprintf(&b, "`%s` ", words[r.IntN(len(words))])
				case 1:
					fmt.Fprintf(&b, "[%s](https://github.com/acme/repo/pull/%d) ", words[r.IntN(len(words))], r.IntN(9999))
				case 2:
					fmt.Fprintf(&b, "**%s** ", words[r.IntN(len(words))])
				default:
					b.WriteString(words[r.IntN(len(words))] + " ")
				}
			}
			b.WriteString("\n\n")
		}
		for i := 0; i < 4; i++ {
			fmt.Fprintf(&b, "- %s\n", title(r))
		}
		b.WriteString("\n```go\nfunc example() error {\n\treturn computeNewValue(input, options.Modern)\n}\n```\n\n")
	}
	return b.String()
}
