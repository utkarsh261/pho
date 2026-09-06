package prdetail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/ui/theme"
)

const (
	reviewerStateApproved  = "APPROVED"
	reviewerStateChanges   = "CHANGES_REQUESTED"
	reviewerStateCommented = "COMMENTED"
)

// reviewerBadge is one user in the header strip with the state of their
// latest submitted review.
type reviewerBadge struct {
	login string
	state string
}

type reviewerEntry struct {
	state string
	ts    time.Time
}

// reviewerSummaries collapses Detail.Reviewers (one entry per submitted
// review) into one badge per user — latest review wins — and adds users who
// only left PR-level comments as COMMENTED. Ordered approved → changes
// requested → commented, most recent first within each group.
func (m *PRDetailModel) reviewerSummaries() []reviewerBadge {
	if m.Detail == nil {
		return nil
	}
	byLogin := make(map[string]*reviewerEntry)

	for _, r := range m.Detail.Reviewers {
		if r.Login == "" || r.State == "PENDING" {
			continue
		}
		state := normalizeReviewerState(r.State)
		if e, ok := byLogin[r.Login]; ok {
			// Latest wins; on equal timestamps the later slice entry wins.
			if !r.SubmittedAt.Before(e.ts) {
				e.state = state
				e.ts = r.SubmittedAt
			}
			continue
		}
		byLogin[r.Login] = &reviewerEntry{state: state, ts: r.SubmittedAt}
	}

	for _, c := range m.Detail.Comments {
		if c.Login == "" {
			continue
		}
		if _, ok := byLogin[c.Login]; ok {
			continue
		}
		byLogin[c.Login] = &reviewerEntry{state: reviewerStateCommented, ts: c.CreatedAt}
	}

	if len(byLogin) == 0 {
		return nil
	}

	rank := func(state string) int {
		switch state {
		case reviewerStateApproved:
			return 0
		case reviewerStateChanges:
			return 1
		default:
			return 2
		}
	}

	badges := make([]reviewerBadge, 0, len(byLogin))
	for login, e := range byLogin {
		badges = append(badges, reviewerBadge{login: login, state: e.state})
	}
	sort.Slice(badges, func(i, j int) bool {
		a, b := badges[i], badges[j]
		if ra, rb := rank(a.state), rank(b.state); ra != rb {
			return ra < rb
		}
		ta, tb := byLogin[a.login].ts, byLogin[b.login].ts
		if !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.login < b.login
	})
	return badges
}

// normalizeReviewerState maps a GitHub review state onto one of the three
// strip states.
func normalizeReviewerState(state string) string {
	switch state {
	case reviewerStateApproved, reviewerStateChanges:
		return state
	default:
		return reviewerStateCommented
	}
}

// renderReviewerStrip renders the "Reviewers  ✓ @alice  ! @dave" header line,
// dropping tail badges (with a "+N") until it fits width. Returns "" when
// there is nothing to show.
func (m *PRDetailModel) renderReviewerStrip(width int) string {
	badges := m.reviewerSummaries()
	if len(badges) == 0 || width <= 0 {
		return ""
	}
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	label := th.MutedTxt.Render("Reviewers")
	segments := make([]string, len(badges))
	for i, b := range badges {
		segments[i] = renderReviewerBadge(th, b)
	}
	for shown := len(segments); shown >= 0; shown-- {
		parts := segments[:shown:shown]
		if shown < len(segments) {
			parts = append(parts, th.MutedTxt.Render(fmt.Sprintf("+%d", len(segments)-shown)))
		}
		line := label + "  " + strings.Join(parts, "  ")
		if lipgloss.Width(line) <= width {
			return line
		}
	}
	return ""
}

func renderReviewerBadge(th *theme.Theme, b reviewerBadge) string {
	var icon string
	switch b.state {
	case reviewerStateApproved:
		icon = th.ReviewApproved.Render("✓")
	case reviewerStateChanges:
		icon = th.ReviewChanges.Render("!")
	default:
		icon = th.ReviewMuted.Render("·")
	}
	return icon + " " + th.PrimaryTxt.Render("@"+b.login)
}
