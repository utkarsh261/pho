package prdetail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/domain"
)

var reviewerTestBase = time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

// makeReviewersModel returns a model with mixed reviewers (incl. a duplicate
// review by alice) and one comment-only user.
func makeReviewersModel(width, height int) *PRDetailModel {
	m := makePRDetail(width, height, nil, nil)
	m.Summary = domain.PullRequestSummary{
		Number: 142,
		Title:  "Fix auth token refresh",
		State:  domain.PRStateOpen,
		Author: "utkarsh261",
		Repo:   "owner/repo",
	}
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:        "owner/repo",
		Number:      142,
		Title:       "Fix auth token refresh",
		Author:      "utkarsh261",
		State:       domain.PRStateOpen,
		BodyExcerpt: "Refreshes auth tokens.",
		Reviewers: []domain.PreviewReviewer{
			{Login: "carol", State: "COMMENTED", Body: "some notes", SubmittedAt: reviewerTestBase.Add(-3 * time.Hour)},
			{Login: "alice", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-2 * time.Hour)},
			{Login: "dave", State: "CHANGES_REQUESTED", SubmittedAt: reviewerTestBase.Add(-1 * time.Hour)},
			{Login: "alice", State: "COMMENTED", SubmittedAt: reviewerTestBase.Add(-5 * time.Hour)},
		},
		Comments: []domain.PreviewComment{
			{ID: "c1", Login: "bob", Body: "thanks for the PR", CreatedAt: reviewerTestBase.Add(-30 * time.Minute)},
		},
	}
	return m
}

func badgesToString(badges []reviewerBadge) string {
	parts := make([]string, len(badges))
	for i, b := range badges {
		parts[i] = b.state + ":" + b.login
	}
	return strings.Join(parts, ",")
}

func TestReviewerSummariesDedupKeepsLatestState(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "COMMENTED", SubmittedAt: reviewerTestBase.Add(-3 * time.Hour)},
			{Login: "alice", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-1 * time.Hour)},
		},
	}
	got := badgesToString(m.reviewerSummaries())
	if got != "APPROVED:alice" {
		t.Errorf("expected latest review state APPROVED, got %q", got)
	}
}

func TestReviewerSummariesTieKeepsLaterEntry(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", SubmittedAt: reviewerTestBase},
			{Login: "alice", State: "COMMENTED", SubmittedAt: reviewerTestBase},
		},
	}
	got := badgesToString(m.reviewerSummaries())
	if got != "COMMENTED:alice" {
		t.Errorf("expected later slice entry to win on tie, got %q", got)
	}
}

func TestReviewerSummariesSkipsPendingAndEmptyLogin(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "", State: "APPROVED", SubmittedAt: reviewerTestBase},
			{Login: "ghost", State: "PENDING", SubmittedAt: reviewerTestBase},
			{Login: "alice", State: "DISMISSED", SubmittedAt: reviewerTestBase},
		},
	}
	got := badgesToString(m.reviewerSummaries())
	if got != "COMMENTED:alice" {
		t.Errorf("expected only alice (DISMISSED→COMMENTED), got %q", got)
	}
}

func TestReviewerSummariesIncludesCommentOnlyAuthors(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(80, 30)
	found := false
	for _, b := range m.reviewerSummaries() {
		if b.login == "bob" {
			found = true
			if b.state != reviewerStateCommented {
				t.Errorf("expected bob to be COMMENTED, got %q", b.state)
			}
		}
	}
	if !found {
		t.Error("expected comment-only author bob in strip")
	}
}

func TestReviewerSummariesCommentNeverDowngradesReview(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-5 * time.Hour)},
			{Login: "dave", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-1 * time.Hour)},
		},
		Comments: []domain.PreviewComment{
			{ID: "c1", Login: "alice", Body: "one more thing", CreatedAt: reviewerTestBase},
		},
	}
	// alice's newer comment bumps neither her state nor her ordering position.
	got := badgesToString(m.reviewerSummaries())
	if got != "APPROVED:dave,APPROVED:alice" {
		t.Errorf("expected dave (newer review) before alice, got %q", got)
	}
}

func TestReviewerSummariesOrdering(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(80, 30)
	want := "APPROVED:alice,CHANGES_REQUESTED:dave,COMMENTED:bob,COMMENTED:carol"
	if got := badgesToString(m.reviewerSummaries()); got != want {
		t.Errorf("badges order:\n got %q\nwant %q", got, want)
	}
}

func TestReviewerSummariesEmptyReturnsNil(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	if got := m.reviewerSummaries(); got != nil {
		t.Errorf("expected nil with nil detail, got %v", got)
	}
	m.Detail = &domain.PRPreviewSnapshot{}
	if got := m.reviewerSummaries(); got != nil {
		t.Errorf("expected nil with empty detail, got %v", got)
	}
}

func TestReviewerStripEmptyReturnsEmptyString(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	if got := m.renderReviewerStrip(78); got != "" {
		t.Errorf("expected empty strip with no detail, got %q", got)
	}
	m.Detail = &domain.PRPreviewSnapshot{}
	if got := m.renderReviewerStrip(78); got != "" {
		t.Errorf("expected empty strip with empty detail, got %q", got)
	}
}

func TestReviewerStripFitsWhenSpaceAllows(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(200, 30)
	got := stripANSI(m.renderReviewerStrip(198))
	for _, want := range []string{"@alice", "@dave", "@bob", "@carol"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in wide strip, got %q", want, got)
		}
	}
	if strings.Contains(got, "+") {
		t.Errorf("expected no overflow indicator in wide strip, got %q", got)
	}
}

func TestReviewerStripOverflowIndicator(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "reviewer1", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-1 * time.Hour)},
			{Login: "reviewer2", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-2 * time.Hour)},
			{Login: "reviewer3", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-3 * time.Hour)},
			{Login: "reviewer4", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-4 * time.Hour)},
			{Login: "reviewer5", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-5 * time.Hour)},
			{Login: "reviewer6", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-6 * time.Hour)},
		},
	}
	got := stripANSI(m.renderReviewerStrip(40))
	if !strings.Contains(got, "+") {
		t.Errorf("expected overflow indicator, got %q", got)
	}
	if !strings.HasPrefix(strings.TrimSpace(strings.SplitN(got, "+", 2)[0]), "Reviewers") {
		t.Errorf("expected strip to start with label, got %q", got)
	}
}

// TestViewHeightInvariant pins that View() never renders more rows than the
// model's Height, with and without the reviewer strip and with compose open —
// the strip adds a header row, and the body must shrink to compensate.
func TestViewHeightInvariant(t *testing.T) {
	t.Parallel()
	for _, h := range []int{24, 30, 40} {
		for name, prep := range map[string]func(m *PRDetailModel){
			"no_strip": func(m *PRDetailModel) {},
			"strip":    func(m *PRDetailModel) { m.Detail = makeReviewersModel(m.Width, m.Height).Detail },
			"strip_compose": func(m *PRDetailModel) {
				m.Detail = makeReviewersModel(m.Width, m.Height).Detail
				m.compose.active = true
			},
		} {
			m := makePRDetail(100, h, nil, nil)
			prep(m)
			if got := lipgloss.Height(m.View()); got != h {
				t.Errorf("[%s h=%d] View height = %d, want %d", name, h, got, h)
			}
		}
	}
}

func TestReviewerStripNotInCommitHeader(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(80, 30)
	m.CommitMode = true
	if strings.Contains(m.renderHeader(), "Reviewers") {
		t.Error("reviewer strip must not render in commit-mode header")
	}
}

// ── Golden tests (regenerate: go test -run TestHeaderReviewers -update ./internal/ui/views/prdetail/) ──

func TestHeaderReviewersGoldenWidths(t *testing.T) {
	t.Parallel()
	for _, w := range []int{79, 80, 120} {
		m := makeReviewersModel(w, 30)
		got := stripANSI(m.renderHeader())
		checkGolden(t, got, fmt.Sprintf("header_reviewers_w%d.txt", w))
	}
}

func TestHeaderReviewersTruncatedGolden(t *testing.T) {
	t.Parallel()
	m := makePRDetail(80, 30, nil, nil)
	m.Summary = domain.PullRequestSummary{
		Number: 142,
		Title:  "Fix auth token refresh",
		State:  domain.PRStateOpen,
		Author: "utkarsh261",
		Repo:   "owner/repo",
	}
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:   "owner/repo",
		Number: 142,
		Title:  "Fix auth token refresh",
		Author: "utkarsh261",
		State:  domain.PRStateOpen,
		Reviewers: []domain.PreviewReviewer{
			{Login: "reviewer1", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-1 * time.Hour)},
			{Login: "reviewer2", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-2 * time.Hour)},
			{Login: "reviewer3", State: "CHANGES_REQUESTED", SubmittedAt: reviewerTestBase.Add(-3 * time.Hour)},
			{Login: "reviewer4", State: "COMMENTED", Body: "notes", SubmittedAt: reviewerTestBase.Add(-4 * time.Hour)},
			{Login: "reviewer5", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-5 * time.Hour)},
			{Login: "reviewer6", State: "APPROVED", SubmittedAt: reviewerTestBase.Add(-6 * time.Hour)},
			{Login: "reviewer7", State: "COMMENTED", Body: "hm", SubmittedAt: reviewerTestBase.Add(-7 * time.Hour)},
		},
	}
	got := stripANSI(m.renderHeader())
	checkGolden(t, got, "header_reviewers_truncated_w80.txt")
}

// TestHeaderReviewersNoReviewersGolden locks that a PR without reviewers or
// comments keeps the single-line header.
func TestHeaderReviewersNoReviewersGolden(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(80, 30)
	m.Detail.Reviewers = nil
	m.Detail.Comments = nil
	got := stripANSI(m.renderHeader())
	checkGolden(t, got, "header_no_reviewers_w80.txt")
}
