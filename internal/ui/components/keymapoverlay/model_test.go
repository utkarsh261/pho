package keymapoverlay

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

func TestModel_ToggleVisible(t *testing.T) {
	m := NewModel()
	if m.Visible {
		t.Fatal("expected overlay to start hidden")
	}
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	if !m.Visible {
		t.Fatal("expected overlay to be visible")
	}
}

func TestModel_QClosesOverlay(t *testing.T) {
	m := NewModel()
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.Visible {
		t.Fatal("expected q to close overlay")
	}
}

func TestModel_EscClosesOverlay(t *testing.T) {
	m := NewModel()
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.Visible {
		t.Fatal("expected esc to close overlay")
	}
}

func TestModel_QuestionMarkClosesOverlay(t *testing.T) {
	m := NewModel()
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.Visible {
		t.Fatal("expected ? to close overlay")
	}
}

func TestModel_ViewRendersTitle(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	view := m.View()
	if !strings.Contains(view, "Keybindings") {
		t.Fatalf("expected 'Keybindings' title in view, got:\n%s", view)
	}
}

func TestModel_ViewRendersBindings(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 40
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	view := m.View()
	assertContains(t, view, "j/k")
	assertContains(t, view, "Move up/down")
	assertContains(t, view, "PR List")
	assertContains(t, view, "Global")
	assertContains(t, view, "Jump to PR/Repo")
}

func TestModel_ViewRendersFooter(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 40
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	view := m.View()
	assertContains(t, view, "? / esc / q to close")
}

func TestModel_ViewOverPreservesBackground(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	// Build a background with exactly terminal-height lines so ViewOver compositing works.
	bgLines := make([]string, 24)
	for i := range bgLines {
		bgLines[i] = fmt.Sprintf("background line %d", i+1)
	}
	bg := strings.Join(bgLines, "\n")
	view := m.ViewOver(bg)
	// Box content should appear.
	assertContains(t, view, "Keybindings")
	// At least some background lines outside the box should remain.
	if !strings.Contains(view, "background line 1") && !strings.Contains(view, "background line 24") {
		t.Fatal("expected at least one background edge line to remain")
	}
}

func TestModel_ThemedViewUsesRoundedBorder(t *testing.T) {
	m := NewModel()
	m.SetTheme(theme.Default())
	m.width = 80
	m.height = 24
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	view := m.View()
	assertContains(t, view, "╭")
	assertContains(t, view, "╰")
}

func TestBuildBindings_DashboardFocusesFirst(t *testing.T) {
	groups := BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusRepoPanel})
	if len(groups) == 0 {
		t.Fatal("expected groups")
	}
	if groups[0].Name != "Navigation" {
		t.Fatalf("expected first group Navigation, got %q", groups[0].Name)
	}
	// Repo Panel group should exist.
	found := false
	for _, g := range groups {
		if g.Name == "Repo Panel" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected Repo Panel group")
	}
}

func TestBuildBindings_PRDetailDiffTab(t *testing.T) {
	groups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDiff})
	found := false
	for _, g := range groups {
		if g.Name == "Diff" {
			found = true
			// Should contain visual mode.
			hasVisual := false
			for _, b := range g.Bindings {
				if b.Key == "space" {
					hasVisual = true
				}
			}
			if !hasVisual {
				t.Fatal("expected Diff group to contain space binding")
			}
		}
	}
	if !found {
		t.Fatal("expected Diff group for PR detail diff tab")
	}
}

func TestBuildBindings_PRDetailCommentsTab(t *testing.T) {
	groups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabComments})
	found := false
	for _, g := range groups {
		if g.Name == "Comments" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected Comments group for PR detail comments tab")
	}
}

func TestBuildBindings_PRDetailDescriptionTabOmitsDiff(t *testing.T) {
	groups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDescription})
	for _, g := range groups {
		if g.Name == "Diff" || g.Name == "Comments" {
			t.Fatalf("expected no %q group for Description tab", g.Name)
		}
	}
}

func TestBuildBindings_PRDetailHasSearchGroup(t *testing.T) {
	for _, tab := range []prdetail.ContentTab{prdetail.TabDescription, prdetail.TabDiff, prdetail.TabComments} {
		groups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: tab})
		found := false
		for _, g := range groups {
			if g.Name == "Search" {
				found = true
				hasSlash := false
				for _, b := range g.Bindings {
					if b.Key == "/" {
						hasSlash = true
					}
				}
				if !hasSlash {
					t.Fatalf("expected Search group to contain / binding for tab %d", tab)
				}
			}
		}
		if !found {
			t.Fatalf("expected Search group for tab %d", tab)
		}
	}
}

func TestBuildBindings_PRDetailHasDiscardDrafts(t *testing.T) {
	groups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDiff, DraftCount: 3})
	for _, g := range groups {
		if g.Name == "Actions" {
			found := false
			for _, b := range g.Bindings {
				if b.Key == "D" && b.Description == "Discard all drafts" {
					found = true
				}
			}
			if !found {
				t.Fatal("expected Actions group to contain D: Discard all drafts")
			}
		}
	}
}

func TestBuildBindings_PRDetailOmitsDiscardWhenNoDrafts(t *testing.T) {
	groups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDiff, DraftCount: 0})
	for _, g := range groups {
		if g.Name == "Actions" {
			for _, b := range g.Bindings {
				if b.Key == "D" {
					t.Fatal("expected Actions group to omit D when no drafts")
				}
			}
		}
	}
}

func TestBuildBindings_PRDetailContextualX(t *testing.T) {
	openGroups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDiff, PRState: domain.PRStateOpen})
	closedGroups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDiff, PRState: domain.PRStateClosed})
	mergedGroups := BuildBindings(Context{View: domain.PrimaryViewPRDetail, Focus: domain.FocusPRDetail, Tab: prdetail.TabDiff, PRState: domain.PRStateMerged})

	for _, g := range openGroups {
		if g.Name == "Actions" {
			found := false
			for _, b := range g.Bindings {
				if b.Key == "x" && b.Description == "Close" {
					found = true
				}
			}
			if !found {
				t.Fatal("expected x: Close for open PR")
			}
		}
	}

	for _, g := range closedGroups {
		if g.Name == "Actions" {
			found := false
			for _, b := range g.Bindings {
				if b.Key == "x" && b.Description == "Reopen" {
					found = true
				}
			}
			if !found {
				t.Fatal("expected x: Reopen for closed PR")
			}
		}
	}

	for _, g := range mergedGroups {
		if g.Name == "Actions" {
			for _, b := range g.Bindings {
				if b.Key == "x" {
					t.Fatal("expected no x binding for merged PR")
				}
			}
		}
	}
}

func TestModel_BoxFitsContent(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24
	m.Visible = true
	m.Groups = BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	boxW, boxH := m.boxSize()
	if boxW <= 0 || boxH <= 0 {
		t.Fatalf("expected positive box size, got %dx%d", boxW, boxH)
	}
	if boxW > m.width {
		t.Fatalf("box width %d exceeds terminal width %d", boxW, m.width)
	}
	if boxH > m.height {
		t.Fatalf("box height %d exceeds terminal height %d", boxH, m.height)
	}
}

func TestBuildBindings_DashboardActionsIncludeCreatePR(t *testing.T) {
	t.Parallel()

	groups := BuildBindings(Context{View: domain.PrimaryViewDashboard, Focus: domain.FocusPRListPanel})
	found := false
	for _, g := range groups {
		if g.Name == "Actions" {
			for _, b := range g.Bindings {
				if b.Key == "n" && b.Description == "Create pull request" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected Actions group to contain n: Create pull request")
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q in output, got:\n%s", want, got)
	}
}

func TestOverlay_MergeBindingIsShiftM(t *testing.T) {
	groups := BuildBindings(Context{
		View:    domain.PrimaryViewPRDetail,
		Focus:   domain.FocusPRDetail,
		Tab:     prdetail.TabDiff,
		PRState: domain.PRStateOpen,
	})

	for _, g := range groups {
		if g.Name == "Actions" {
			for _, b := range g.Bindings {
				if b.Key == "m" && b.Description == "Merge" {
					t.Fatal("expected m: Merge to be removed, found in Actions group")
				}
			}
			found := false
			for _, b := range g.Bindings {
				if b.Key == "M" && b.Description == "Merge" {
					found = true
				}
			}
			if !found {
				t.Fatal("expected M: Merge in Actions group")
			}
		}
	}
}

func TestOverlay_ResolveBindingInCommentsTab(t *testing.T) {
	groups := BuildBindings(Context{
		View:    domain.PrimaryViewPRDetail,
		Focus:   domain.FocusPRDetail,
		Tab:     prdetail.TabComments,
		PRState: domain.PRStateOpen,
	})

	found := false
	for _, g := range groups {
		if g.Name == "Comments" {
			for _, b := range g.Bindings {
				if b.Key == "m" && b.Description == "Resolve/Unresolve" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected m: Resolve/Unresolve in Comments group")
	}
}

func TestOverlay_ResolveBindingNotInOtherTabs(t *testing.T) {
	for _, tab := range []prdetail.ContentTab{prdetail.TabDiff, prdetail.TabDescription, prdetail.TabCommits} {
		groups := BuildBindings(Context{
			View:    domain.PrimaryViewPRDetail,
			Focus:   domain.FocusPRDetail,
			Tab:     tab,
			PRState: domain.PRStateOpen,
		})
		for _, g := range groups {
			for _, b := range g.Bindings {
				if b.Key == "m" && b.Description == "Resolve/Unresolve" {
					t.Fatalf("expected m: Resolve/Unresolve NOT in tab %v", tab)
				}
			}
		}
	}
}

func TestOverlay_JumpUnresolvedBindingInComments(t *testing.T) {
	groups := BuildBindings(Context{
		View:    domain.PrimaryViewPRDetail,
		Focus:   domain.FocusPRDetail,
		Tab:     prdetail.TabComments,
		PRState: domain.PRStateOpen,
	})

	found := false
	for _, g := range groups {
		if g.Name == "Comments" {
			for _, b := range g.Bindings {
				if b.Key == "[/]" && b.Description == "Prev/Next unresolved" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected [/: Prev/Next unresolved in Comments group")
	}
}
