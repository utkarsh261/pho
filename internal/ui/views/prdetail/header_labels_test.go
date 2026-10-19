package prdetail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/domain"
)

func TestHeaderLabelsSitInBottomBorder(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(120, 30)
	m.Detail.Labels = []domain.Label{{Name: "bug", Color: "d73a4a"}, {Name: "area/diff", Color: "0e8a16"}}
	m.Detail.LabelCount = 2

	lines := strings.Split(m.renderHeader(), "\n")
	if len(lines) != 4 {
		t.Fatalf("header has %d lines, want 4", len(lines))
	}
	bottom := ansi.Strip(lines[3])
	if !strings.HasPrefix(bottom, "╰─") || !strings.HasSuffix(bottom, " ● bug  ● area/diff ─╯") {
		t.Errorf("bottom border = %q", bottom)
	}
}

func TestHeaderLabelsKeepHeightAndWidth(t *testing.T) {
	t.Parallel()
	many := make([]domain.Label, 20)
	for i := range many {
		many[i] = domain.Label{Name: "label-number-" + strings.Repeat("x", i%7), Color: "abcdef"}
	}
	for _, labels := range [][]domain.Label{nil, many[:1], many} {
		for w := 10; w <= 160; w++ {
			m := makeReviewersModel(w, 30)
			m.Detail.Labels = labels
			m.Detail.LabelCount = len(labels) + 5
			header := m.renderHeader()
			if h := lipgloss.Height(header); h != 4 {
				t.Fatalf("width %d, %d labels: height %d, want 4", w, len(labels), h)
			}
			for i, line := range strings.Split(header, "\n") {
				if lw := lipgloss.Width(line); lw != w {
					t.Fatalf("width %d, %d labels: line %d is %d wide: %q", w, len(labels), i, lw, ansi.Strip(line))
				}
			}
		}
	}
}

func TestHeaderWithoutDetailHasPlainBottomBorder(t *testing.T) {
	t.Parallel()
	m := makeReviewersModel(60, 30)
	m.Detail = nil
	lines := strings.Split(m.renderHeader(), "\n")
	if got := ansi.Strip(lines[len(lines)-1]); got != "╰"+strings.Repeat("─", 58)+"╯" {
		t.Errorf("bottom border = %q", got)
	}
}
