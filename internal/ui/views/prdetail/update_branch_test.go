package prdetail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// updateBranchPRService is a cmds.PRService fake for the update-branch flow.
// It mirrors mergePRService's shape so the test file is self-contained.
type updateBranchPRService struct {
	updateBranchFn      func(ctx context.Context, repo domain.Repository, number int, expectedHeadSHA string) error
	updateBranchCalls   int
	capturedExpectedSHA string
}

func (s *updateBranchPRService) LoadDetail(_ context.Context, _ domain.Repository, _ int, _ bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (s *updateBranchPRService) LoadDiff(_ context.Context, _ domain.Repository, _ int, _ string, _ bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (s *updateBranchPRService) LoadPRCommits(_ context.Context, _ domain.Repository, _ int, _ bool) ([]domain.Commit, error) {
	return nil, nil
}
func (s *updateBranchPRService) LoadCommitDiff(_ context.Context, _ domain.Repository, _ string, _ bool) (diffmodel.DiffModel, error) {
	return diffmodel.DiffModel{}, nil
}
func (s *updateBranchPRService) PostComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) PostCommentReply(_ context.Context, _ domain.Repository, _, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) PostReviewComment(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) PostThreadReply(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) ApprovePR(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) SubmitReviewWithComments(_ context.Context, _ domain.Repository, _, _, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *updateBranchPRService) SaveDraftComments(_ context.Context, _ domain.Repository, _ int, _ string, _ []domain.DraftInlineComment) error {
	return nil
}
func (s *updateBranchPRService) LoadDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (s *updateBranchPRService) DeleteDraftComments(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *updateBranchPRService) MergePR(_ context.Context, _ domain.Repository, _ int, _, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) CheckMergeable(_ context.Context, _ domain.Repository, _ int) (domain.MergeableState, error) {
	return domain.MergeableState{}, nil
}
func (s *updateBranchPRService) ClosePR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *updateBranchPRService) ReopenPR(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *updateBranchPRService) UpdatePR(_ context.Context, _ domain.Repository, _ int, _, _, _ string) error {
	return nil
}
func (s *updateBranchPRService) CreatePR(_ context.Context, _ domain.CreatePRParams) (domain.PullRequestSummary, error) {
	return domain.PullRequestSummary{}, nil
}
func (s *updateBranchPRService) FetchRepoInfo(_ context.Context, _ domain.Repository) (domain.RepoInfo, error) {
	return domain.RepoInfo{}, nil
}
func (s *updateBranchPRService) ResolveThread(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *updateBranchPRService) UnresolveThread(_ context.Context, _ domain.Repository, _ int, _ string) error {
	return nil
}
func (s *updateBranchPRService) UpdateBranch(ctx context.Context, repo domain.Repository, number int, expectedHeadSHA string) error {
	s.updateBranchCalls++
	s.capturedExpectedSHA = expectedHeadSHA
	if s.updateBranchFn != nil {
		return s.updateBranchFn(ctx, repo, number, expectedHeadSHA)
	}
	return nil
}

// newUpdateModel constructs a PRDetailModel wired with the updateBranchPRService
// fake and a Detail snapshot reflecting the given mergeability and state.
func newUpdateModel(mergeState string, prState domain.PRState) *PRDetailModel {
	repo := testutil.Repo("acme/api")
	summary := domain.PullRequestSummary{
		ID:         "pr_123",
		Repo:       repo.FullName,
		Number:     42,
		Title:      "Feature",
		Author:     "octocat",
		HeadRefOID: "abc123",
	}
	svc := &updateBranchPRService{}
	m := NewModel(summary, repo, svc)
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:       repo.FullName,
		Number:     42,
		State:      prState,
		Mergeable:  "MERGEABLE",
		MergeState: mergeState,
	}
	m.Width = 120
	m.Height = 40
	return m
}

// svc returns the fake service from a model built by newUpdateModel.
func (m *PRDetailModel) updateSvc() *updateBranchPRService {
	svc, _ := m.PRService.(*updateBranchPRService)
	return svc
}

// ─── Gating ─────────────────────────────────────────────────────────────────

func TestUpdateKeyNonBehind(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("CLEAN", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone on non-BEHIND PR, got %d", m.updateStep)
	}
	if m.updateErr == "" {
		t.Fatal("expected updateErr to be set when PR is not behind")
	}
	if !strings.Contains(m.updateErr, "not behind base") {
		t.Fatalf("expected 'not behind base' in hint, got: %s", m.updateErr)
	}
	if m.updateSvc().updateBranchCalls != 0 {
		t.Fatalf("expected 0 service calls, got %d", m.updateSvc().updateBranchCalls)
	}
}

func TestUpdateKeyClosedPR(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateClosed)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone on closed PR, got %d", m.updateStep)
	}
	if m.updateErr == "" {
		t.Fatal("expected updateErr to be set on closed PR")
	}
	if !strings.Contains(m.updateErr, "closed") {
		t.Fatalf("expected 'closed' in hint, got: %s", m.updateErr)
	}
	if m.updateSvc().updateBranchCalls != 0 {
		t.Fatalf("expected 0 service calls, got %d", m.updateSvc().updateBranchCalls)
	}
}

func TestUpdateKeyMergedPR(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("MERGED", domain.PRStateMerged)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone on merged PR, got %d", m.updateStep)
	}
	if m.updateSvc().updateBranchCalls != 0 {
		t.Fatalf("expected 0 service calls on merged PR")
	}
}

func TestUpdateKeyOnDraftAllow(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Detail.IsDraft = true
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepConfirm {
		t.Fatalf("expected updateStepConfirm on draft BEHIND PR, got %d", m.updateStep)
	}
	if m.updateSvc().updateBranchCalls != 0 {
		t.Fatalf("expected 0 service calls at confirm stage")
	}
}

func TestLowerUDPoesNotStartFlow(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone for lowercase u, got %d", m.updateStep)
	}
	if m.updateSvc().updateBranchCalls != 0 {
		t.Fatalf("expected 0 service calls for lowercase u, got %d", m.updateSvc().updateBranchCalls)
	}
}

// ─── Flow success path ──────────────────────────────────────────────────────

func TestUpdateFlowSuccess(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)

	// U → confirm
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepConfirm {
		t.Fatalf("expected updateStepConfirm after U, got %d", m.updateStep)
	}
	if got := m.StatusHint(); !strings.Contains(got, "Update branch #42") || !strings.Contains(got, "(y/n)") {
		t.Fatalf("expected confirm hint, got: %q", got)
	}

	// y → executing, returns UpdateBranchCmd
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.updateStep != updateStepExecuting {
		t.Fatalf("expected updateStepExecuting after y, got %d", m.updateStep)
	}
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd after confirm")
	}
	// Execute the cmd → should yield cmds.UpdateBranchMsg with no Err.
	msg := cmd()
	updMsg, ok := msg.(cmds.UpdateBranchMsg)
	if !ok {
		t.Fatalf("expected cmds.UpdateBranchMsg, got %T", msg)
	}
	if updMsg.Err != nil {
		t.Fatalf("expected nil Err from successful UpdateBranch, got %v", updMsg.Err)
	}
	if updMsg.Repo != "acme/api" || updMsg.Number != 42 {
		t.Fatalf("unexpected msg fields: %+v", updMsg)
	}
	if m.updateSvc().updateBranchCalls != 1 {
		t.Fatalf("expected 1 service call after executing cmd, got %d", m.updateSvc().updateBranchCalls)
	}

	// Feed the success msg back → reset + refresh cmd batched.
	_, afterCmd := m.Update(updMsg)
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone after success msg, got %d", m.updateStep)
	}
	if m.updateErr != "" {
		t.Fatalf("expected empty updateErr after success, got: %q", m.updateErr)
	}
	if afterCmd == nil {
		t.Fatal("expected non-nil cmd batch (refresh) after success")
	}
}

func TestUpdateFlowSendsEmptyExpectedHeadSHA(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	cmd()
	if got := m.updateSvc().capturedExpectedSHA; got != "" {
		t.Fatalf("expected empty expected_head_sha (server default), got %q", got)
	}
}

// ─── Cancel / error paths ───────────────────────────────────────────────────

func TestUpdateFlowCancelAtConfirmWithN(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone after n-cancel, got %d", m.updateStep)
	}
}

func TestUpdateFlowCancelAtConfirmWithEsc(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone after esc-cancel, got %d", m.updateStep)
	}
}

func TestUpdateFlowNetworkError(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m.Update(cmds.UpdateBranchMsg{Repo: "acme/api", Number: 42, Err: errors.New("timeout")})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone after error, got %d", m.updateStep)
	}
	if m.updateErr == "" {
		t.Fatal("expected updateErr set after network error")
	}
	if !strings.Contains(m.updateErr, "Update branch failed:") {
		t.Fatalf("expected 'Update branch failed:' prefix, got: %s", m.updateErr)
	}
	if !strings.Contains(m.updateErr, "timeout") {
		t.Fatalf("expected 'timeout' in error, got: %s", m.updateErr)
	}
}

func TestUpdateFlowMergeError422(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m.Update(cmds.UpdateBranchMsg{Repo: "acme/api", Number: 42, Err: errors.New("rest: unexpected status 422: branch is not behind base")})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone after 422, got %d", m.updateStep)
	}
	if !strings.Contains(m.updateErr, "422") {
		t.Fatalf("expected '422' in updateErr, got: %s", m.updateErr)
	}
}

// ─── Concurrency guards ─────────────────────────────────────────────────────

func TestUpdateBlockedWhenMergeInProgress(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.mergeStep = mergeStepSelectMethod
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone when merge flow is active, got %d", m.updateStep)
	}
	if m.updateSvc().updateBranchCalls != 0 {
		t.Fatalf("expected 0 service calls when merge flow is active")
	}
}

func TestMergeBlockedWhenUpdateInProgress(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("MERGEABLE", domain.PRStateOpen)
	m.Detail.Mergeable = "MERGEABLE"
	m.updateStep = updateStepExecuting
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	if m.mergeStep != mergeStepNone {
		t.Fatalf("expected mergeStepNone when update flow is active, got %d", m.mergeStep)
	}
}

func TestUpdateBlockedWhenCloseInProgress(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.closeStep = closeStepConfirm
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone when close flow is active, got %d", m.updateStep)
	}
}

// ─── Visual mode / drafts ───────────────────────────────────────────────────

func TestUpdateVisualModeIgnored(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.visual.Active = true
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone when visual mode is active, got %d", m.updateStep)
	}
}

func TestUpdateDraftsSurvive(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.drafts = []domain.DraftInlineComment{{Path: "main.go", Line: 10, Body: "fix this"}}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if len(m.drafts) != 1 {
		t.Fatalf("expected drafts to survive, got %d", len(m.drafts))
	}
}

func TestUpdateEscBehavior(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.updateStep != updateStepNone {
		t.Fatalf("expected updateStepNone after esc, got %d", m.updateStep)
	}
}

// ─── StatusHint UI text ─────────────────────────────────────────────────────

func TestUpdateStatusHint_Confirm(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	got := m.StatusHint()
	want := "Update branch #42 with base? (y/n)"
	if got != want {
		t.Fatalf("StatusHint mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestUpdateStatusHint_Executing(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.updateStep = updateStepExecuting
	got := m.StatusHint()
	if got != "Updating branch..." {
		t.Fatalf("expected 'Updating branch...', got: %q", got)
	}
}

func TestUpdateStatusHint_ErrorDisplacement(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.updateErr = "stale error text"
	// Any non-U / non-x / non-M key dismisses updateErr per handleUpdateKey
	// updateStepNone branch. Using "j" to avoid keys consumed by sibling flow
	// handlers (x → close, M → merge) before reaching handleUpdateKey.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.updateErr != "" {
		t.Fatalf("expected updateErr to be cleared by next key, got: %q", m.updateErr)
	}
}

func TestUpdateStatusHint_NotBehindError(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("CLEAN", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	got := m.StatusHint()
	if !strings.Contains(got, "not behind base") {
		t.Fatalf("expected 'not behind base' in StatusHint, got: %q", got)
	}
}

// ─── Service invocation contract ─────────────────────────────────────────────

func TestUpdateCmdCallsServiceWithRightArgs(t *testing.T) {
	t.Parallel()
	m := newUpdateModel("BEHIND", domain.PRStateOpen)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	cmd()
	svc := m.updateSvc()
	if svc.updateBranchCalls != 1 {
		t.Fatalf("expected 1 call, got %d", svc.updateBranchCalls)
	}
	if svc.capturedExpectedSHA != "" {
		t.Errorf("expected empty expected_head_sha, got %q", svc.capturedExpectedSHA)
	}
}

// ─── Golden snapshot tests ──────────────────────────────────────────────────
//
// These exercise the rendered output of the PR-detail view while the
// update-branch flow is active at various states. They generate golden files
// under testdata/golden/ the first time they run with -update, and on
// subsequent runs they compare the live render against the committed goldens.
//
// To regenerate goldens after intentional UI changes:
//
//	go test -run 'TestUpdate.*Golden' -update ./internal/ui/views/prdetail/...

// updateGoldenWidths is the same width sweep used by compose/commits golden
// tests; we keep it for parity.
var updateGoldenWidths = composeGoldenWidths

// newUpdateGoldenModel builds a PRDetailModel with believable Detail/files so
// that the full View() renders header + sidebar + content (mirroring
// render_integration_test's setup). updateStep is the caller's responsibility.
func newUpdateGoldenModel(width int, mergeState string, prState domain.PRState) *PRDetailModel {
	m := makePRDetail(width, 40, makeFiles("pkg/foo/bar.go", "pkg/quux/main.go"), []domain.PreviewCheckRow{
		{Name: "ci/build", State: "SUCCESS"},
		{Name: "ci/test", State: "FAILURE"},
	})
	m.PRService = &updateBranchPRService{}
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:        "owner/repo",
		Number:      42,
		State:       prState,
		Title:       "Update branch snapshot",
		BodyExcerpt: "This PR has some interesting changes.",
		Mergeable:   "MERGEABLE",
		MergeState:  mergeState,
	}
	return m
}

func TestUpdateView_ConfirmGolden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := newUpdateGoldenModel(w, "BEHIND", domain.PRStateOpen)
			m.SetTheme(theme.Default())
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
			if m.updateStep != updateStepConfirm {
				t.Fatalf("setup: expected updateStep=confirm, got %d", m.updateStep)
			}
			got := descStripANSI(m.View())
			if got == "" {
				t.Fatal("rendered View is empty")
			}
			checkGolden(t, got, fmt.Sprintf("update_branch_view_confirm_w%d.txt", w))
		})
	}
}

func TestUpdateView_ExecutingGolden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := newUpdateGoldenModel(w, "BEHIND", domain.PRStateOpen)
			m.SetTheme(theme.Default())
			m.updateStep = updateStepExecuting
			got := descStripANSI(m.View())
			if got == "" {
				t.Fatal("rendered View is empty")
			}
			checkGolden(t, got, fmt.Sprintf("update_branch_view_executing_w%d.txt", w))
		})
	}
}

func TestUpdateView_NotBehindErrorGolden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := newUpdateGoldenModel(w, "CLEAN", domain.PRStateOpen)
			m.SetTheme(theme.Default())
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
			if m.updateStep != updateStepNone {
				t.Fatalf("setup: expected updateStepNone on non-BEHIND PR, got %d", m.updateStep)
			}
			if m.updateErr == "" {
				t.Fatal("expected updateErr to be populated")
			}
			got := descStripANSI(m.View())
			if got == "" {
				t.Fatal("rendered View is empty")
			}
			// updateErr surfaces via StatusHint, not View; assert the goldens
			// capture the unaffected header/body instead. StatusHint is verified
			// separately in TestUpdateStatusHint_NotBehindErrorGolden.
			checkGolden(t, got, fmt.Sprintf("update_branch_view_not_behind_w%d.txt", w))
		})
	}
}

func TestUpdateStatusHint_ConfirmGolden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		_ = w // StatusHint is width-agnostic; iterate widths anyway for parity.
		m := newUpdateModel("BEHIND", domain.PRStateOpen)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
		got := descStripANSI(m.StatusHint())
		checkGolden(t, got, fmt.Sprintf("update_branch_status_confirm_w%d.txt", w))
	}
}

func TestUpdateStatusHint_ExecutingGolden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		_ = w
		m := newUpdateModel("BEHIND", domain.PRStateOpen)
		m.updateStep = updateStepExecuting
		got := descStripANSI(m.StatusHint())
		checkGolden(t, got, fmt.Sprintf("update_branch_status_executing_w%d.txt", w))
	}
}

func TestUpdateStatusHint_NotBehindErrorGolden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		_ = w
		m := newUpdateModel("CLEAN", domain.PRStateOpen)
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
		got := descStripANSI(m.StatusHint())
		checkGolden(t, got, fmt.Sprintf("update_branch_status_not_behind_w%d.txt", w))
	}
}

func TestUpdateStatusHint_Error422Golden(t *testing.T) {
	t.Parallel()
	for _, w := range updateGoldenWidths {
		_ = w
		m := newUpdateModel("BEHIND", domain.PRStateOpen)
		m.updateErr = "Update branch failed: rest: unexpected status 422: branch is not behind base"
		got := descStripANSI(m.StatusHint())
		checkGolden(t, got, fmt.Sprintf("update_branch_status_error_422_w%d.txt", w))
	}
}
