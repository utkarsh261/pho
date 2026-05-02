package prdetail

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
)

func TestCheckoutKeyEmitsCommand(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.Repo.LocalPath = "/workspace/repo"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd == nil {
		t.Fatal("expected b to emit a command")
	}
	// The command is a tea.Batch containing the checkout command.
	// Just verify the model state changed.
	if !m.checkoutInFlight {
		t.Fatal("expected checkoutInFlight=true")
	}
}

func TestCheckoutKeyNoLocalPathShowsError(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.Repo.LocalPath = ""

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd == nil {
		t.Fatal("expected b to return a timer command for empty LocalPath")
	}
	if m.checkoutErr != "Not a local repo" {
		t.Fatalf("expected checkoutErr 'Not a local repo', got %q", m.checkoutErr)
	}
	hint := m.StatusHint()
	if !strings.Contains(hint, "Not a local repo") {
		t.Fatalf("expected error in status hint, got %q", hint)
	}
	// Verify the command is a tick that will clear the error.
	msg := cmd()
	if _, ok := msg.(checkoutClearMsg); !ok {
		t.Fatalf("expected checkoutClearMsg, got %T", msg)
	}
}

func TestCheckoutKeyIgnoredInCommitMode(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.CommitMode = true
	m.Repo.LocalPath = "/workspace/repo"

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd != nil {
		t.Fatal("expected b to be ignored in commit mode")
	}
}

func TestCheckoutKeyIgnoredWhileInFlight(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.Repo.LocalPath = "/workspace/repo"
	m.checkoutInFlight = true

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd != nil {
		t.Fatal("expected b to be ignored while checkout is in flight")
	}
}

func TestCheckoutStatusHintInFlight(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutInFlight = true

	hint := m.StatusHint()
	if !strings.Contains(hint, "Checking out") {
		t.Fatalf("expected 'Checking out' hint, got %q", hint)
	}
}

func TestCheckoutStatusHintError(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutErr = "working tree dirty"

	hint := m.StatusHint()
	if !strings.Contains(hint, "working tree dirty") {
		t.Fatalf("expected error in status hint, got %q", hint)
	}
}

func TestCheckoutStatusHintSuccess(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutStatus = "Checked out feature/foo"

	hint := m.StatusHint()
	if !strings.Contains(hint, "Checked out feature/foo") {
		t.Fatalf("expected success in status hint, got %q", hint)
	}
}

func TestCheckoutResultSuccess(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutInFlight = true

	m, cmd := m.Update(checkoutResultMsg{branch: "feature/foo"})
	if m.checkoutInFlight {
		t.Fatal("expected checkoutInFlight=false after success")
	}
	if m.checkoutStatus != "Checked out feature/foo" {
		t.Fatalf("expected checkoutStatus, got %q", m.checkoutStatus)
	}
	if cmd == nil {
		t.Fatal("expected timer command to clear status")
	}
	msg := cmd()
	if _, ok := msg.(checkoutClearMsg); !ok {
		t.Fatalf("expected checkoutClearMsg, got %T", msg)
	}
}

func TestCheckoutResultError(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutInFlight = true

	m, cmd := m.Update(checkoutResultMsg{err: cmds.CheckoutBranchCmd(domain.Repository{}, 1, "x", false)().(cmds.CheckoutResult).Err})
	if m.checkoutInFlight {
		t.Fatal("expected checkoutInFlight=false after error")
	}
	if m.checkoutErr == "" {
		t.Fatal("expected checkoutErr after failure")
	}
	if cmd == nil {
		t.Fatal("expected timer command to clear error")
	}
	msg := cmd()
	if _, ok := msg.(checkoutClearMsg); !ok {
		t.Fatalf("expected checkoutClearMsg, got %T", msg)
	}
}

func TestCheckoutClearMsg(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutStatus = "Checked out feature/foo"
	m.checkoutErr = ""
	m.checkoutInFlight = false

	m, _ = m.Update(checkoutClearMsg{})
	if m.checkoutStatus != "" {
		t.Fatalf("expected checkoutStatus cleared, got %q", m.checkoutStatus)
	}
	if m.checkoutErr != "" {
		t.Fatalf("expected checkoutErr cleared, got %q", m.checkoutErr)
	}
}

func TestCheckoutCmdsResultMapping(t *testing.T) {
	t.Parallel()
	m := makeCheckoutModel()
	m.checkoutInFlight = true

	m, cmd := m.Update(cmds.CheckoutResult{Branch: "feature/bar", Err: nil})
	if !m.checkoutInFlight {
		t.Fatal("expected checkoutInFlight still true after cmds.CheckoutResult (mapping only)")
	}
	// Execute the returned cmd to get the mapped message.
	msg := cmd()
	mapped, ok := msg.(checkoutResultMsg)
	if !ok {
		t.Fatalf("expected checkoutResultMsg, got %T", msg)
	}
	if mapped.branch != "feature/bar" {
		t.Fatalf("expected branch 'feature/bar', got %q", mapped.branch)
	}
	// Feed the mapped message back into Update to trigger state change.
	m, _ = m.Update(mapped)
	if m.checkoutInFlight {
		t.Fatal("expected checkoutInFlight=false after mapped result")
	}
	if m.checkoutStatus != "Checked out feature/bar" {
		t.Fatalf("expected checkoutStatus 'Checked out feature/bar', got %q", m.checkoutStatus)
	}
}

func makeCheckoutModel() *PRDetailModel {
	m := NewModel(domain.PullRequestSummary{
		Repo:        "org/repo",
		Number:      42,
		ID:          "pr-id-42",
		HeadRefName: "feature/foo",
		State:       domain.PRStateOpen,
	}, domain.Repository{Host: "github.com", FullName: "org/repo"}, nil)
	m.Detail = &domain.PRPreviewSnapshot{
		Repo:   "org/repo",
		Number: 42,
		State:  domain.PRStateOpen,
	}
	m.Width = 100
	m.Height = 40
	return m
}
