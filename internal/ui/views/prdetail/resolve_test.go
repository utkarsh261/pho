package prdetail

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

type resolvePRService struct {
	resolveFn   func(ctx context.Context, repo domain.Repository, number int, threadID string) error
	unresolveFn func(ctx context.Context, repo domain.Repository, number int, threadID string) error
}

func (s *resolvePRService) LoadDetail(_ context.Context, _ domain.Repository, _ int, _ bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (s *resolvePRService) LoadDiff(_ context.Context, _ domain.Repository, _ int, _ string, _ bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (s *resolvePRService) LoadPRCommits(_ context.Context, _ domain.Repository, _ int, _ bool) ([]domain.Commit, error) {
	return nil, nil
}
func (s *resolvePRService) LoadCommitDiff(_ context.Context, _ domain.Repository, _ string, _ bool) (diffmodel.DiffModel, error) {
	return diffmodel.DiffModel{}, nil
}
func (s *resolvePRService) PostComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *resolvePRService) PostCommentReply(_ context.Context, _ domain.Repository, _, _, _ string) error {
	return nil
}
func (s *resolvePRService) PostReviewComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *resolvePRService) PostThreadReply(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *resolvePRService) ApprovePR(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *resolvePRService) SubmitReviewWithComments(_ context.Context, _ domain.Repository, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *resolvePRService) SaveDraftComments(_ context.Context, _ domain.Repository, _ int, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *resolvePRService) LoadDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (s *resolvePRService) DeleteDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *resolvePRService) MergePR(_ context.Context, _ domain.Repository, _ int, _, _, _ string) error {
	return nil
}
func (s *resolvePRService) CheckMergeable(_ context.Context, _ domain.Repository, _ int) (domain.MergeableState, error) {
	return domain.MergeableState{}, nil
}
func (s *resolvePRService) ClosePR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *resolvePRService) ReopenPR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *resolvePRService) ResolveThread(_ context.Context, _ domain.Repository, _ int, threadID string) error {
	if s.resolveFn != nil {
		return s.resolveFn(context.Background(), domain.Repository{}, 0, threadID)
	}
	return nil
}
func (s *resolvePRService) UnresolveThread(_ context.Context, _ domain.Repository, _ int, threadID string) error {
	if s.unresolveFn != nil {
		return s.unresolveFn(context.Background(), domain.Repository{}, 0, threadID)
	}
	return nil
}
func (s *resolvePRService) UpdatePR(_ context.Context, _ domain.Repository, _ int, _, _, _ string) error {
	return nil
}
func (s *resolvePRService) CreatePR(_ context.Context, _ domain.CreatePRParams) (domain.PullRequestSummary, error) {
	return domain.PullRequestSummary{}, nil
}
func (s *resolvePRService) FetchRepoInfo(_ context.Context, _ domain.Repository) (domain.RepoInfo, error) {
	return domain.RepoInfo{}, nil
}

func makeResolveModel(threads ...domain.PreviewReviewThread) *PRDetailModel {
	repo := domain.Repository{FullName: "acme/api", Host: "github.com"}
	summary := domain.PullRequestSummary{
		ID:         "pr_123",
		Repo:       repo.FullName,
		Number:     42,
		Title:      "Feature",
		Author:     "octocat",
		HeadRefOID: "abc123",
	}
	m := NewModel(summary, repo, &resolvePRService{})
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:          repo.FullName,
		Number:        42,
		State:         "OPEN",
		Mergeable:     "MERGEABLE",
		ReviewThreads: threads,
	}
	m.ViewerLogin = "octocat"
	m.SetTheme(theme.Default())
	m.Width = 120
	m.Height = 40
	m.activeTab = TabComments
	m.leftPanel.Focus = FocusContent
	return m
}

func makeUnresolvedThread(id, path string, line int, comments ...domain.PreviewThreadComment) domain.PreviewReviewThread {
	return domain.PreviewReviewThread{
		ID:       id,
		Path:     path,
		Line:     line,
		Comments: comments,
	}
}

func makeResolvedThread(id, path string, line int, resolver string, comments ...domain.PreviewThreadComment) domain.PreviewReviewThread {
	return domain.PreviewReviewThread{
		ID:         id,
		Path:       path,
		Line:       line,
		IsResolved: true,
		ResolvedBy: resolver,
		Comments:   comments,
	}
}

func threadComment(id, login, body string, ts time.Time) domain.PreviewThreadComment {
	return domain.PreviewThreadComment{ID: id, Login: login, Body: body, CreatedAt: ts}
}

func pressM(m *PRDetailModel) (*PRDetailModel, tea.Cmd) {
	return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
}

func pressEnter(m *PRDetailModel) (*PRDetailModel, tea.Cmd) {
	return m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
}

func pressBracket(m *PRDetailModel, forward bool) (*PRDetailModel, tea.Cmd) {
	r := '['
	if forward {
		r = ']'
	}
	return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

// ── m resolve/unresolve toggle ──────────────────────────────────────────────

func TestM_ResolveUnresolvedThread(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.commentCursor = 0 // cursor on the thread entry

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if !thread.IsResolved {
		t.Error("expected IsResolved=true after m")
	}
	if thread.ResolvedBy != "octocat" {
		t.Errorf("expected ResolvedBy=octocat, got %q", thread.ResolvedBy)
	}
	if !m.pendingToggle.active() {
		t.Error("expected pendingToggle to be active")
	}
	if !m.pendingToggle.TargetResolved {
		t.Error("expected TargetResolved=true")
	}
}

func TestM_UnresolveResolvedThread(t *testing.T) {
	thread := makeResolvedThread("t1", "a.go", 5, "bob",
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	// Resolved + collapsed → cursor on the summary entry.
	m.commentCursor = 0

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected IsResolved=false after unresolve")
	}
	if thread.ResolvedBy != "" {
		t.Errorf("expected ResolvedBy empty, got %q", thread.ResolvedBy)
	}
	if !m.pendingToggle.active() {
		t.Error("expected pendingToggle to be active")
	}
	if m.pendingToggle.TargetResolved {
		t.Error("expected TargetResolved=false")
	}
}

func TestM_OnReplyEntryResolvesWholeThread(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()),
		threadComment("c2", "bob", "yes", time.Now().Add(time.Hour)))
	m := makeResolveModel(thread)
	// Cursor on the reply entry (index 1 in the thread).
	m.commentCursor = 1

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if !thread.IsResolved {
		t.Error("expected whole thread resolved from reply entry")
	}
}

func TestM_OnDraft_NoOp(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.drafts = []domain.DraftInlineComment{
		{ID: "d1", Path: "a.go", Line: 10, Body: "draft comment", CreatedAt: time.Now()},
	}
	m.commentEntriesDirty = true
	m.commentCursor = 0 // cursor on the draft entry

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected no resolve on draft entry")
	}
	if m.pendingToggle.active() {
		t.Error("expected no pendingToggle on draft entry")
	}
}

func TestM_OnReviewSummary_NoOp(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.Detail.Reviewers = []domain.PreviewReviewer{
		{Login: "alice", State: "APPROVED", Body: "LGTM", SubmittedAt: time.Now()},
	}
	m.commentEntriesDirty = true
	// Cursor on the review summary entry (index 0, before threads).
	m.commentCursor = 0

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected no resolve on review summary entry")
	}
}

func TestM_OnPRComment_NoOp(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", later))
	m := makeResolveModel(thread)
	m.Detail.Comments = []domain.PreviewComment{
		{ID: "pc1", Login: "carol", Body: "looks good", CreatedAt: earlier},
	}
	m.commentEntriesDirty = true
	// Cursor on the PR-level comment (index 0, sorts before thread).
	m.commentCursor = 0

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected no resolve on PR-level comment")
	}
}

func TestM_WithNoCursor_NoOp(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.commentCursor = -1

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected no resolve with no cursor")
	}
}

func TestM_WhileMergeInProgress_Blocked(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.commentCursor = 0
	m.mergeStep = mergeStepSelectMethod // simulate merge in progress

	m, _ = pressM(m)

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected no resolve while merge in progress")
	}
}

func TestM_WhileToggleInProgress_Blocked(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.commentCursor = 0
	m.pendingToggle = pendingToggleState{ThreadID: "t1"}

	m, _ = pressM(m)

	// Should not re-trigger; pendingToggle should still be the original.
	if !m.pendingToggle.active() {
		t.Error("expected original pendingToggle to remain")
	}
}

// ── Merge remap: m does NOT start merge, M does ─────────────────────────────

func TestM_DoesNotStartMerge(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.activeTab = TabComments
	m.leftPanel.Focus = FocusContent
	m.commentCursor = -1 // no cursor → m is a no-op, not merge

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if m.mergeStep != mergeStepNone {
		t.Errorf("expected mergeStepNone, got %d", m.mergeStep)
	}
}

func TestShiftM_StartsMerge(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})

	if m.mergeStep != mergeStepSelectMethod {
		t.Errorf("expected mergeStepSelectMethod, got %d", m.mergeStep)
	}
}

// ── Enter expand ─────────────────────────────────────────────────────────────

func TestEnter_OnCollapsedResolved_Expands(t *testing.T) {
	thread := makeResolvedThread("t1", "a.go", 5, "bob",
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.commentEntriesDirty = true
	entries := m.commentEntries()
	// Find the summary entry.
	summaryIdx := -1
	for i, e := range entries {
		if e.isResolvedSummary {
			summaryIdx = i
			break
		}
	}
	if summaryIdx < 0 {
		t.Fatal("expected a resolved summary entry")
	}
	m.commentCursor = summaryIdx

	m, _ = pressEnter(m)

	if !m.expandedResolved["t1"] {
		t.Error("expected t1 in expandedResolved after Enter")
	}
	entries = m.commentEntries()
	// After expand, cursor should be on the first comment, not a summary.
	if m.commentCursor >= len(entries) {
		t.Fatal("cursor out of range")
	}
	if entries[m.commentCursor].isResolvedSummary {
		t.Error("expected cursor on a real comment, not summary")
	}
	if entries[m.commentCursor].threadID != "t1" {
		t.Errorf("expected cursor on t1 entry, got threadID %q", entries[m.commentCursor].threadID)
	}
}

func TestEnter_OnUnresolved_JumpsToCode(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.commentEntriesDirty = true
	m.commentCursor = 0

	// Enter on an unresolved thread should not expand (no resolved summary).
	// It calls jumpToCommentCode which is safe to call (no-op without diff).
	m, _ = pressEnter(m)

	// No expansion should occur.
	if m.expandedResolved["t1"] {
		t.Error("expected no expansion for unresolved thread")
	}
}

// ── expandedResolved lifecycle ───────────────────────────────────────────────

func TestResolve_RemovesFromExpandedResolved(t *testing.T) {
	// Unresolved thread with a stale expandedResolved entry (e.g. from a
	// previous resolve→expand→unresolve cycle). Resolving should clear it.
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.expandedResolved = map[string]bool{"t1": true} // stale entry
	m.commentEntriesDirty = true
	m.commentCursor = 0

	m, _ = pressM(m) // resolve

	if m.expandedResolved["t1"] {
		t.Error("expected t1 removed from expandedResolved after resolve")
	}
}

func TestUnresolve_IgnoresExpandedResolved(t *testing.T) {
	thread := makeResolvedThread("t1", "a.go", 5, "bob",
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	// Thread is resolved + collapsed (not in expandedResolved).
	m.commentEntriesDirty = true
	// Cursor on the summary.
	entries := m.commentEntries()
	for i, e := range entries {
		if e.isResolvedSummary {
			m.commentCursor = i
			break
		}
	}

	m, _ = pressM(m) // unresolve

	// Map should be untouched (unresolved threads ignore it).
	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected thread unresolved")
	}
}

func TestRefresh_PreservesExpandedResolved(t *testing.T) {
	thread := makeResolvedThread("t1", "a.go", 5, "bob",
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.expandedResolved = map[string]bool{"t1": true}

	// Simulate a PRDetailLoaded (manual R).
	m.Update(cmds.PRDetailLoaded{
		Detail: domain.PRPreviewSnapshot{
			ReviewThreads: []domain.PreviewReviewThread{
				makeResolvedThread("t1", "a.go", 5, "bob",
					threadComment("c1", "alice", "should we rename?", time.Now())),
			},
		},
	})

	if !m.expandedResolved["t1"] {
		t.Error("expected expandedResolved preserved across PRDetailLoaded")
	}
}

// ── [/] jump unresolved ──────────────────────────────────────────────────────

func TestBracketNext_JumpsToNextUnresolved(t *testing.T) {
	t1 := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	t2 := makeResolvedThread("t2", "b.go", 10, "bob", threadComment("c2", "bob", "second", time.Now().Add(time.Hour)))
	t3 := makeUnresolvedThread("t3", "c.go", 15, threadComment("c3", "carol", "third", time.Now().Add(2*time.Hour)))
	m := makeResolveModel(t1, t2, t3)
	m.commentEntriesDirty = true
	entries := m.commentEntries()
	// Find first unresolved thread-start.
	startIdx := -1
	for i, e := range entries {
		if e.isThreadStart && e.threadID == "t1" {
			startIdx = i
			break
		}
	}
	m.commentCursor = startIdx

	m, _ = pressBracket(m, true)

	entries = m.commentEntries()
	if m.commentCursor >= len(entries) {
		t.Fatal("cursor out of range")
	}
	if entries[m.commentCursor].threadID != "t3" {
		t.Errorf("expected cursor on t3, got threadID %q", entries[m.commentCursor].threadID)
	}
}

func TestBracketPrev_JumpsToPrevUnresolved(t *testing.T) {
	t1 := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	t2 := makeUnresolvedThread("t2", "c.go", 15, threadComment("c3", "carol", "third", time.Now().Add(2*time.Hour)))
	m := makeResolveModel(t1, t2)
	m.commentEntriesDirty = true
	entries := m.commentEntries()
	// Put cursor on t2's start.
	t2Idx := -1
	for i, e := range entries {
		if e.isThreadStart && e.threadID == "t2" {
			t2Idx = i
			break
		}
	}
	m.commentCursor = t2Idx

	m, _ = pressBracket(m, false)

	entries = m.commentEntries()
	if entries[m.commentCursor].threadID != "t1" {
		t.Errorf("expected cursor on t1, got threadID %q", entries[m.commentCursor].threadID)
	}
}

func TestBracket_WithZeroUnresolved_NoOp(t *testing.T) {
	t1 := makeResolvedThread("t1", "a.go", 5, "bob", threadComment("c1", "alice", "first", time.Now()))
	m := makeResolveModel(t1)
	m.commentEntriesDirty = true
	m.commentCursor = 0

	originalCursor := m.commentCursor
	m, _ = pressBracket(m, true)

	if m.commentCursor != originalCursor {
		t.Errorf("expected cursor unchanged, got %d (was %d)", m.commentCursor, originalCursor)
	}
}

func TestBracket_SkipsResolvedSummary(t *testing.T) {
	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	t1 := makeResolvedThread("t1", "a.go", 5, "bob", threadComment("c1", "alice", "first", earlier))
	t2 := makeUnresolvedThread("t2", "b.go", 10, threadComment("c2", "carol", "second", later))
	m := makeResolveModel(t1, t2)
	m.commentEntriesDirty = true
	entries := m.commentEntries()
	// Cursor on the resolved summary (t1).
	summaryIdx := -1
	for i, e := range entries {
		if e.isResolvedSummary && e.threadID == "t1" {
			summaryIdx = i
			break
		}
	}
	if summaryIdx < 0 {
		t.Fatal("expected resolved summary entry")
	}
	m.commentCursor = summaryIdx

	m, _ = pressBracket(m, true)

	entries = m.commentEntries()
	if entries[m.commentCursor].threadID != "t2" {
		t.Errorf("expected cursor on t2 (skipped resolved), got %q", entries[m.commentCursor].threadID)
	}
}

// ── ThreadResolved/Unresolved/Failed handlers ───────────────────────────────

func TestThreadResolvedMsg_ClearsPending(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	m := makeResolveModel(thread)
	m.commentCursor = 0
	m, _ = pressM(m)

	if !m.pendingToggle.active() {
		t.Fatal("expected pendingToggle active after m")
	}

	m.Update(cmds.ThreadResolvedMsg{ThreadID: "t1"})

	if m.pendingToggle.active() {
		t.Error("expected pendingToggle cleared after ThreadResolvedMsg")
	}
}

func TestThreadResolveFailed_RevertsState(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	m := makeResolveModel(thread)
	m.commentCursor = 0
	m, _ = pressM(m)

	// Thread should be optimistically resolved.
	if !m.Detail.ReviewThreads[0].IsResolved {
		t.Fatal("expected optimistic resolve")
	}

	m.Update(cmds.ThreadResolveFailed{ThreadID: "t1", Err: errors.New("network error")})

	thread = m.Detail.ReviewThreads[0]
	if thread.IsResolved {
		t.Error("expected IsResolved reverted to false after failure")
	}
	if m.resolveErr == "" {
		t.Error("expected resolveErr set after failure")
	}
	if m.pendingToggle.active() {
		t.Error("expected pendingToggle cleared after failure")
	}
}

// ── PRDetailLoaded re-anchor + mitigation ───────────────────────────────────

func TestPRDetailLoaded_ReanchorsCursor(t *testing.T) {
	t1 := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	t2 := makeUnresolvedThread("t2", "b.go", 10, threadComment("c2", "carol", "second", time.Now().Add(time.Hour)))
	m := makeResolveModel(t1, t2)
	m.commentCursor = 0
	m, _ = pressM(m) // resolve t1

	// Simulate reload returning the thread as resolved.
	m.Update(cmds.PRDetailLoaded{
		Detail: domain.PRPreviewSnapshot{
			State: "OPEN",
			ReviewThreads: []domain.PreviewReviewThread{
				makeResolvedThread("t1", "a.go", 5, "octocat",
					threadComment("c1", "alice", "first", time.Now())),
				t2,
			},
		},
	})

	// Cursor should be re-anchored to t1 (now a collapsed summary).
	entries := m.commentEntries()
	if m.commentCursor >= len(entries) {
		t.Fatal("cursor out of range")
	}
	if entries[m.commentCursor].threadID != "t1" {
		t.Errorf("expected cursor re-anchored to t1, got %q", entries[m.commentCursor].threadID)
	}
}

func TestPRDetailLoaded_ForcesOptimisticValue(t *testing.T) {
	t1 := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	m := makeResolveModel(t1)
	m.commentCursor = 0
	m, _ = pressM(m) // optimistic resolve

	// Simulate stale reload returning isResolved=false (replication lag).
	m.Update(cmds.PRDetailLoaded{
		Detail: domain.PRPreviewSnapshot{
			State: "OPEN",
			ReviewThreads: []domain.PreviewReviewThread{
				// Stale: still unresolved.
				makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now())),
			},
		},
	})

	// Mitigation should force the optimistic value.
	th := m.Detail.ReviewThreads[0]
	if !th.IsResolved {
		t.Error("expected mitigation to force IsResolved=true over stale payload")
	}
	if th.ResolvedBy != "octocat" {
		t.Errorf("expected mitigation to force ResolvedBy=octocat, got %q", th.ResolvedBy)
	}
}

// ── CommentPosted auto-expand resolved thread ───────────────────────────────

func TestCommentPosted_AutoExpandsResolvedThread(t *testing.T) {
	thread := makeResolvedThread("t1", "a.go", 5, "bob",
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.compose.Open(composeModeReply, commentEntry{threadID: "t1", path: "a.go", line: 5}, 0)

	m.Update(cmds.CommentPosted{})

	if !m.expandedResolved["t1"] {
		t.Error("expected t1 auto-expanded after CommentPosted")
	}
}

func TestCommentPosted_DoesNotExpandUnresolvedThread(t *testing.T) {
	thread := makeUnresolvedThread("t1", "a.go", 5,
		threadComment("c1", "alice", "should we rename?", time.Now()))
	m := makeResolveModel(thread)
	m.compose.Open(composeModeReply, commentEntry{threadID: "t1", path: "a.go", line: 5}, 0)

	m.Update(cmds.CommentPosted{})

	if m.expandedResolved["t1"] {
		t.Error("expected t1 NOT expanded for unresolved thread")
	}
}

// ── Unresolved count ────────────────────────────────────────────────────────

func TestUnresolvedThreadCount(t *testing.T) {
	t1 := makeUnresolvedThread("t1", "a.go", 5, threadComment("c1", "alice", "first", time.Now()))
	t2 := makeResolvedThread("t2", "b.go", 10, "bob", threadComment("c2", "bob", "second", time.Now().Add(time.Hour)))
	t3 := makeUnresolvedThread("t3", "c.go", 15, threadComment("c3", "carol", "third", time.Now().Add(2*time.Hour)))
	m := makeResolveModel(t1, t2, t3)

	if count := m.unresolvedThreadCount(); count != 2 {
		t.Errorf("expected 2 unresolved, got %d", count)
	}
}

func TestUnresolvedThreadCount_Zero(t *testing.T) {
	t1 := makeResolvedThread("t1", "a.go", 5, "bob", threadComment("c1", "alice", "first", time.Now()))
	m := makeResolveModel(t1)

	if count := m.unresolvedThreadCount(); count != 0 {
		t.Errorf("expected 0 unresolved, got %d", count)
	}
}
