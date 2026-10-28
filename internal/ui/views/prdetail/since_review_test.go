package prdetail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/anchor"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/rest"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const (
	sinceFrom = "rev1234567890"
	sinceHead = "head234567890"
)

// The PR diff: a.go lines 1-4 and b.go.
const sinceFullDiff = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 package a
-func old() {}
+func newer() {}
+func extra() {}
 // end
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,2 +1,2 @@
 package b
-var x = 1
+var x = 2
`

// The changes since the review: one line the PR diff also has (a.go:3) and
// one it doesn't (a.go:11).
const sinceRangeDiff = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -2,2 +2,2 @@
 func newer() {}
-func extra() {}
+func extra() { return }
@@ -10,2 +10,2 @@
 ctx10
-old11
+new11
`

func parseTestDiff(t *testing.T, raw, sha string) diffmodel.DiffModel {
	t.Helper()
	dm, err := parse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dm.HeadSHA = sha
	anchor.Generate(dm, sha)
	cum := 0
	for i := range dm.Files {
		dm.Files[i].StartRow = cum
		cum += dm.Files[i].DisplayRows
	}
	return *dm
}

func sinceDetail(reviewSHA string) domain.PRPreviewSnapshot {
	d := domain.PRPreviewSnapshot{Number: 1, Repo: "owner/repo", HeadRefOID: sinceHead, Title: "Test PR"}
	if reviewSHA != "" {
		d.Reviewers = []domain.PreviewReviewer{{
			Login: "me", State: "COMMENTED", CommitSHA: reviewSHA,
			SubmittedAt: time.Now().Add(-48 * time.Hour),
		}}
	}
	return d
}

// newSinceModel opens a PR the viewer reviewed at reviewSHA with the PR diff
// loaded. The range diff is not loaded yet.
func newSinceModel(t *testing.T, reviewSHA string) *PRDetailModel {
	t.Helper()
	return newSinceModelWith(t, sinceDetail(reviewSHA), sinceFullDiff)
}

// newSinceModelWith opens a PR with the given detail and PR diff loaded.
func newSinceModelWith(t *testing.T, detail domain.PRPreviewSnapshot, fullRaw string) *PRDetailModel {
	t.Helper()
	m := NewModel(domain.PullRequestSummary{
		Number: 1, Title: "Test PR", Author: "alice", Repo: "owner/repo",
		State: domain.PRStateOpen, HeadRefOID: sinceHead, BaseRefName: "main",
	}, domain.Repository{FullName: "owner/repo"}, &prServiceStub{})
	m.Width, m.Height = 120, 30
	m.ViewerLogin = "me"
	m.SetTheme(theme.Default())
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 1, Detail: detail})
	m, _ = m.Update(cmds.DiffLoaded{Repo: "owner/repo", Number: 1, Diff: parseTestDiff(t, fullRaw, sinceHead)})
	m.leftPanel.Focus = FocusContent
	return m
}

func sinceRangeLoaded(t *testing.T, rd diffmodel.RangeDiff) cmds.RangeDiffLoaded {
	t.Helper()
	rd.FromSHA, rd.ToSHA = sinceFrom, sinceHead
	if rd.Commits == 0 {
		rd.Commits = 2
	}
	return cmds.RangeDiffLoaded{Repo: "owner/repo", Number: 1, FromSHA: sinceFrom, ToSHA: sinceHead, Range: rd}
}

func diffPaths(dm *diffmodel.DiffModel) string {
	if dm == nil {
		return "<nil>"
	}
	var p []string
	for _, f := range dm.Files {
		p = append(p, f.NewPath)
	}
	return strings.Join(p, ",")
}

// lineAt finds the hunk/line index of the line whose Raw is raw.
func lineAt(t *testing.T, f *diffmodel.DiffFile, raw string) (int, int) {
	t.Helper()
	for hi, h := range f.Hunks {
		for li, l := range h.Lines {
			if l.Raw == raw {
				return hi, li
			}
		}
	}
	t.Fatalf("no line %q", raw)
	return 0, 0
}

func TestSinceReviewPrefetchesAfterDetail(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	if !m.since.loading || m.since.from != sinceFrom || m.since.to != sinceHead {
		t.Fatalf("since = %+v, want a fetch of %s..%s", m.since, sinceFrom, sinceHead)
	}
	if m := newSinceModel(t, sinceHead); m.since.loading {
		t.Error("fetched although head is the reviewed commit")
	}
	if m := newSinceModel(t, ""); m.since.loading {
		t.Error("fetched without a review")
	}
}

func TestSinceReviewToggle(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	if m.since.active {
		t.Fatal("prefetch switched scope on its own")
	}

	m = pressKey(m, "s")
	if !m.since.active || diffPaths(m.Diff) != "a.go" || m.activeTab != TabDiff {
		t.Fatalf("after s: active=%v diff=%s tab=%v", m.since.active, diffPaths(m.Diff), m.activeTab)
	}
	if m.leftPanel.ScopeLabel != "since review" || len(m.leftPanel.Files) != 1 {
		t.Errorf("left panel = %q, %d files", m.leftPanel.ScopeLabel, len(m.leftPanel.Files))
	}
	if hint := m.StatusHint(); !strings.Contains(hint, "Showing 2 commits since your review on rev1234 (2d)") {
		t.Errorf("status = %q", hint)
	}

	m = pressKey(m, "s")
	if m.since.active || diffPaths(m.Diff) != "a.go,b.go" || m.leftPanel.ScopeLabel != "" {
		t.Fatalf("after second s: active=%v diff=%s", m.since.active, diffPaths(m.Diff))
	}
}

func TestSinceReviewFromOtherTabSwitchesToDiff(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m.switchTab(TabComments)
	m = pressKey(m, "s")
	if m.activeTab != TabDiff || !m.since.active {
		t.Errorf("tab=%v active=%v", m.activeTab, m.since.active)
	}
}

func TestSinceReviewNotices(t *testing.T) {
	tests := []struct {
		name   string
		review string
		want   string
	}{
		{"no review", "", "You haven't reviewed this PR yet"},
		{"reviewed head", sinceHead, "No changes since your review"},
		{"still loading", sinceFrom, "Loading changes since your review…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pressKey(newSinceModel(t, tt.review), "s")
			if m.since.active {
				t.Error("scope switched on")
			}
			if got := m.StatusHint(); got != tt.want {
				t.Errorf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSinceReviewEntersWhenPendingLoadLands(t *testing.T) {
	m := pressKey(newSinceModel(t, sinceFrom), "s")
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	if !m.since.active || diffPaths(m.Diff) != "a.go" {
		t.Errorf("active=%v diff=%s", m.since.active, diffPaths(m.Diff))
	}
}

func TestSinceReviewRewrittenStaysOnFullDiff(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Rewritten: true}))
	if m.hasNewSinceReview() {
		t.Error("rewritten range flagged as new changes")
	}
	m = pressKey(m, "s")
	if m.since.active || diffPaths(m.Diff) != "a.go,b.go" {
		t.Errorf("active=%v diff=%s", m.since.active, diffPaths(m.Diff))
	}
	if got := m.StatusHint(); !strings.Contains(got, "rebased since your review on rev1234") {
		t.Errorf("status = %q", got)
	}
}

func TestSinceReviewLoadErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("fetch range diff: %w", &rest.StatusError{Code: 404}), "Your reviewed commit rev1234 is no longer on GitHub"},
		{errors.New("boom"), "Couldn't load changes since your review · s: retry"},
	} {
		m := pressKey(newSinceModel(t, sinceFrom), "s")
		msg := sinceRangeLoaded(t, diffmodel.RangeDiff{})
		msg.Err = tc.err
		m, _ = m.Update(msg)
		if m.since.active || m.StatusHint() != tc.want {
			t.Errorf("active=%v status=%q, want %q", m.since.active, m.StatusHint(), tc.want)
		}
		// s retries after a failure.
		m = pressKey(m, "s")
		if !m.since.loading {
			t.Error("s after a failure did not refetch")
		}
	}
}

func TestSinceReviewIgnoresStaleRange(t *testing.T) {
	m := pressKey(newSinceModel(t, sinceFrom), "s")
	msg := sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, "older")})
	msg.ToSHA = "older"
	m, _ = m.Update(msg)
	if m.since.active || m.since.rng != nil || !m.since.loading {
		t.Errorf("stale range applied: %+v", m.since)
	}
}

func TestSinceReviewDiffLoadedKeepsRangeOnScreen(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m = pressKey(m, "s")
	m, _ = m.Update(cmds.DiffLoaded{Repo: "owner/repo", Number: 1, Diff: parseTestDiff(t, sinceFullDiff, sinceHead)})
	if diffPaths(m.Diff) != "a.go" {
		t.Fatalf("DiffLoaded replaced the range diff: %s", diffPaths(m.Diff))
	}
	m = pressKey(m, "s")
	if diffPaths(m.Diff) != "a.go,b.go" {
		t.Errorf("full diff not restored: %s", diffPaths(m.Diff))
	}
}

func TestSinceReviewExitsWhenViewerReviewsHead(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m = pressKey(m, "s")
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 1, Detail: sinceDetail(sinceHead)})
	if m.since.active || diffPaths(m.Diff) != "a.go,b.go" {
		t.Errorf("active=%v diff=%s", m.since.active, diffPaths(m.Diff))
	}
}

func TestSinceReviewHidesLeftDrafts(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m.drafts = []domain.DraftInlineComment{
		{Path: "a.go", Line: 3, Side: "RIGHT", Body: "right"},
		{Path: "a.go", Line: 3, Side: "LEFT", Body: "left"},
	}
	m = pressKey(m, "s")
	f := &m.Diff.Files[0]
	rhi, rli := lineAt(t, f, "func extra() { return }")
	lhi, lli := lineAt(t, f, "func extra() {}")
	if !m.draftCovered[hunkLineKey{0, rhi, rli}] {
		t.Error("RIGHT draft not shown")
	}
	if m.draftCovered[hunkLineKey{0, lhi, lli}] {
		t.Error("LEFT draft shown on the range diff")
	}
}

func TestSinceReviewCommentOnlyOnPRDiffLines(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"func extra() { return }", true}, // a.go:3 is in the PR diff
		{"new11", false},                  // a.go:11 is not
		{"func extra() {}", false},        // removed lines refer to the reviewed commit
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			m := newSinceModel(t, sinceFrom)
			m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
			m = pressKey(m, "s")
			hi, li := lineAt(t, &m.Diff.Files[0], tt.raw)
			m.visual = visualModeState{Active: true, FileIdx: 0, HunkIdx: hi, StartLine: li, EndLine: li}
			m = pressKey(m, "c")
			if m.compose.active != tt.want {
				t.Errorf("compose open = %v, want %v (status %q)", m.compose.active, tt.want, m.StatusHint())
			}
			if !tt.want && !strings.HasPrefix(m.StatusHint(), "Can't comment here") {
				t.Errorf("status = %q", m.StatusHint())
			}
		})
	}
}

// sinceTop renders the header and the panels' tab rows: everything the
// since-review scope changes outside the diff body.
func sinceTop(m *PRDetailModel) string {
	lines := strings.Split(plainText(m.View()), "\n")
	return strings.Join(lines[:min(len(lines), 8)], "\n") + "\n"
}

func TestSinceReviewTabBarGolden(t *testing.T) {
	for _, w := range []int{80, 120} {
		m := newSinceModel(t, sinceFrom)
		m.Width = w
		m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Commits: 3, Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))

		checkGolden(t, sinceTop(m), fmt.Sprintf("since_review_desc_dot_w%d.txt", w))

		m.switchTab(TabDiff)
		checkGolden(t, sinceTop(m), fmt.Sprintf("since_review_nudge_w%d.txt", w))

		m = pressKey(m, "s")
		checkGolden(t, sinceTop(m), fmt.Sprintf("since_review_active_w%d.txt", w))

		m.since.rng.BaseMerged = true
		checkGolden(t, sinceTop(m), fmt.Sprintf("since_review_merged_base_w%d.txt", w))
	}
}

func TestSinceReviewNarrowHeader(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m.Width = 70
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m = pressKey(m, "s")
	if out := plainText(m.View()); !strings.Contains(out, "1 files changed since rev1234") {
		t.Errorf("narrow header missing scope:\n%s", out)
	}
}

// rangeStub records LoadRangeDiff calls.
type rangeStub struct {
	prServiceStub
	calls int
}

func (s *rangeStub) LoadRangeDiff(_ context.Context, _ domain.Repository, _ int, _, _ string, _ bool) (diffmodel.RangeDiff, error) {
	s.calls++
	return diffmodel.RangeDiff{}, nil
}

func TestSinceReviewCmdHitsService(t *testing.T) {
	stub := &rangeStub{}
	m := NewModel(domain.PullRequestSummary{Number: 1, Repo: "owner/repo", HeadRefOID: sinceHead},
		domain.Repository{FullName: "owner/repo"}, stub)
	m.ViewerLogin = "me"
	d := sinceDetail(sinceFrom)
	m.Detail = &d
	cmd := m.loadSinceReviewCmd()
	if cmd == nil {
		t.Fatal("no command")
	}
	msg, ok := cmd().(cmds.RangeDiffLoaded)
	if !ok || stub.calls != 1 || msg.FromSHA != sinceFrom || msg.ToSHA != sinceHead {
		t.Errorf("msg=%+v calls=%d", msg, stub.calls)
	}
	if m.loadSinceReviewCmd() != nil {
		t.Error("refetched while loading")
	}
}

func TestSinceReviewPillClearsAfterReviewingHead(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m.switchTab(TabDiff)
	if !strings.Contains(plainText(m.View()), "new since review") {
		t.Fatal("no pill before the new review")
	}
	// The viewer reviews the head; the old range is still cached.
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "owner/repo", Number: 1, Detail: sinceDetail(sinceHead)})
	if m.hasNewSinceReview() || strings.Contains(plainText(m.View()), "new since review") {
		t.Error("pill still shown after reviewing the head")
	}
}

func TestSinceReviewFindsReviewOutsideRecentWindow(t *testing.T) {
	// Only LatestReviews has the viewer's review, as when more than the
	// fetched window of later reviews came after it.
	d := sinceDetail("")
	d.LatestReviews = []domain.ReviewSummary{{AuthorLogin: "me", State: "COMMENTED", CommitSHA: sinceFrom, SubmittedAt: time.Now()}}
	m := newSinceModelWith(t, d, sinceFullDiff)
	if !m.since.loading || m.since.from != sinceFrom {
		t.Errorf("since = %+v, want a fetch from the review in LatestReviews", m.since)
	}
}

func TestSinceReviewSwapMarksOpenInlineDraftStale(t *testing.T) {
	m := pressKey(newSinceModel(t, sinceFrom), "s") // range still loading
	hi, li := lineAt(t, &m.Diff.Files[1], "var x = 2")
	m.visual = visualModeState{Active: true, FileIdx: 1, HunkIdx: hi, StartLine: li, EndLine: li}
	m.compose.Open(composeModeDraftInline, commentEntry{}, 0)

	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	if !m.since.active {
		t.Fatal("range did not land")
	}
	if !m.inlineDraftStale || m.visual.Active {
		t.Errorf("stale=%v visual=%v, want the draft marked stale and the selection dropped", m.inlineDraftStale, m.visual.Active)
	}
}

func TestSinceReviewResetsFileListOnSwap(t *testing.T) {
	m := newSinceModel(t, sinceFrom)
	m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, sinceRangeDiff, sinceHead)}))
	m.switchTab(TabDiff)
	m.leftPanel.Cursor, m.leftPanel.LastOpenedIndex, m.ContentScroll = 1, 1, 5
	m = pressKey(m, "s") // the range has one file
	if m.leftPanel.Cursor != 0 || m.leftPanel.LastOpenedIndex != 0 || m.ContentScroll != 0 {
		t.Errorf("cursor=%d lastOpened=%d scroll=%d, want all 0", m.leftPanel.Cursor, m.leftPanel.LastOpenedIndex, m.ContentScroll)
	}
}

// The PR diff has two hunks of c.go; the range has one hunk bridging them.
func TestSinceReviewCommentMustStayInOnePRHunk(t *testing.T) {
	full := "diff --git a/c.go b/c.go\n--- a/c.go\n+++ b/c.go\n" +
		"@@ -1,3 +1,3 @@\n l1\n-old2\n+new2\n l3\n" +
		"@@ -20,3 +20,3 @@\n l20\n-old21\n+new21\n l22\n"
	var rng strings.Builder
	rng.WriteString("diff --git a/c.go b/c.go\n--- a/c.go\n+++ b/c.go\n@@ -2,20 +2,20 @@\n new2\n")
	for i := 3; i <= 20; i++ {
		fmt.Fprintf(&rng, " l%d\n", i)
	}
	rng.WriteString("-new21\n+newer21\n")

	for _, tt := range []struct {
		from, to string
		want     bool
	}{
		{"new2", "l3", true},       // both in the PR's first hunk
		{"new2", "newer21", false}, // one end in each PR hunk
	} {
		m := newSinceModelWith(t, sinceDetail(sinceFrom), full)
		m, _ = m.Update(sinceRangeLoaded(t, diffmodel.RangeDiff{Diff: parseTestDiff(t, rng.String(), sinceHead)}))
		m = pressKey(m, "s")
		f := &m.Diff.Files[0]
		hi, start := lineAt(t, f, tt.from)
		_, end := lineAt(t, f, tt.to)
		m.visual = visualModeState{Active: true, FileIdx: 0, HunkIdx: hi, StartLine: start, EndLine: end}
		if got := m.sinceReviewCommentable(); got != tt.want {
			t.Errorf("%s..%s commentable = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
