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

type mergePRService struct {
	checkMergeableFn func(ctx context.Context, repo domain.Repository, number int) (domain.MergeableState, error)
	mergePRFn        func(ctx context.Context, repo domain.Repository, number int, prID, headRefOID, method string) error
}

func (s *mergePRService) LoadDetail(_ context.Context, _ domain.Repository, _ int, _ bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (s *mergePRService) LoadDiff(_ context.Context, _ domain.Repository, _ int, _ string, _ bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (s *mergePRService) LoadPRCommits(_ context.Context, _ domain.Repository, _ int, _ bool) ([]domain.Commit, error) {
	return nil, nil
}
func (s *mergePRService) LoadCommitDiff(_ context.Context, _ domain.Repository, _ string, _ bool) (diffmodel.DiffModel, error) {
	return diffmodel.DiffModel{}, nil
}
func (s *mergePRService) PostComment(_ context.Context, _, _ string) error       { return nil }
func (s *mergePRService) PostReviewComment(_ context.Context, _, _ string) error { return nil }
func (s *mergePRService) ApprovePR(_ context.Context, _, _ string) error         { return nil }
func (s *mergePRService) SubmitReviewWithComments(_ context.Context, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *mergePRService) SaveDraftComments(_ context.Context, _ domain.Repository, _ int, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *mergePRService) LoadDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (s *mergePRService) DeleteDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *mergePRService) MergePR(ctx context.Context, repo domain.Repository, number int, prID, headRefOID, method string) error {
	if s.mergePRFn != nil {
		return s.mergePRFn(ctx, repo, number, prID, headRefOID, method)
	}
	return nil
}
func (s *mergePRService) CheckMergeable(ctx context.Context, repo domain.Repository, number int) (domain.MergeableState, error) {
	if s.checkMergeableFn != nil {
		return s.checkMergeableFn(ctx, repo, number)
	}
	return domain.MergeableState{}, nil
}
func (s *mergePRService) ClosePR(_ context.Context, _ domain.Repository, _ int, _ string) error  { return nil }
func (s *mergePRService) ReopenPR(_ context.Context, _ domain.Repository, _ int, _ string) error { return nil }

func newMergeModel(mergeable, mergeState string) *PRDetailModel {
	repo := testutil.Repo("acme/api")
	summary := domain.PullRequestSummary{
		ID:         "pr_123",
		Repo:       repo.FullName,
		Number:     42,
		Title:      "Feature",
		Author:     "octocat",
		HeadRefOID: "abc123",
	}
	m := NewModel(summary, repo, &mergePRService{})
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:       repo.FullName,
		Number:     42,
		State:      "OPEN",
		Mergeable:  mergeable,
		MergeState: mergeState,
	}
	m.Width = 120
	m.Height = 40
	return m
}

func TestMergeKeyNonMergeable(t *testing.T) {
	m := newMergeModel("CONFLICTING", "CONFLICTING")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.mergeErr == "" {
		t.Fatal("expected mergeErr to be set for non-mergeable PR")
	}
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected mergeStepNone, got %d", m.mergeStep)
	}
	hint := m.StatusHint()
	if hint != "PR is not mergeable (conflicting)" {
		t.Fatalf("expected hint about non-mergeable, got: %s", hint)
	}
}

func TestMergeFlowSuccess(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")

	// m -> s -> y
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.mergeStep != mergeStepSelectMethod {
		t.Fatalf("expected select method, got %d", m.mergeStep)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.mergeStep != mergeStepConfirm {
		t.Fatalf("expected confirm, got %d", m.mergeStep)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.mergeStep != mergeStepChecking {
		t.Fatalf("expected checking, got %d", m.mergeStep)
	}

	// Simulate check completion.
	m.Update(cmds.MergeableChecked{Repo: "acme/api", Number: 42, State: domain.MergeableState{Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN", HeadRefOid: "abc123"}})
	if m.mergeStep != mergeStepExecuting {
		t.Fatalf("expected executing, got %d", m.mergeStep)
	}

	// Simulate merge success.
	m.Update(cmds.MergePRMsg{Repo: "acme/api", Number: 42, Method: "SQUASH"})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected mergeStepNone after success, got %d", m.mergeStep)
	}
	if m.Detail.State != "MERGED" {
		t.Fatalf("expected optimistic MERGED state, got %s", m.Detail.State)
	}
}

func TestMergeFlowCancelAtSelect(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected mergeStepNone after cancel, got %d", m.mergeStep)
	}
}

func TestMergeFlowCancelAtConfirm(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected mergeStepNone after cancel, got %d", m.mergeStep)
	}
}

func TestMergeFlowPreCheckFails(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m.Update(cmds.MergeableChecked{Repo: "acme/api", Number: 42, State: domain.MergeableState{Mergeable: "CONFLICTING", MergeStateStatus: "CONFLICTING"}})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected reset after pre-check fail, got %d", m.mergeStep)
	}
	if m.mergeErr == "" {
		t.Fatal("expected mergeErr after pre-check fail")
	}
}

func TestMergeFlowNetworkError(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m.Update(cmds.MergeableChecked{Repo: "acme/api", Number: 42, Err: errors.New("timeout")})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected reset after network error, got %d", m.mergeStep)
	}
	if m.mergeErr == "" {
		t.Fatal("expected mergeErr after network error")
	}
}

func TestMergeFlowMergeError(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.mergeStep = mergeStepExecuting
	m.Update(cmds.MergePRMsg{Repo: "acme/api", Number: 42, Method: "SQUASH", Err: errors.New("merge rejected")})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected reset after merge error, got %d", m.mergeStep)
	}
	if m.mergeErr == "" {
		t.Fatal("expected mergeErr after merge error")
	}
}

func TestMergeEscBehavior(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	// First esc during select method resets flow.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected mergeStepNone after first esc, got %d", m.mergeStep)
	}
}

func TestMergeDraftsSurvive(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.drafts = []domain.DraftInlineComment{{Path: "main.go", Line: 10, Body: "fix this"}}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if len(m.drafts) != 1 {
		t.Fatalf("expected drafts to survive, got %d", len(m.drafts))
	}
}

func TestMergeVisualModeIgnored(t *testing.T) {
	m := newMergeModel("MERGEABLE", "CLEAN")
	m.visual.Active = true
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.mergeStep != mergeStepNone {
		t.Fatal("expected merge to be ignored while in visual mode")
	}
}

func TestHeaderShowsNonMergeableConflicting(t *testing.T) {
	m := newMergeModel("CONFLICTING", "CONFLICTING")
	header := m.renderHeader()
	if !strings.Contains(header, "conflicting") {
		t.Fatalf("expected header to show conflicting, got:\n%s", header)
	}
}

func TestHeaderShowsBlocked(t *testing.T) {
	m := newMergeModel("BLOCKED", "BLOCKED")
	header := m.renderHeader()
	if !strings.Contains(header, "blocked") {
		t.Fatalf("expected header to show blocked, got:\n%s", header)
	}
}

func TestHeaderHidesUnknownMergeable(t *testing.T) {
	m := newMergeModel("UNKNOWN", "")
	header := m.renderHeader()
	if strings.Contains(header, "unknown") {
		t.Fatalf("expected header to hide unknown mergeability, got:\n%s", header)
	}
}


