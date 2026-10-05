package createpr

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

func openPanel(t *testing.T, repo domain.Repository, data cmds.CreatePRFormData) *Model {
	t.Helper()
	m := NewModel()
	m.Open(repo)
	m.SetTheme(theme.Default())
	m.SetFormData(data)
	return m
}

func TestPanelWarnsWhenHeadNotOnOrigin(t *testing.T) {
	repo := domain.Repository{FullName: "owner/repo", LocalPath: "/tmp/repo"}
	data := cmds.CreatePRFormData{
		DefaultBase: "main", CurrentBranch: "feature",
		LocalBranches:  []string{"main", "feature"},
		RemoteBranches: []string{"origin/main"},
	}
	view := stripANSI(openPanel(t, repo, data).PanelView(80, 30))
	if !strings.Contains(view, "Not on origin yet — run git push -u origin feature") {
		t.Fatalf("expected unpushed warning, got:\n%s", view)
	}

	data.RemoteBranches = append(data.RemoteBranches, "origin/feature")
	if view := stripANSI(openPanel(t, repo, data).PanelView(80, 30)); strings.Contains(view, "Not on origin") {
		t.Fatalf("warning shown for a pushed branch:\n%s", view)
	}

	// Without a local checkout Submit doesn't check origin, so neither do we.
	data.RemoteBranches = []string{"origin/main"}
	if view := stripANSI(openPanel(t, domain.Repository{FullName: "owner/repo"}, data).PanelView(80, 30)); strings.Contains(view, "Not on origin") {
		t.Fatalf("warning shown without a local path:\n%s", view)
	}
}

func TestPanelRouteLine(t *testing.T) {
	data := cmds.CreatePRFormData{DefaultBase: "main", CurrentBranch: "feature", LocalBranches: []string{"main", "feature"}}
	view := stripANSI(openPanel(t, domain.Repository{FullName: "owner/repo"}, data).PanelView(80, 30))
	if !strings.Contains(view, "feature → main  ·  owner/repo") {
		t.Fatalf("route line missing:\n%s", view)
	}

	data.IsFork, data.ParentFullName = true, "upstream/repo"
	view = stripANSI(openPanel(t, domain.Repository{FullName: "me/repo"}, data).PanelView(80, 30))
	if !strings.Contains(view, "me/repo → upstream/repo") {
		t.Fatalf("fork target missing:\n%s", view)
	}
}

func TestSubmitButtonLabelFollowsDraft(t *testing.T) {
	draft := false
	b := &submitButton{draft: &draft}
	if got := stripANSI(b.View()); !strings.Contains(got, "Create pull request") || strings.Contains(got, "draft") {
		t.Fatalf("label = %q", got)
	}
	draft = true
	if got := stripANSI(b.View()); !strings.Contains(got, "Create draft pull request") {
		t.Fatalf("draft label = %q", got)
	}
}

// A long error (the push hint) wraps inside the panel instead of being cut.
func TestPanelErrorWraps(t *testing.T) {
	data := cmds.CreatePRFormData{DefaultBase: "main", CurrentBranch: "feature", LocalBranches: []string{"main", "feature"}}
	m := openPanel(t, domain.Repository{FullName: "owner/repo"}, data)
	msg := `branch "a-rather-long-feature-branch-name" has not been pushed to origin — run: git push -u origin a-rather-long-feature-branch-name`
	m.SetError(errors.New(msg))
	view := stripANSI(m.PanelView(60, 40))
	for i, l := range strings.Split(view, "\n") {
		if w := lipgloss.Width(l); w > 60 {
			t.Errorf("line %d width %d > 60: %q", i, w, l)
		}
	}
	flat := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(flat, "git push -u origin") || !strings.HasSuffix(flat, "feature-branch-name") {
		t.Fatalf("error text lost:\n%s", view)
	}
}

// In a short panel the fields drop their blank separators so the submit
// button stays visible; in a tall one they keep them.
func TestPanelCompactsWhenShort(t *testing.T) {
	data := cmds.CreatePRFormData{DefaultBase: "main", CurrentBranch: "feature", LocalBranches: []string{"main", "feature"}}
	m := openPanel(t, domain.Repository{FullName: "owner/repo"}, data)

	tall := stripANSI(m.PanelView(80, 40))
	if !strings.Contains(tall, "Create pull request    Ctrl+S") || !strings.Contains(tall, "Head branch") {
		t.Fatalf("tall panel missing content:\n%s", tall)
	}
	spaced := regexp.MustCompile(`main ›\s*\n\s*\n\s*Head branch`)
	if !spaced.MatchString(tall) {
		t.Fatalf("expected a blank line between fields in a tall panel:\n%s", tall)
	}

	short := stripANSI(m.PanelView(80, 18))
	if h := strings.Count(short, "\n") + 1; h > 18 {
		t.Fatalf("short panel is %d rows, want <= 18:\n%s", h, short)
	}
	if !strings.Contains(short, "Create pull request    Ctrl+S") {
		t.Fatalf("submit button cut off in short panel:\n%s", short)
	}
	if spaced.MatchString(short) {
		t.Fatalf("short panel kept blank lines between fields:\n%s", short)
	}

	// Growing again restores the spacing.
	if again := stripANSI(m.PanelView(80, 40)); again != tall {
		t.Fatalf("spacing not restored after growing:\n%s", again)
	}
}

// In the floating box the error wraps once, to the space inside the border
// and padding, so no word is left alone on a line by a second wrap.
func TestOverlayErrorWrapsOnce(t *testing.T) {
	data := cmds.CreatePRFormData{DefaultBase: "main", CurrentBranch: "feature", LocalBranches: []string{"main", "feature"}}
	m := openPanel(t, domain.Repository{FullName: "owner/repo"}, data)
	m.SetSize(80, 30)
	m.SetError(errors.New(`branch "a-rather-long-feature-branch-name" has not been pushed to origin — run: git push -u origin a-rather-long-feature-branch-name`))
	view := stripANSI(m.View())
	for _, l := range strings.Split(view, "\n") {
		inner := strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│"))
		if inner == "has" || inner == "origin" {
			t.Fatalf("error re-wrapped, stranded %q:\n%s", inner, view)
		}
	}
}
