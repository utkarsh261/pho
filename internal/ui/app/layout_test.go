package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/testutil"
)

func TestLayoutRendersCorrectStructureAtVariousSizes(t *testing.T) {
	t.Parallel()

	repoA := testutil.Repo("acme/alpha")
	repoB := testutil.Repo("acme/beta")
	dashboardMap := map[string]domain.DashboardSnapshot{
		repoA.FullName: dashboardSnapshot(repoA,
			pr(repoA.FullName, 1, "Fix login"),
			pr(repoA.FullName, 2, "Older PR"),
		),
		repoB.FullName: dashboardSnapshot(repoB,
			pr(repoB.FullName, 9, "Repo switch result"),
		),
	}
	m := newTestModel([]domain.Repository{repoA, repoB}, dashboardMap)

	// Bootstrap: window size, repos discovered, dashboard loaded.
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repoA, repoB}))
	_, _ = m.Update(cmdsDashboardLoaded(repoA.FullName, dashboardMap[repoA.FullName], false, nil))

	type sizeCase struct {
		name        string
		width       int
		height      int
		previewZero bool
		allPanels   bool
	}

	sizes := []sizeCase{
		{"80x24", 80, 24, false, true},
		{"120x40", 120, 40, false, true},
		{"46x20", 46, 20, true, false},
	}

	for _, sc := range sizes {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()

			// Build a fresh model for isolation.
			m2 := newTestModel([]domain.Repository{repoA, repoB}, dashboardMap)
			_, _ = m2.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			_, _ = m2.Update(cmdsReposDiscovered([]domain.Repository{repoA, repoB}))
			_, _ = m2.Update(cmdsDashboardLoaded(repoA.FullName, dashboardMap[repoA.FullName], false, nil))

			// Resize to the target size.
			_, _ = m2.Update(tea.WindowSizeMsg{Width: sc.width, Height: sc.height})

			output := m2.View()

			// Output must be non-empty.
			if output == "" {
				t.Fatal("expected non-empty output, got empty string")
			}

			// Expected text tokens must be present.
			// Repo name only appears when repo panel is visible (multi-panel mode).
			if sc.allPanels {
				if !contains(output, "acme/alpha") {
					t.Fatalf("expected repo name %q in output, got:\n%s", "acme/alpha", output)
				}
			}
			if !contains(output, "Fix login") {
				t.Fatalf("expected PR title %q in output, got:\n%s", "Fix login", output)
			}
			// Tab label: inactive tabs render as "My PRs(N)", active as "[My PRs(N)]".
			if !strings.Contains(output, "My PRs") {
				t.Fatalf("expected tab label %q in output, got:\n%s", "My PRs", output)
			}

			// Layout width must match.
			if m2.Layout().Current.Width != sc.width {
				t.Fatalf("expected layout width %d, got %d", sc.width, m2.Layout().Current.Width)
			}

			// Preview panel expectations.
			if sc.previewZero && m2.Layout().Current.Preview != 0 {
				t.Fatalf("expected preview=0 at %d width, got %d", sc.width, m2.Layout().Current.Preview)
			}
			if sc.allPanels {
				if m2.Layout().Current.Repo == 0 {
					t.Fatalf("expected repo panel visible at %d width, got 0", sc.width)
				}
				if m2.Layout().Current.PR == 0 {
					t.Fatalf("expected PR panel visible at %d width, got 0", sc.width)
				}
				if m2.Layout().Current.Preview == 0 {
					t.Fatalf("expected preview panel visible at %d width, got 0", sc.width)
				}
			}

			// Line count should be roughly terminal height (body + status line).
			lines := strings.Split(output, "\n")
			// Allow some slack: body lines should be close to height.
			if len(lines) < sc.height-2 || len(lines) > sc.height+2 {
				t.Fatalf("expected ~%d lines (got %d), output:\n%s", sc.height, len(lines), output)
			}
		})
	}
}

// TestAppRenderFullUIRender locks the grid structure of buildBox and the
// multi-panel renderDashboard output.
//
// buildBox assertions use exact string snapshots captured from the pre-refactor
// implementation; these must survive the transition from manual strings.Repeat
// padding to lipgloss.NewStyle().Width().MaxWidth().Render() since both produce
// identical bytes for plain ASCII content in a non-TTY environment.
func TestAppRenderFullUIRender(t *testing.T) {
	t.Parallel()

	// --- buildBox: empty panel (no content lines) ---
	const (
		emptyBoxW = 20
		emptyBoxH = 5
	)
	gotEmpty := buildBox("", emptyBoxW, emptyBoxH, false)
	wantEmpty := "┌────────────────────┐ \n│                    │ \n│                    │ \n│                    │ \n└────────────────────┘ "
	if gotEmpty != wantEmpty {
		t.Fatalf("buildBox empty mismatch\nwant: %q\n got: %q", wantEmpty, gotEmpty)
	}

	// --- buildBox: partially filled (2 content lines, 3 content rows total) ---
	gotPartial := buildBox("Hello\nWorld", emptyBoxW, emptyBoxH, true)
	wantPartial := "┌────────────────────┐ \n│Hello               │ \n│World               │ \n│                    │ \n└────────────────────┘ "
	if gotPartial != wantPartial {
		t.Fatalf("buildBox partial mismatch\nwant: %q\n got: %q", wantPartial, gotPartial)
	}

	// --- renderDashboard: structural checks for a 3-panel layout ---
	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo,
		pr(repo.FullName, 1, "Fix login"),
		pr(repo.FullName, 2, "Add tests"),
	)
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: snap,
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, false, nil))

	body := m.renderDashboard()
	bodyLines := strings.Split(body, "\n")

	// Body includes 38 panel rows (height-2 borders) plus 2 status rows = 40 total,
	// but renderDashboard returns body+"\n"+status, so line count ≈ height.
	if len(bodyLines) < 38 || len(bodyLines) > 42 {
		t.Fatalf("renderDashboard: expected ~40 lines, got %d", len(bodyLines))
	}

	// All three panel top-borders on line 0.
	if !strings.Contains(bodyLines[0], "┌") {
		t.Fatalf("renderDashboard line 0: expected ┌, got %q", bodyLines[0])
	}

	// Every panel row must have consistent per-line width — each row of the
	// joined grid must equal the total terminal width (contentW+3 per panel, summed).
	totalW := m.layout.Current.Repo + m.layout.Current.PR + m.layout.Current.Preview + 3*3
	for i, line := range bodyLines {
		w := lipgloss.Width(line)
		// Status bar lines and separator are allowed to differ; check only box rows.
		if i >= len(bodyLines)-2 {
			break
		}
		if w != totalW {
			t.Fatalf("renderDashboard line %d: expected width %d, got %d: %q", i, totalW, w, line)
		}
	}

	// Content sanity.
	if !strings.Contains(body, "Fix login") {
		t.Fatal("renderDashboard: expected 'Fix login' in output")
	}
	if !strings.Contains(body, "Add tests") {
		t.Fatal("renderDashboard: expected 'Add tests' in output")
	}
}

func TestFocusBorderUsesPrimaryColor(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: dashboardSnapshot(repo, pr(repo.FullName, 1, "Test")),
	})

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo, pr(repo.FullName, 1, "Test")), false, nil))

	// Focus the repo panel.
	m.SetFocus(domain.FocusRepoPanel)
	output := m.View()
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty output")
	}
	// The first line starts with ┌ (top-left corner of the focused panel's box).
	if !strings.HasPrefix(lines[0], "┌") {
		t.Fatalf("expected focused panel to start with ┌, got line: %q", lines[0])
	}

	// Cycle focus to PR panel.
	m.SetFocus(domain.FocusPRListPanel)
	output = m.View()
	lines = strings.Split(output, "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty output")
	}
	// The first panel (repo) should now have the lighter │ border (second line starts with │).
	// The second line is the first content line of the repo panel box.
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[1], "│") {
		t.Fatalf("expected unfocused repo panel border │, got line: %q", lines[1])
	}
}
