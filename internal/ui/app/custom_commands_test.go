package app

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/customcmd"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/components/keymapoverlay"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

type customRun struct{ key, command, dir string }

func withCustom(t *testing.T, m *Model, bindings ...customcmd.Binding) *[]customRun {
	t.Helper()
	parsed, err := customcmd.Parse(bindings)
	if err != nil {
		t.Fatal(err)
	}
	var runs []customRun
	m.deps.Keybindings = parsed
	m.deps.RunCustom = func(key, command, dir string) tea.Cmd {
		runs = append(runs, customRun{key, command, dir})
		return nil
	}
	return &runs
}

func customPR() domain.PullRequestSummary {
	p := pr("acme/alpha", 42, "Add thing")
	p.HeadRefName = "feat/thing"
	p.HeadRefOID = "abc123"
	p.Author = "octocat"
	return p
}

const allFields = "{{.RepoName}} {{.RepoPath}} {{.PrNumber}} {{.HeadRefName}} {{.BaseRefName}} {{.HeadSHA}} {{.Author}} {{.URL}}"

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestCustomKeyOnDashboard(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	runs := withCustom(t, m, customcmd.Binding{Key: "w", Command: allFields})
	m.focus = domain.FocusPRListPanel

	key(m, runeKey('w'))

	if len(*runs) != 1 {
		t.Fatalf("runs = %v, want one", *runs)
	}
	want := customRun{"w", "acme/alpha /tmp/test/repo 42 feat/thing main abc123 octocat https://github.com/acme/alpha/pull/42", "/tmp/test/repo"}
	if (*runs)[0] != want {
		t.Errorf("run = %+v\nwant  %+v", (*runs)[0], want)
	}
}

func TestCustomKeyOnDashboardEachFocus(t *testing.T) {
	t.Parallel()
	for _, focus := range []domain.FocusTarget{domain.FocusRepoPanel, domain.FocusPRListPanel, domain.FocusPreviewPanel} {
		repo := testutil.Repo("acme/alpha")
		m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
		runs := withCustom(t, m, customcmd.Binding{Key: "w", Command: "true"})
		m.focus = focus
		key(m, runeKey('w'))
		if len(*runs) != 1 {
			t.Errorf("focus %v: runs = %d, want 1", focus, len(*runs))
		}
	}
}

func TestCustomKeyUnboundDoesNothing(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	runs := withCustom(t, m, customcmd.Binding{Key: "w", Command: "true"})
	m.focus = domain.FocusPRListPanel
	key(m, runeKey('p'))
	if len(*runs) != 0 {
		t.Fatalf("unbound key ran %v", *runs)
	}
}

func TestCustomKeyNoPRSelected(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, nil)
	runs := withCustom(t, m, customcmd.Binding{Key: "w", Command: "true"})
	m.focus = domain.FocusPRListPanel
	key(m, runeKey('w'))
	if len(*runs) != 0 {
		t.Fatalf("ran without a PR: %v", *runs)
	}
	if !strings.Contains(m.status.Notice, "No PR selected") {
		t.Errorf("notice = %q", m.status.Notice)
	}
}

func TestCustomKeyNeedsLocalRepoForRepoPath(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/alpha")
	repo.LocalPath = ""
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	runs := withCustom(t, m,
		customcmd.Binding{Key: "w", Command: "cd {{.RepoPath}} && lazygit"},
		customcmd.Binding{Key: "P", Command: "gh pr view {{.PrNumber}}"})
	m.focus = domain.FocusPRListPanel

	key(m, runeKey('w'))
	if len(*runs) != 0 {
		t.Fatalf("ran without a local repo: %v", *runs)
	}
	if !strings.Contains(m.status.Notice, "Not a local repo") {
		t.Errorf("notice = %q", m.status.Notice)
	}

	// Without RepoPath it runs, from pho's own directory.
	key(m, runeKey('P'))
	if len(*runs) != 1 || (*runs)[0].dir != "" || (*runs)[0].command != "gh pr view 42" {
		t.Fatalf("runs = %+v", *runs)
	}
}

func TestCustomKeyTypedIntoPalette(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	runs := withCustom(t, m, customcmd.Binding{Key: "w", Command: "true"})
	key(m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.focus != domain.FocusCmdPalette {
		t.Fatalf("focus = %v, want palette", m.focus)
	}
	key(m, runeKey('w'))
	if len(*runs) != 0 {
		t.Fatalf("palette typing ran %v", *runs)
	}
	if !strings.Contains(m.palette.Query(), "w") {
		t.Errorf("palette query = %q, want it to hold w", m.palette.Query())
	}
}

func TestCustomKeyInertWhileOverlayOpen(t *testing.T) {
	t.Parallel()
	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	runs := withCustom(t, m, customcmd.Binding{Key: "w", Name: "worktree", Command: "true"})
	m.focus = domain.FocusPRListPanel
	key(m, runeKey('?'))
	if !m.keymapOverlay.Visible {
		t.Fatal("overlay not open")
	}
	key(m, runeKey('w'))
	if len(*runs) != 0 {
		t.Fatalf("ran under the overlay: %v", *runs)
	}
}

func openCustomPRDetail(t *testing.T, repo domain.Repository) (*Model, *[]customRun) {
	t.Helper()
	summary := customPR()
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{summary})
	runs := withCustom(t, m, customcmd.Binding{Key: "w", Command: allFields})
	m.focus = domain.FocusPRListPanel
	_, _ = m.Update(dashboard.SelectPRMsg{Tab: domain.TabMyPRs, Index: 0, Repo: repo.FullName, Number: 42, Summary: summary})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("view = %s, want PR detail", m.currentView())
	}
	return m, runs
}

func TestCustomKeyInPRDetail(t *testing.T) {
	t.Parallel()
	m, runs := openCustomPRDetail(t, testutil.Repo("acme/alpha"))
	// PR detail asks the root model through a message.
	_, _ = m.Update(prdetail.CustomKey{Key: "w"})
	if len(*runs) != 1 {
		t.Fatalf("runs = %v, want one", *runs)
	}
	want := customRun{"w", "acme/alpha /tmp/test/repo 42 feat/thing main abc123 octocat https://github.com/acme/alpha/pull/42", "/tmp/test/repo"}
	if (*runs)[0] != want {
		t.Errorf("run = %+v\nwant  %+v", (*runs)[0], want)
	}
}

// `pho pr N` starts from a bare {repo, number}; until the PR loads, its
// fields would render as empty strings.
func TestCustomKeyWaitsForPRToLoad(t *testing.T) {
	t.Parallel()
	m, runs := openCustomPRDetail(t, testutil.Repo("acme/alpha"))
	m.prDetail.Summary = domain.PullRequestSummary{Repo: "acme/alpha", Number: 42}
	_, _ = m.Update(prdetail.CustomKey{Key: "w"})
	if len(*runs) != 0 {
		t.Fatalf("ran before the PR loaded: %v", *runs)
	}
	if hint := m.prDetail.StatusHint(); !strings.Contains(hint, "PR still loading") {
		t.Errorf("hint = %q", hint)
	}
}

func TestCustomKeyPRDetailKeyPath(t *testing.T) {
	t.Parallel()
	m, runs := openCustomPRDetail(t, testutil.Repo("acme/alpha"))
	cmd := key(m, runeKey('w'))
	drain(m, cmd)
	if len(*runs) != 1 {
		t.Fatalf("runs = %v, want one", *runs)
	}
}

// drain feeds cmd's messages, and batches', back into m, skipping ones
// that block.
func drain(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				drain(m, c)
			}
			return
		}
		if _, ok := msg.(prdetail.CustomKey); ok {
			_, _ = m.Update(msg)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCustomCommandFailureNotice(t *testing.T) {
	t.Parallel()
	exitErr := exec.Command("sh", "-c", "exit 3").Run()

	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	withCustom(t, m, customcmd.Binding{Key: "w", Command: "true"})
	_, _ = m.Update(customcmd.FinishedMsg{Key: "w", Err: exitErr})
	if !strings.Contains(m.status.Notice, "Custom command w failed (exit 3)") {
		t.Errorf("dashboard notice = %q", m.status.Notice)
	}
	// The next key clears it.
	key(m, runeKey('j'))
	if strings.Contains(m.status.Notice, "failed") {
		t.Errorf("notice survived a key press: %q", m.status.Notice)
	}

	d, _ := openCustomPRDetail(t, repo)
	_, _ = d.Update(customcmd.FinishedMsg{Key: "w", Err: errors.New("exec: \"sh\": not found")})
	if hint := d.prDetail.StatusHint(); !strings.Contains(hint, "Custom command w failed") {
		t.Errorf("PR detail hint = %q", hint)
	}
	_, _ = d.Update(customcmd.FinishedMsg{Key: "w"})
	if hint := d.prDetail.StatusHint(); strings.Contains(hint, "failed") {
		t.Errorf("success left the failure up: %q", hint)
	}
}

func TestCustomKeysInOverlay(t *testing.T) {
	t.Parallel()
	custom := []keymapoverlay.Binding{{Key: "w", Description: "worktree"}}
	has := func(groups []keymapoverlay.Group) bool {
		for _, g := range groups {
			if g.Name == "Custom" && len(g.Bindings) == 1 && g.Bindings[0] == custom[0] {
				return true
			}
		}
		return false
	}
	for _, view := range []domain.PrimaryView{domain.PrimaryViewDashboard, domain.PrimaryViewPRDetail} {
		if !has(keymapoverlay.BuildBindings(keymapoverlay.Context{View: view, Custom: custom})) {
			t.Errorf("%s: no Custom group", view)
		}
		if has(keymapoverlay.BuildBindings(keymapoverlay.Context{View: view})) {
			t.Errorf("%s: Custom group without bindings", view)
		}
	}
	if has(keymapoverlay.BuildBindings(keymapoverlay.Context{View: domain.PrimaryViewCommitDetail, Custom: custom})) {
		t.Error("commit detail lists custom keys, which don't work there")
	}

	// The app passes its bindings in.
	repo := testutil.Repo("acme/alpha")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{customPR()})
	withCustom(t, m, customcmd.Binding{Key: "w", Name: "worktree", Command: "true"})
	m.focus = domain.FocusPRListPanel
	key(m, runeKey('?'))
	if !has(m.keymapOverlay.Groups) {
		t.Errorf("overlay groups = %+v", m.keymapOverlay.Groups)
	}
}
