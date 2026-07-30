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
)

func TestCloseKeyStartsConfirmForOpenPR(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if m.closeStep != closeStepConfirm {
		t.Fatalf("expected closeStepConfirm, got %d", m.closeStep)
	}
	if m.closeTarget != "CLOSE" {
		t.Fatalf("expected closeTarget CLOSE, got %q", m.closeTarget)
	}
}

func TestCloseKeyStartsConfirmForClosedPR(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateClosed)

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if m.closeStep != closeStepConfirm {
		t.Fatalf("expected closeStepConfirm, got %d", m.closeStep)
	}
	if m.closeTarget != "REOPEN" {
		t.Fatalf("expected closeTarget REOPEN, got %q", m.closeTarget)
	}
}

func TestCloseKeyShowsErrorForMergedPR(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateMerged)

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("expected a no-op command for merged PR")
	}
	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg from no-op command, got %T", msg)
	}
	if m.closeStep != closeStepNone {
		t.Fatalf("expected closeStepNone, got %d", m.closeStep)
	}
	if m.closeErr == "" {
		t.Fatal("expected error for merged PR")
	}
	hint := m.StatusHint()
	if !strings.Contains(hint, "cannot be closed") {
		t.Fatalf("expected error in status hint, got %q", hint)
	}
}

func TestCloseConfirmYFiresCloseCmd(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepConfirm
	m.closeTarget = "CLOSE"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected y to fire close command")
	}
	msg := cmd()
	stateMsg, ok := msg.(cmds.PRStateChangedMsg)
	if !ok {
		t.Fatalf("expected PRStateChangedMsg, got %T", msg)
	}
	if stateMsg.NewState != domain.PRStateClosed {
		t.Fatalf("expected NewState CLOSED, got %q", stateMsg.NewState)
	}
	if m.closeStep != closeStepExecuting {
		t.Fatalf("expected closeStepExecuting, got %d", m.closeStep)
	}
}

func TestCloseConfirmYFiresReopenCmd(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateClosed)
	m.closeStep = closeStepConfirm
	m.closeTarget = "REOPEN"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected y to fire reopen command")
	}
	msg := cmd()
	stateMsg, ok := msg.(cmds.PRStateChangedMsg)
	if !ok {
		t.Fatalf("expected PRStateChangedMsg, got %T", msg)
	}
	if stateMsg.NewState != domain.PRStateOpen {
		t.Fatalf("expected NewState OPEN, got %q", stateMsg.NewState)
	}
}

func TestCloseConfirmNCancels(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepConfirm
	m.closeTarget = "CLOSE"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("expected n to return a no-op command")
	}
	cmd()

	if m.closeStep != closeStepNone {
		t.Fatalf("expected closeStepNone after cancel, got %d", m.closeStep)
	}
}

func TestCloseConfirmEscCancels(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepConfirm
	m.closeTarget = "CLOSE"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected esc to return a no-op command")
	}
	cmd()

	if m.closeStep != closeStepNone {
		t.Fatalf("expected closeStepNone after cancel, got %d", m.closeStep)
	}
}

func TestCloseSuccessUpdatesState(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepExecuting
	m.closeTarget = "CLOSE"

	m, _ = m.Update(cmds.PRStateChangedMsg{Repo: "org/repo", Number: 42, NewState: domain.PRStateClosed})

	if m.closeStep != closeStepNone {
		t.Fatalf("expected closeStepNone after success, got %d", m.closeStep)
	}
	if m.Detail == nil || m.Detail.State != domain.PRStateClosed {
		t.Fatalf("expected Detail.State CLOSED, got %v", m.Detail)
	}
}

func TestReopenSuccessUpdatesState(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateClosed)
	m.closeStep = closeStepExecuting
	m.closeTarget = "REOPEN"

	m, _ = m.Update(cmds.PRStateChangedMsg{Repo: "org/repo", Number: 42, NewState: domain.PRStateOpen})

	if m.closeStep != closeStepNone {
		t.Fatalf("expected closeStepNone after success, got %d", m.closeStep)
	}
	if m.Detail == nil || m.Detail.State != domain.PRStateOpen {
		t.Fatalf("expected Detail.State OPEN, got %v", m.Detail)
	}
}

func TestCloseFailureShowsError(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepExecuting
	m.closeTarget = "CLOSE"

	m, _ = m.Update(cmds.PRStateChangedMsg{Repo: "org/repo", Number: 42, NewState: domain.PRStateClosed, Err: errors.New("network error")})

	if m.closeStep != closeStepNone {
		t.Fatalf("expected closeStepNone after failure, got %d", m.closeStep)
	}
	if m.closeErr == "" {
		t.Fatal("expected closeErr after failure")
	}
	hint := m.StatusHint()
	if !strings.Contains(hint, "Failed") {
		t.Fatalf("expected error in status hint, got %q", hint)
	}
}

func TestCloseStatusHintConfirm(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepConfirm
	m.closeTarget = "CLOSE"

	hint := m.StatusHint()
	if !strings.Contains(hint, "Close #42?") {
		t.Fatalf("expected close confirm hint, got %q", hint)
	}
}

func TestReopenStatusHintConfirm(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateClosed)
	m.closeStep = closeStepConfirm
	m.closeTarget = "REOPEN"

	hint := m.StatusHint()
	if !strings.Contains(hint, "Reopen #42?") {
		t.Fatalf("expected reopen confirm hint, got %q", hint)
	}
}

func TestCloseStatusHintExecuting(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeStep = closeStepExecuting
	m.closeTarget = "CLOSE"

	hint := m.StatusHint()
	if !strings.Contains(hint, "Closing...") {
		t.Fatalf("expected closing hint, got %q", hint)
	}
}

func TestReopenStatusHintExecuting(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateClosed)
	m.closeStep = closeStepExecuting
	m.closeTarget = "REOPEN"

	hint := m.StatusHint()
	if !strings.Contains(hint, "Reopening...") {
		t.Fatalf("expected reopening hint, got %q", hint)
	}
}

func TestCloseErrorClearsOnAnyKey(t *testing.T) {
	t.Parallel()
	m := makeCloseReopenModel(domain.PRStateOpen)
	m.closeErr = "some error"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.closeErr != "" {
		t.Fatalf("expected closeErr cleared, got %q", m.closeErr)
	}
	_ = cmd
}

type closeReopenPRService struct{}

func (s *closeReopenPRService) LoadDetail(_ context.Context, _ domain.Repository, _ int, _ bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (s *closeReopenPRService) LoadDiff(_ context.Context, _ domain.Repository, _ int, _ string, _ bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (s *closeReopenPRService) LoadPRCommits(_ context.Context, _ domain.Repository, _ int, _ bool) ([]domain.Commit, error) {
	return nil, nil
}
func (s *closeReopenPRService) LoadCommitDiff(_ context.Context, _ domain.Repository, _ string, _ bool) (diffmodel.DiffModel, error) {
	return diffmodel.DiffModel{}, nil
}
func (s *closeReopenPRService) PostComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *closeReopenPRService) PostCommentReply(_ context.Context, _ domain.Repository, _, _, _ string) error {
	return nil
}
func (s *closeReopenPRService) PostReviewComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *closeReopenPRService) PostThreadReply(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *closeReopenPRService) ApprovePR(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *closeReopenPRService) SubmitReviewWithComments(_ context.Context, _ domain.Repository, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *closeReopenPRService) SaveDraftComments(_ context.Context, _ domain.Repository, _ int, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *closeReopenPRService) LoadDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (s *closeReopenPRService) DeleteDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *closeReopenPRService) MergePR(_ context.Context, _ domain.Repository, _ int, _, _, _ string) error {
	return nil
}
func (s *closeReopenPRService) CheckMergeable(_ context.Context, _ domain.Repository, _ int) (domain.MergeableState, error) {
	return domain.MergeableState{}, nil
}
func (s *closeReopenPRService) ClosePR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *closeReopenPRService) ReopenPR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *closeReopenPRService) UpdateBranch(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}

func (s *closeReopenPRService) UpdatePR(_ context.Context, _ domain.Repository, _ int, _ string, _ string, _ string) error {
	return nil
}
func (s *closeReopenPRService) CreatePR(_ context.Context, _ domain.CreatePRParams) (domain.PullRequestSummary, error) {
	return domain.PullRequestSummary{}, nil
}
func (s *closeReopenPRService) FetchRepoInfo(_ context.Context, _ domain.Repository) (domain.RepoInfo, error) {
	return domain.RepoInfo{}, nil
}
func (s *closeReopenPRService) ResolveThread(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *closeReopenPRService) UnresolveThread(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}

func makeCloseReopenModel(state domain.PRState) *PRDetailModel {
	m := NewModel(domain.PullRequestSummary{
		Repo:   "org/repo",
		Number: 42,
		ID:     "pr-id-42",
		State:  state,
	}, domain.Repository{Host: "github.com", FullName: "org/repo"}, &closeReopenPRService{})
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:   "org/repo",
		Number: 42,
		State:  state,
	}
	m.Width = 100
	m.Height = 40
	return m
}
