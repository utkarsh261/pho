package app

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utk/git-term/internal/domain"
	"github.com/utk/git-term/internal/testutil"
)

// truncatedANSI matches an incomplete ANSI escape sequence at the end of a line,
// e.g. "\x1b[31" or "\x1b[1;32;40" without a trailing 'm'.
var truncatedANSI = regexp.MustCompile(`\x1b\[[0-9;]*$`)

func TestOutputHasNoTruncatedANSIEscapeSequences(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{
		repo.FullName: dashboardSnapshot(repo,
			pr(repo.FullName, 1, "Fix login"),
			pr(repo.FullName, 2, "Add tests"),
		),
	})

	// Set terminal size.
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Bootstrap discovery and dashboard load.
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, dashboardSnapshot(repo,
		pr(repo.FullName, 1, "Fix login"),
		pr(repo.FullName, 2, "Add tests"),
	), false, nil))

	output := m.View()
	if output == "" {
		t.Fatal("expected non-empty view output")
	}

	lines := strings.Split(output, "\n")

	for i, line := range lines {
		// No line should end with an incomplete ANSI escape sequence.
		if truncatedANSI.MatchString(line) {
			t.Errorf("line %d ends with truncated ANSI escape sequence: %q", i, line)
		}

		// For non-status lines (all but the last), verify lipgloss.Width
		// matches expected width. The status bar line is the final line and
		// may be padded to terminal width by fitLine, so we skip it.
		if i < len(lines)-1 {
			w := lipgloss.Width(line)
			layoutW := m.Layout().Current.Width
			if w > layoutW {
				t.Errorf("line %d has visible width %d exceeding layout width %d: %q",
					i, w, layoutW, line)
			}
		}
	}
}
