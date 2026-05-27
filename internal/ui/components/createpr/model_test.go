package createpr

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

var updateGolden = flag.Bool("update-createpr", false, "overwrite createpr golden files with current output")

// brokenANSI matches escape sequences that are malformed (missing the ESC prefix
// or having a trailing/leading partial sequence that was split mid-render).
var brokenANSI = regexp.MustCompile(`(?m)(^[^\x1b]*[0-9;]+m|^[^\x1b]*m[0-9;]+$)`)

func hasBrokenANSI(s string) bool {
	return brokenANSI.MatchString(s)
}

// stripANSI removes all ANSI escape codes so we can compare semantic content.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func checkGolden(t *testing.T, got, name string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update-createpr to generate)", goldenPath, err)
	}
	if got != string(data) {
		t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, string(data))
	}
}

func TestOverlayViewLoadingSnapshot(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetSize(80, 24)
	m.SetTheme(theme.Default())

	view := m.View()
	if hasBrokenANSI(view) {
		t.Fatal("loading view contains broken ANSI sequences")
	}
	checkGolden(t, stripANSI(view), "overlay_loading.txt")
}

func TestOverlayViewIdleSnapshot(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetSize(80, 24)
	m.SetTheme(theme.Default())

	data := cmds.CreatePRFormData{
		DefaultBase:    "main",
		CurrentBranch:  "feature",
		LastCommitMsg:  "Add feature",
		LocalBranches:  []string{"main", "feature", "fix"},
		RemoteBranches: []string{"origin/main", "origin/feature"},
	}
	m.SetFormData(data)

	view := m.View()
	if hasBrokenANSI(view) {
		t.Fatal("idle view contains broken ANSI sequences")
	}
	checkGolden(t, stripANSI(view), "overlay_idle.txt")
}

func TestPanelViewIdleSnapshot(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetTheme(theme.Default())

	data := cmds.CreatePRFormData{
		DefaultBase:    "main",
		CurrentBranch:  "feature",
		LastCommitMsg:  "Add feature",
		LocalBranches:  []string{"main", "feature", "fix"},
		RemoteBranches: []string{"origin/main", "origin/feature"},
	}
	m.SetFormData(data)

	view := m.PanelView(80, 24)
	if hasBrokenANSI(view) {
		t.Fatal("panel idle view contains broken ANSI sequences")
	}
	clean := stripANSI(view)
	for i, line := range strings.Split(clean, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Errorf("panel idle: line %d exceeds width (80): %q", i, line)
		}
	}
	checkGolden(t, clean, "panel_idle.txt")
}

func TestOverlayViewErrorSnapshot(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetSize(80, 24)
	m.SetTheme(theme.Default())

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)
	m.SetError(fmt.Errorf("validation failed"))

	view := m.View()
	if hasBrokenANSI(view) {
		t.Fatal("error view contains broken ANSI sequences")
	}
	checkGolden(t, stripANSI(view), "overlay_error.txt")
}

func TestOverlayViewOverDoesNotCorruptBackground(t *testing.T) {
	t.Parallel()

	// Build a realistic styled background with ANSI codes.
	styledLine := "\x1b[38;2;124;58;237mRepo\x1b[0m  \x1b[38;5;81mPR #1\x1b[0m  Title"
	bg := strings.Repeat(styledLine+"\n", 20)

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetSize(80, 24)
	m.SetTheme(theme.Default())

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	result := m.ViewOver(bg)
	if hasBrokenANSI(result) {
		t.Fatal("ViewOver output contains broken ANSI sequences")
	}
	// Verify the styled background text is still present outside the overlay.
	if !strings.Contains(result, "Repo") {
		t.Fatal("ViewOver destroyed background content")
	}
	checkGolden(t, stripANSI(result), "overlay_over_bg.txt")
}

func TestOverlayViewOverInactiveReturnsBackground(t *testing.T) {
	t.Parallel()

	m := NewModel()
	bg := "background content"
	result := m.ViewOver(bg)
	if result != bg {
		t.Errorf("expected background unchanged when inactive, got %q", result)
	}
}

// focusField sends Tab to the form until it reaches the target field,
// runs any returned commands, and returns the view.
func focusField(t *testing.T, m *Model, targetKey string) string {
	t.Helper()
	// Huh forms have 5 fields: base, head, draft, title, body.
	// We iterate up to 10 times to be safe.
	for i := 0; i < 10; i++ {
		cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if cmd != nil {
			msg := cmd()
			// Feed the message back into the model.
			cmd2 := m.Update(msg)
			if cmd2 != nil {
				cmd2()
			}
		}
		if m.form != nil {
			f := m.form.GetFocusedField()
			if f != nil && f.GetKey() == targetKey {
				break
			}
		}
	}
	return m.View()
}

func TestFormFieldFocusSnapshots(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetSize(80, 24)
	m.SetTheme(theme.Default())

	data := cmds.CreatePRFormData{
		DefaultBase:    "main",
		CurrentBranch:  "feature",
		LastCommitMsg:  "Add feature",
		LocalBranches:  []string{"main", "feature", "fix"},
		RemoteBranches: []string{"origin/main", "origin/feature"},
	}
	if cmd := m.SetFormData(data); cmd != nil {
		msg := cmd()
		cmd2 := m.Update(msg)
		if cmd2 != nil {
			cmd2()
		}
	}

	fields := []string{"base", "head", "draft", "title", "body"}
	for _, key := range fields {
		view := focusField(t, m, key)
		if hasBrokenANSI(view) {
			t.Fatalf("focus %s: broken ANSI", key)
		}
		// Strip ANSI and check every line fits within the 80-col overlay.
		clean := stripANSI(view)
		for i, line := range strings.Split(clean, "\n") {
			if lipgloss.Width(line) > 80 {
				t.Errorf("focus %s: line %d exceeds panel width (80): %q", key, i, line)
			}
		}
		checkGolden(t, clean, fmt.Sprintf("focus_%s.txt", key))
	}
}

func TestNewModelInactive(t *testing.T) {
	t.Parallel()

	m := NewModel()
	if m.Active() {
		t.Error("expected new model to be inactive")
	}
}

func TestOpenActivatesOverlay(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{
		Host:     "github.com",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
	}

	m.Open(repo)

	if !m.Active() {
		t.Error("expected overlay to be active after Open")
	}
}

func TestCloseDeactivatesOverlay(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.Close()

	if m.Active() {
		t.Error("expected overlay to be inactive after Close")
	}
}

func TestSetFormDataBuildsForm(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:    "main",
		CurrentBranch:  "feature",
		LastCommitMsg:  "Add feature",
		LocalBranches:  []string{"main", "feature", "fix"},
		RemoteBranches: []string{"origin/main", "origin/feature"},
	}

	m.SetFormData(data)

	if m.form == nil {
		t.Fatal("expected form to be built after SetFormData")
	}
	if m.baseBranches[0] != "main" {
		t.Errorf("expected first base branch to be default, got %q", m.baseBranches[0])
	}
	if m.headBranches[0] != "feature" {
		t.Errorf("expected first head branch to be current, got %q", m.headBranches[0])
	}
}

func TestSubmitSuccess(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{
		Host:     "github.com",
		Owner:    "owner",
		Name:     "repo",
		FullName: "owner/repo",
	}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	// Verify form is built and branch lists are populated.
	if m.form == nil {
		t.Fatal("expected form to be built")
	}
	if len(m.baseBranches) == 0 {
		t.Error("expected base branches to be populated")
	}
	if len(m.headBranches) == 0 {
		t.Error("expected head branches to be populated")
	}
	// Verify default base is first.
	if m.baseBranches[0] != "main" {
		t.Errorf("expected first base branch to be default %q, got %q", "main", m.baseBranches[0])
	}
	// Verify current branch is first in head.
	if m.headBranches[0] != "feature" {
		t.Errorf("expected first head branch to be current %q, got %q", "feature", m.headBranches[0])
	}
}

func TestSubmitEmptyTitleError(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	// Initialize the form.
	if cmd := m.form.Init(); cmd != nil {
		cmd()
	}

	_, err := m.Submit()
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if err.Error() != "title is required" {
		t.Errorf("expected error=%q, got %q", "title is required", err.Error())
	}
}

func TestSubmitForkPrefixesHead(t *testing.T) {
	t.Parallel()

	m := NewModel()
	forkRepo := domain.Repository{
		Host:     "github.com",
		Owner:    "fork-user",
		Name:     "repo",
		FullName: "fork-user/repo",
	}
	m.Open(forkRepo)

	data := cmds.CreatePRFormData{
		DefaultBase:    "main",
		CurrentBranch:  "feature",
		LastCommitMsg:  "Add feature",
		LocalBranches:  []string{"main", "feature"},
		IsFork:         true,
		ParentFullName: "upstream-org/repo",
	}
	m.SetFormData(data)

	// Verify fork data is stored correctly.
	if !m.formData.IsFork {
		t.Error("expected IsFork=true")
	}
	if m.formData.ParentFullName != "upstream-org/repo" {
		t.Errorf("expected ParentFullName=%q, got %q", "upstream-org/repo", m.formData.ParentFullName)
	}
	// Verify form is built.
	if m.form == nil {
		t.Fatal("expected form to be built")
	}
}

func TestSubmitNoFormError(t *testing.T) {
	t.Parallel()

	m := NewModel()
	_, err := m.Submit()
	if err == nil {
		t.Fatal("expected error when form is not initialized")
	}
}

func TestUpdateCancelMsgOnEsc(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command on Esc")
	}
	msg := cmd()
	if _, ok := msg.(CancelMsg); !ok {
		t.Errorf("expected CancelMsg, got %T", msg)
	}
}

func TestUpdateSubmitMsgOnCtrlS(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected command on Ctrl+S")
	}
	msg := cmd()
	if _, ok := msg.(SubmitMsg); !ok {
		t.Errorf("expected SubmitMsg, got %T", msg)
	}
}

func TestUpdateInactiveReturnsNil(t *testing.T) {
	t.Parallel()

	m := NewModel()
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("expected nil command when inactive")
	}
}

func TestUpdateEscCancelsDuringLoading(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	// Status is loading after Open, before SetFormData.

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command on Esc during loading")
	}
	msg := cmd()
	if _, ok := msg.(CancelMsg); !ok {
		t.Errorf("expected CancelMsg, got %T", msg)
	}
}

func TestSetError(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	m.SetError(nil)
	if m.status != overlayStatusError {
		t.Errorf("expected status=error, got %d", m.status)
	}
	if m.errMsg != "" {
		t.Errorf("expected empty error message, got %q", m.errMsg)
	}

	m.SetError(nil) // Reset
	m.SetError(fmt.Errorf("validation failed"))
	if m.errMsg != "validation failed" {
		t.Errorf("expected error=%q, got %q", "validation failed", m.errMsg)
	}
}

func TestSetSubmitting(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)

	data := cmds.CreatePRFormData{
		DefaultBase:   "main",
		CurrentBranch: "feature",
		LastCommitMsg: "Add feature",
		LocalBranches: []string{"main", "feature"},
	}
	m.SetFormData(data)

	m.SetSubmitting()
	if m.status != overlayStatusSubmitting {
		t.Errorf("expected status=submitting, got %d", m.status)
	}
}

func TestViewInactiveReturnsEmpty(t *testing.T) {
	t.Parallel()

	m := NewModel()
	view := m.View()
	if view != "" {
		t.Error("expected empty view when inactive")
	}
}

func TestViewLoadingState(t *testing.T) {
	t.Parallel()

	m := NewModel()
	repo := domain.Repository{FullName: "owner/repo"}
	m.Open(repo)
	m.SetSize(80, 24)

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view in loading state")
	}
	if !strings.Contains(view, "Loading repository info") {
		t.Error("expected loading message in view")
	}
}

func TestAllBranchesDeduplicates(t *testing.T) {
	t.Parallel()

	data := cmds.CreatePRFormData{
		LocalBranches:  []string{"main", "feature"},
		RemoteBranches: []string{"origin/main", "origin/develop"},
	}

	branches := allBranches(data)

	seen := make(map[string]bool)
	for _, b := range branches {
		if seen[b] {
			t.Errorf("duplicate branch %q", b)
		}
		seen[b] = true
	}

	if !seen["main"] || !seen["feature"] || !seen["develop"] {
		t.Errorf("expected main, feature, develop in branches, got %v", branches)
	}
}

func TestAllBranchesStripsOriginPrefix(t *testing.T) {
	t.Parallel()

	data := cmds.CreatePRFormData{
		RemoteBranches: []string{"origin/feature", "origin/main"},
	}

	branches := allBranches(data)

	for _, b := range branches {
		if strings.HasPrefix(b, "origin/") {
			t.Errorf("expected origin/ prefix stripped, got %q", b)
		}
	}
}

func TestFilterOut(t *testing.T) {
	t.Parallel()

	src := []string{"main", "feature", "fix", "develop"}
	exclude := []string{"main", "develop"}

	result := filterOut(exclude, src)

	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	if result[0] != "feature" || result[1] != "fix" {
		t.Errorf("expected [feature, fix], got %v", result)
	}
}

func TestBranchOptions(t *testing.T) {
	t.Parallel()

	branches := []string{"main", "feature"}
	opts := branchOptions(branches)

	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d", len(opts))
	}
}
