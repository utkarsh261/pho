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
// renderHeaderReviewers renders the reviewer strip for the header's meta line,
// where space is shared with author and state. It prefers showing at least one
// badge: when the labelled strip can only fit "Reviewers  +N", it drops the
// label instead, and only falls back to the count when no badge fits at all.
func (m *PRDetailModel) renderHeaderReviewers(width int) string {
	line, shown := m.reviewerStrip(width, true)
	if shown == 0 {
		if bare, n := m.reviewerStrip(width, false); n > 0 {
			return bare
		}
	}
	return line
}

// reviewerStrip renders as many reviewer badges as fit in width, with a "+N"
// overflow marker and an optional "Reviewers" label. It returns the line and
// how many badges it shows; "" when nothing fits.
func (m *PRDetailModel) reviewerStrip(width int, withLabel bool) (string, int) {
	badges := m.reviewerSummaries()
	if len(badges) == 0 || width <= 0 {
		return "", 0
	}
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	prefix := ""
	if withLabel {
		prefix = th.MutedTxt.Render("Reviewers") + "  "
	}
	segments := make([]string, len(badges))
	for i, b := range badges {
		segments[i] = renderReviewerBadge(th, b)
	}
	for shown := len(segments); shown >= 0; shown-- {
		parts := segments[:shown:shown]
		if shown < len(segments) {
			parts = append(parts, th.MutedTxt.Render(fmt.Sprintf("+%d", len(segments)-shown)))
		}
		line := prefix + strings.Join(parts, "  ")
		if lipgloss.Width(line) <= width {
			return line, shown
		}
	}
	return "", 0
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
