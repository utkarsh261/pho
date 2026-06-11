package prdetail

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
)

type editPRService struct {
	updatePRFn func(ctx context.Context, repo domain.Repository, number int, prID, title, body string) error
}

func (s *editPRService) LoadDetail(_ context.Context, _ domain.Repository, _ int, _ bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (s *editPRService) LoadDiff(_ context.Context, _ domain.Repository, _ int, _ string, _ bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (s *editPRService) LoadPRCommits(_ context.Context, _ domain.Repository, _ int, _ bool) ([]domain.Commit, error) {
	return nil, nil
}
func (s *editPRService) LoadCommitDiff(_ context.Context, _ domain.Repository, _ string, _ bool) (diffmodel.DiffModel, error) {
	return diffmodel.DiffModel{}, nil
}
func (s *editPRService) PostComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *editPRService) PostCommentReply(_ context.Context, _ domain.Repository, _, _, _ string) error {
	return nil
}
func (s *editPRService) PostReviewComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *editPRService) PostThreadReply(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *editPRService) ApprovePR(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *editPRService) SubmitReviewWithComments(_ context.Context, _ domain.Repository, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *editPRService) SaveDraftComments(_ context.Context, _ domain.Repository, _ int, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *editPRService) LoadDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (s *editPRService) DeleteDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *editPRService) MergePR(_ context.Context, _ domain.Repository, _ int, _, _, _ string) error {
	return nil
}
func (s *editPRService) CheckMergeable(_ context.Context, _ domain.Repository, _ int) (domain.MergeableState, error) {
	return domain.MergeableState{}, nil
}
func (s *editPRService) ClosePR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *editPRService) ReopenPR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *editPRService) UpdatePR(ctx context.Context, repo domain.Repository, number int, prID, title, body string) error {
	if s.updatePRFn != nil {
		return s.updatePRFn(ctx, repo, number, prID, title, body)
	}
	return nil
}
func (s *editPRService) CreatePR(_ context.Context, _ domain.CreatePRParams) (domain.PullRequestSummary, error) {
	return domain.PullRequestSummary{}, nil
}
func (s *editPRService) FetchRepoInfo(_ context.Context, _ domain.Repository) (domain.RepoInfo, error) {
	return domain.RepoInfo{}, nil
}

func newEditModel() *PRDetailModel {
	repo := testutil.Repo("acme/api")
	summary := domain.PullRequestSummary{
		ID:     "pr_123",
		Repo:   repo.FullName,
		Number: 42,
		Title:  "Original Title",
		Author: "octocat",
	}
	m := NewModel(summary, repo, &editPRService{})
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:        repo.FullName,
		Number:      42,
		Title:       "Original Title",
		BodyExcerpt: "Original body text.",
		State:       "OPEN",
	}
	m.Width = 120
	m.Height = 40
	return m
}

func TestEditKeyShowsPrompt(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.editPrompt == "" {
		t.Fatal("expected editPrompt to be set")
	}
	hint := m.StatusHint()
	if !strings.Contains(hint, "[t]itle") || !strings.Contains(hint, "[b]ody") {
		t.Fatalf("expected prompt hint, got %q", hint)
	}
}

func TestEditKeyBlockedDuringMerge(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.mergeStep = mergeStepSelectMethod
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.editPrompt != "" {
		t.Fatal("expected editPrompt to be empty during merge")
	}
}

func TestEditKeyBlockedDuringClose(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.closeStep = closeStepConfirm
	m.closeTarget = "CLOSE"
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.editPrompt != "" {
		t.Fatal("expected editPrompt to be empty during close")
	}
}

func TestEditPromptTCancels(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.editPrompt = "Edit: [t]itle or [b]ody?"
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.editPrompt != "" {
		t.Fatalf("expected editPrompt cleared, got %q", m.editPrompt)
	}
}

func TestEditPromptTOpensCompose(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.editPrompt = "Edit: [t]itle or [b]ody?"
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if m.editPrompt != "" {
		t.Fatal("expected editPrompt cleared")
	}
	if !m.compose.active {
		t.Fatal("expected compose active")
	}
	if m.compose.mode != composeModeEditTitle {
		t.Fatalf("expected composeModeEditTitle, got %d", m.compose.mode)
	}
	if m.compose.input.Value() != "Original Title" {
		t.Fatalf("expected pre-filled title, got %q", m.compose.input.Value())
	}
}

func TestEditPromptBOpensCompose(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.editPrompt = "Edit: [t]itle or [b]ody?"
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if m.editPrompt != "" {
		t.Fatal("expected editPrompt cleared")
	}
	if !m.compose.active {
		t.Fatal("expected compose active")
	}
	if m.compose.mode != composeModeEditBody {
		t.Fatalf("expected composeModeEditBody, got %d", m.compose.mode)
	}
	if m.compose.input.Value() != "Original body text." {
		t.Fatalf("expected pre-filled body, got %q", m.compose.input.Value())
	}
}

func TestEditTitleEmptyRejected(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditTitle, commentEntry{}, 0)
	m.compose.SetText("")
	m, _ = m.Update(submitComposeMsg{body: ""})
	if m.compose.status != composeStatusError {
		t.Fatalf("expected composeStatusError, got %d", m.compose.status)
	}
	if m.editPosting {
		t.Fatal("expected editPosting false")
	}
}

func TestEditTitleSuccessUpdatesSummary(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditTitle, commentEntry{}, 0)
	m.compose.SetText("New Title")
	m, cmd := m.Update(submitComposeMsg{body: "New Title"})
	if !m.editPosting {
		t.Fatal("expected editPosting true")
	}
	if cmd == nil {
		t.Fatal("expected command to be emitted")
	}
	msg := cmd()
	upd, ok := msg.(cmds.PRUpdated)
	if !ok {
		t.Fatalf("expected PRUpdated, got %T", msg)
	}
	if upd.Title != "New Title" {
		t.Fatalf("expected title New Title, got %q", upd.Title)
	}

	m, _ = m.Update(upd)
	if m.Summary.Title != "New Title" {
		t.Fatalf("expected Summary.Title updated, got %q", m.Summary.Title)
	}
	if m.Detail.Title != "New Title" {
		t.Fatalf("expected Detail.Title updated, got %q", m.Detail.Title)
	}
	if m.editPosting {
		t.Fatal("expected editPosting false after success")
	}
}

func TestEditBodySuccessUpdatesDetail(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditBody, commentEntry{}, 0)
	m.compose.SetText("New body text.")
	m, cmd := m.Update(submitComposeMsg{body: "New body text."})
	if !m.editPosting {
		t.Fatal("expected editPosting true")
	}
	if cmd == nil {
		t.Fatal("expected command to be emitted")
	}
	msg := cmd()
	upd, ok := msg.(cmds.PRUpdated)
	if !ok {
		t.Fatalf("expected PRUpdated, got %T", msg)
	}
	if upd.Body != "New body text." {
		t.Fatalf("expected body New body text., got %q", upd.Body)
	}

	m, _ = m.Update(upd)
	if m.Detail.BodyExcerpt != "New body text." {
		t.Fatalf("expected Detail.BodyExcerpt updated, got %q", m.Detail.BodyExcerpt)
	}
	if m.editPosting {
		t.Fatal("expected editPosting false after success")
	}
}

func TestEditFailureShowsError(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditTitle, commentEntry{}, 0)
	m.compose.SetText("New Title")
	m, _ = m.Update(submitComposeMsg{body: "New Title"})
	if !m.editPosting {
		t.Fatal("expected editPosting true")
	}

	m, _ = m.Update(cmds.PRUpdated{Repo: "acme/api", Number: 42, Title: "New Title", Err: errors.New("network error")})
	if m.editPosting {
		t.Fatal("expected editPosting false after error")
	}
	if m.editErr == "" {
		t.Fatal("expected editErr set")
	}
	if m.compose.status != composeStatusError {
		t.Fatalf("expected composeStatusError, got %d", m.compose.status)
	}
}

func TestEditPostingBlocksOtherActions(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.editPosting = true
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.editPrompt != "" {
		t.Fatal("expected editPrompt empty while posting")
	}
}

func TestEditSuccessShowsStatus(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditTitle, commentEntry{}, 0)
	m.compose.SetText("New Title")
	m, _ = m.Update(submitComposeMsg{body: "New Title"})
	m, _ = m.Update(cmds.PRUpdated{Repo: "acme/api", Number: 42, Title: "New Title"})
	if m.compose.status != composeStatusSuccess {
		t.Fatalf("expected composeStatusSuccess, got %d", m.compose.status)
	}
}

func TestEditPromptIgnoresUnrelatedKeys(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.editPrompt = "Edit: [t]itle or [b]ody?"
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.editPrompt == "" {
		t.Fatal("expected editPrompt to remain active on unrelated key")
	}
}

func TestEditSuccessAutoDismisses(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditTitle, commentEntry{}, 0)
	m.compose.SetText("New Title")
	m, _ = m.Update(submitComposeMsg{body: "New Title"})
	m, _ = m.Update(cmds.PRUpdated{Repo: "acme/api", Number: 42, Title: "New Title"})
	if m.compose.status != composeStatusSuccess {
		t.Fatalf("expected composeStatusSuccess, got %d", m.compose.status)
	}
	// Simulate the timer firing.
	m, _ = m.Update(composeSuccessDismissMsg{})
	if m.compose.active {
		t.Fatal("expected compose closed after dismiss")
	}
}

func TestEditBodySuccessAutoDismisses(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	m.compose.Open(composeModeEditBody, commentEntry{}, 0)
	m.compose.SetText("New body text.")
	m, _ = m.Update(submitComposeMsg{body: "New body text."})
	m, _ = m.Update(cmds.PRUpdated{Repo: "acme/api", Number: 42, Body: "New body text."})
	if m.compose.status != composeStatusSuccess {
		t.Fatalf("expected composeStatusSuccess, got %d", m.compose.status)
	}
	// Simulate the timer firing.
	m, _ = m.Update(composeSuccessDismissMsg{})
	if m.compose.active {
		t.Fatal("expected compose closed after dismiss")
	}
	if m.postedComment {
		t.Fatal("expected postedComment false for edit dismiss")
	}
}

func TestPostedCommentTarget_RemembersThreadID(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "thread1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "bob", Body: "early", CreatedAt: mustTime("2024-01-01T00:00:00Z")},
				},
			},
		},
	}
	m.switchTab(TabComments)
	m = pressKey(m, "j")
	if m.commentCursor != 0 {
		t.Fatalf("expected commentCursor=0, got %d", m.commentCursor)
	}
	entries := m.commentEntries()
	if len(entries) == 0 || entries[0].threadID != "thread1" {
		t.Fatalf("expected first entry to be thread1, got %+v", entries)
	}
	m = pressKey(m, "r")
	if !m.compose.active {
		t.Fatal("expected compose active after r")
	}
	if m.compose.target.threadID != "thread1" {
		t.Fatalf("expected target threadID=thread1, got %q", m.compose.target.threadID)
	}
	m.compose.SetText("reply body")
	m, _ = m.Update(submitComposeMsg{body: "reply body"})
	m, _ = m.Update(cmds.PRDetailLoaded{Detail: *m.Detail})
	m, _ = m.Update(composeSuccessDismissMsg{})
	if !m.postedComment {
		t.Fatal("expected postedComment true after dismiss")
	}
	if m.postedCommentTarget.threadID != "thread1" {
		t.Errorf("expected postedCommentTarget.threadID=thread1, got %q", m.postedCommentTarget.threadID)
	}
}

func TestPostedCommentScrollsToThread(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	t1 := mustTime("2024-01-01T09:00:00Z")
	t2 := mustTime("2024-01-02T09:00:00Z")
	t3 := mustTime("2024-01-03T09:00:00Z")
	m.Detail = &domain.PRPreviewSnapshot{
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", Body: "LGTM", SubmittedAt: t1},
		},
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "recent comment", CreatedAt: t3},
		},
		ReviewThreads: []domain.PreviewReviewThread{
			{
				ID: "thread1", Path: "a.go", Line: 1,
				Comments: []domain.PreviewThreadComment{
					{ID: "c1", Login: "carol", Body: "thread root", CreatedAt: t2},
				},
			},
		},
	}
	entries := m.commentEntries()
	threadIdx := -1
	for i, e := range entries {
		if e.threadID == "thread1" {
			threadIdx = i
			break
		}
	}
	if threadIdx < 0 {
		t.Fatal("thread1 not found")
	}

	m.postedComment = true
	m.postedCommentTarget = commentEntry{threadID: "thread1"}
	m, _ = m.Update(cmds.PRDetailLoaded{Detail: *m.Detail})

	if m.commentCursor != threadIdx {
		t.Errorf("expected commentCursor=%d (thread), got %d", threadIdx, m.commentCursor)
	}
	if m.postedComment {
		t.Error("expected postedComment to be reset after PRDetailLoaded")
	}
}

func TestPostedCommentScrollsToLastWhenNoTarget(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "hello", CreatedAt: mustTime("2024-01-01T00:00:00Z")},
		},
	}
	m.switchTab(TabComments)
	m.postedComment = true
	m.postedCommentTarget = commentEntry{}
	m, _ = m.Update(cmds.PRDetailLoaded{Detail: *m.Detail})
	entries := m.commentEntries()
	if m.commentCursor != len(entries)-1 {
		t.Errorf("expected commentCursor=%d (last), got %d", len(entries)-1, m.commentCursor)
	}
}

func TestPostedCommentTarget_RemembersCommentID(t *testing.T) {
	t.Parallel()
	m := makeInlineReviewModel(100, 40)
	m.PRService = &prServiceStub{}
	m.Detail = &domain.PRPreviewSnapshot{
		Comments: []domain.PreviewComment{
			{ID: "pc1", Login: "bob", Body: "hello", CreatedAt: mustTime("2024-01-01T00:00:00Z")},
		},
	}
	m.switchTab(TabComments)
	m = pressKey(m, "j")
	m = pressKey(m, "r")
	if !m.compose.active {
		t.Fatal("expected compose active after r")
	}
	m.compose.SetText("reply body")
	m, _ = m.Update(submitComposeMsg{body: "reply body"})
	m, _ = m.Update(cmds.PRDetailLoaded{Detail: *m.Detail})
	m, _ = m.Update(composeSuccessDismissMsg{})
	if !m.postedComment {
		t.Fatal("expected postedComment true after dismiss")
	}
	if m.postedCommentTarget.commentID != "pc1" {
		t.Errorf("expected postedCommentTarget.commentID=pc1, got %q", m.postedCommentTarget.commentID)
	}
}
