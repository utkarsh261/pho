package layout

import tea "github.com/charmbracelet/bubbletea"

const (
	minRepoWidth    = 16
	minPRWidth      = 30
	minPreviewWidth = 20

	// Each panel needs 2 chars for left+right borders.
	// Each panel also gets 1 char right padding (for symmetry with left side).
	// Gaps between panels add 1 char each (but this is covered by the right padding).
	panelBorderOverhead = 2
	panelRightPad       = 1

	// 3 panels: 3*2 borders + 3*1 right-pad = 9 overhead.
	// 2 panels: 2*2 borders + 2*1 right-pad = 6 overhead.
	// 1 panel: 1*2 borders + 1*1 right-pad = 3 overhead.
	threePaneMinWidth = minRepoWidth + minPRWidth + minPreviewWidth + 3*panelBorderOverhead + 3*panelRightPad
	twoPaneMinWidth   = minRepoWidth + minPRWidth + 2*panelBorderOverhead + 2*panelRightPad
)

type DashboardLayout struct {
	Width   int
	Height  int
	Repo    int
	PR      int
	Preview int
}

type LayoutState struct {
	Width   int
	Height  int
	Current DashboardLayout
}

// NewLayoutState builds a layout state for the provided terminal size.
func NewLayoutState(width, height int) LayoutState {
	return LayoutState{}.SetSize(width, height)
}

// SetSize recalculates the dashboard layout for a terminal size.
func (s LayoutState) SetSize(width, height int) LayoutState {
	s.Width = width
	s.Height = height
	s.Current = Calculate(width, height)
	return s
}

func (s LayoutState) Update(msg tea.Msg) LayoutState {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return s.SetSize(m.Width, m.Height)
	default:
		return s
	}
}

// Calculate derives panel widths for the dashboard body.
// Widths returned are content widths (excluding borders and gaps).
func Calculate(width, height int) DashboardLayout {
	layout := DashboardLayout{
		Width:  max(width, 0),
		Height: max(height, 0),
	}

	if width <= 0 || height <= 0 {
		return layout
	}

	// Calculate available content width after accounting for borders and right-padding.
	switch {
	case width >= threePaneMinWidth:
		available := width - 3*panelBorderOverhead - 3*panelRightPad
		widths := proportionalWidths(available, []panelSpec{
			{name: "repo", min: minRepoWidth},
			{name: "pr", min: minPRWidth},
			{name: "preview", min: minPreviewWidth},
		})
		layout.Repo = widths[0]
		layout.PR = widths[1]
		layout.Preview = widths[2]
	case width >= twoPaneMinWidth:
		available := width - 2*panelBorderOverhead - 2*panelRightPad
		widths := proportionalWidths(available, []panelSpec{
			{name: "repo", min: minRepoWidth},
			{name: "pr", min: minPRWidth},
		})
		layout.Repo = widths[0]
		layout.PR = widths[1]
		layout.Preview = 0
	default:
		// Single panel: borders on left+right, plus right padding.
		available := width - panelBorderOverhead - panelRightPad
		layout.Repo = 0
		layout.PR = available
		layout.Preview = 0
	}

	return layout
}

type panelSpec struct {
	name string
	min  int
}

func proportionalWidths(total int, specs []panelSpec) []int {
	widths := make([]int, len(specs))
	if total <= 0 || len(specs) == 0 {
		return widths
	}

	sum := 0
	for _, spec := range specs {
		sum += spec.min
	}
	if sum == 0 {
		widths[len(widths)-1] = total
		return widths
	}

	type remainder struct {
		index int
		value int
	}

	remainders := make([]remainder, len(specs))
	allocated := 0
	for i, spec := range specs {
		product := total * spec.min
		widths[i] = product / sum
		allocated += widths[i]
		remainders[i] = remainder{
			index: i,
			value: product % sum,
		}
	}

	leftover := total - allocated
	for leftover > 0 {
		best := 0
		for i := 1; i < len(remainders); i++ {
			if remainders[i].value > remainders[best].value {
				best = i
				continue
			}
			if remainders[i].value == remainders[best].value && remainders[i].index < remainders[best].index {
				best = i
			}
		}
		widths[remainders[best].index]++
		remainders[best].value = -1
		leftover--
	}

	return widths
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
