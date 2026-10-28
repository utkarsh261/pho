// Package stackmap draws a stack of PRs top-first, trunk at the bottom, with
// each PR's CI and review state. The dashboard preview and the stack picker
// share it.
package stackmap

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// Options controls how Rows lays out a stack.
type Options struct {
	Width   int
	Current int  // the PR being looked at, drawn as a filled node
	Cursor  int  // member index to highlight as selected; -1 for none
	Indent  bool // indent side branches of forked stacks
	Window  int  // members kept either side of the focus in long stacks; 0 keeps all
}

// Header is the stack's title line: "Stack · 3 PRs onto main".
func Header(st *domain.Stack) string {
	return fmt.Sprintf("Stack · %d PRs onto %s", len(st.Members), st.Trunk)
}

// Rows renders one line per member, top of the stack first, then the trunk.
func Rows(th *theme.Theme, st *domain.Stack, opt Options) []string {
	if th == nil {
		th = theme.Default()
	}
	focus := opt.Cursor
	if focus < 0 {
		focus = st.IndexOf(opt.Current)
	}
	from, to := 0, len(st.Members)
	if opt.Window > 0 && len(st.Members) > 2*opt.Window+1 {
		from = max(0, min(focus-opt.Window, len(st.Members)-2*opt.Window-1))
		to = from + 2*opt.Window + 1
	}

	var lines []string
	if from > 0 {
		lines = append(lines, th.FaintTxt.Render(fmt.Sprintf("  ⋮ %d more", from)))
	}
	for i := from; i < to; i++ {
		lines = append(lines, row(th, st.Members[i], opt, i == opt.Cursor))
	}
	if rest := len(st.Members) - to; rest > 0 {
		lines = append(lines, th.FaintTxt.Render(fmt.Sprintf("  ⋮ %d more", rest)))
	}
	trunk := "  " + th.FaintTxt.Render("╰ "+st.Trunk)
	if tag := landingTag(th, st); tag != "" {
		trunk += th.FaintTxt.Render("  ·  ") + tag
	}
	return append(lines, truncate(trunk, opt.Width))
}

func row(th *theme.Theme, mem domain.StackMember, opt Options, selected bool) string {
	edge := " "
	if selected {
		edge = th.PrimaryTxt.Render("▎")
	}
	indent := ""
	if opt.Indent {
		indent = strings.Repeat("  ", mem.Indent)
	}
	current := mem.Number == opt.Current
	node := th.FaintTxt.Render("○")
	num := th.MutedTxt.Render(fmt.Sprintf("#%d", mem.Number))
	title := lipgloss.NewStyle().Foreground(th.Text)
	switch {
	case current:
		node = th.PrimaryTxt.Render("●")
		num = th.Number.Render(fmt.Sprintf("#%d", mem.Number))
		title = title.Bold(true).Foreground(th.TextBright)
	case mem.State == domain.PRStateMerged:
		node = lipgloss.NewStyle().Foreground(th.StateMerged).Render("✓")
		title = title.Foreground(th.TextDim)
	case selected:
		title = title.Foreground(th.TextBright)
	}

	var meta string
	switch {
	case mem.State == domain.PRStateMerged:
		meta = lipgloss.NewStyle().Foreground(th.StateMerged).Render("merged")
	case mem.State == domain.PRStateClosed:
		meta = lipgloss.NewStyle().Foreground(th.StateClosed).Render("closed")
	case mem.Summary == nil:
		meta = th.FaintTxt.Render("not loaded")
	default:
		meta = CIIcon(th, mem.Summary.CIStatus) + " " + ReviewIcon(th, mem.Summary.ReviewDecision, mem.Summary.IsDraft)
	}

	left := edge + " " + indent + node + " " + num + " "
	titleMax := max(opt.Width-lipgloss.Width(left)-lipgloss.Width(meta)-2, 1)
	line := left + title.Render(truncate(mem.Title, titleMax))
	gap := max(opt.Width-lipgloss.Width(line)-lipgloss.Width(meta)-1, 1)
	line += strings.Repeat(" ", gap) + meta + " "
	if selected {
		line = th.FillRow(th.Highlight, opt.Width, line)
	}
	return line
}

// landingTag says whether the stack's lowest open PR can land, since every
// PR above waits on it: "#126 next to land" or "#126 blocks 2 above".
func landingTag(th *theme.Theme, st *domain.Stack) string {
	lowest := -1
	for i := len(st.Members) - 1; i >= 0; i-- {
		if st.Members[i].State == domain.PRStateOpen || st.Members[i].State == "" {
			lowest = i
			break
		}
	}
	if lowest < 0 || st.Members[lowest].Summary == nil {
		return ""
	}
	mem := st.Members[lowest]
	s := mem.Summary
	if !s.IsDraft && s.CIStatus == domain.CIStatusSuccess && s.ReviewDecision == domain.ReviewDecisionApproved {
		return th.CISuccess.Render(fmt.Sprintf("#%d next to land", mem.Number))
	}
	above := 0
	for _, m := range st.Members[:lowest] {
		if m.State == domain.PRStateOpen || m.State == "" {
			above++
		}
	}
	if above == 0 {
		return ""
	}
	return th.CIPending.Render(fmt.Sprintf("#%d blocks %d above", mem.Number, above))
}

// CIIcon is a PR's CI state as a coloured glyph.
func CIIcon(th *theme.Theme, status domain.CIStatus) string {
	switch status {
	case domain.CIStatusSuccess:
		return th.CISuccess.Render("✓")
	case domain.CIStatusFailure, domain.CIStatusError:
		return th.CIFailure.Render("!")
	case domain.CIStatusPending:
		return th.CIPending.Render("…")
	default:
		return th.CIMuted.Render("·")
	}
}

// ReviewIcon is a PR's review decision as a coloured glyph; D for drafts.
func ReviewIcon(th *theme.Theme, decision domain.ReviewDecision, isDraft bool) string {
	if isDraft {
		return th.ReviewDraft.Render("D")
	}
	switch decision {
	case domain.ReviewDecisionApproved:
		return th.ReviewApproved.Render("✓")
	case domain.ReviewDecisionChangesRequested:
		return th.ReviewChanges.Render("!")
	case domain.ReviewDecisionReviewRequired:
		return th.ReviewRequired.Render("?")
	default:
		return th.ReviewMuted.Render("·")
	}
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(s) + "…"
}
