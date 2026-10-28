package prdetail

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/github/rest"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// sinceReviewState backs the Diff tab's "since your review" scope: the
// changes from the commit the viewer last reviewed to the current head.
type sinceReviewState struct {
	// active means m.Diff is the range diff; full holds the PR diff then.
	active bool
	full   *model.DiffModel

	// from/to is the range last asked for. rng is its result once loaded.
	from, to string
	rng      *model.RangeDiff
	loading  bool
	err      error
	// enterOnLoad switches to the range as soon as it lands (s was pressed
	// while it was loading).
	enterOnLoad bool
}

// lastReview is the viewer's latest submitted review of this PR.
func (m *PRDetailModel) lastReview() (domain.PreviewReviewer, bool) {
	if m.Detail == nil {
		return domain.PreviewReviewer{}, false
	}
	return domain.LastReview(m.Detail.Reviewers, m.ViewerLogin)
}

// HasLastReview reports whether the viewer has reviewed this PR, i.e.
// whether s has anything to compare against.
func (m *PRDetailModel) HasLastReview() bool {
	_, ok := m.lastReview()
	return ok && !m.CommitMode
}

// loadSinceReviewCmd fetches the range from the viewer's last review to the
// head unless it is already loaded or on its way. It is a no-op when the
// viewer hasn't reviewed or nothing was pushed since.
func (m *PRDetailModel) loadSinceReviewCmd() tea.Cmd {
	if m.CommitMode || m.PRService == nil {
		return nil
	}
	r, ok := m.lastReview()
	head := m.Summary.HeadRefOID
	if !ok || head == "" || r.CommitSHA == head {
		return nil
	}
	s := &m.since
	if s.from == r.CommitSHA && s.to == head && (s.loading || s.rng != nil) {
		return nil
	}
	s.from, s.to = r.CommitSHA, head
	s.rng, s.err, s.loading = nil, nil, true
	return cmds.LoadRangeDiffCmd(m.PRService, m.Repo, m.Summary.Number, s.from, s.to, false)
}

// syncSinceReviewCmd runs after each detail load: it leaves the scope when
// there is no longer anything to show (e.g. the viewer just reviewed the
// head) and fetches the range for the current review and head.
func (m *PRDetailModel) syncSinceReviewCmd() tea.Cmd {
	if m.since.active {
		if r, ok := m.lastReview(); !ok || r.CommitSHA == m.Summary.HeadRefOID {
			m.exitSinceReview()
		}
	}
	return m.loadSinceReviewCmd()
}

// hasNewSinceReview reports whether there are pushes since the viewer's last
// review that the since-review scope can show.
func (m *PRDetailModel) hasNewSinceReview() bool {
	rd := m.since.rng
	return rd != nil && !rd.Rewritten && len(rd.Diff.Files) > 0 && rd.ToSHA == m.Summary.HeadRefOID
}

// toggleSinceReview handles s: it switches the Diff tab between all changes
// and the changes since the viewer's last review.
func (m *PRDetailModel) toggleSinceReview() tea.Cmd {
	if m.since.active {
		m.exitSinceReview()
		m.diffNotice = "Showing all changes"
		return nil
	}
	if m.Detail == nil {
		m.diffNotice = "PR still loading"
		return nil
	}
	r, ok := m.lastReview()
	switch {
	case !ok:
		m.diffNotice = "You haven't reviewed this PR yet"
		return nil
	case r.CommitSHA == m.Summary.HeadRefOID:
		m.diffNotice = "No changes since your review"
		return nil
	}
	m.switchTab(TabDiff)
	cmd := m.loadSinceReviewCmd()
	if m.since.rng == nil {
		m.since.enterOnLoad = true
		m.diffNotice = "Loading changes since your review…"
		return cmd
	}
	m.enterSinceReview()
	return cmd
}

// enterSinceReview swaps the loaded range diff in, or explains why it can't.
func (m *PRDetailModel) enterSinceReview() {
	rd := m.since.rng
	switch {
	case rd == nil:
		return
	case rd.Rewritten:
		m.diffNotice = fmt.Sprintf("Branch was rebased since your review on %s — can't separate the new changes", shortSHA(rd.FromSHA))
		return
	case len(rd.Diff.Files) == 0:
		m.diffNotice = "No changes since your review"
		return
	}
	if !m.since.active {
		m.since.full = m.Diff
	}
	m.since.active = true
	m.leftPanel.ScopeLabel = "since review"
	m.setActiveDiff(&rd.Diff)

	commits := "1 commit"
	if rd.Commits != 1 {
		commits = fmt.Sprintf("%d commits", rd.Commits)
	}
	notice := fmt.Sprintf("Showing %s since your review on %s", commits, shortSHA(rd.FromSHA))
	if r, ok := m.lastReview(); ok && r.CommitSHA == rd.FromSHA && !r.SubmittedAt.IsZero() {
		notice += " (" + relativeTime(r.SubmittedAt) + ")"
	}
	if rd.BaseMerged {
		notice += " · includes a merge, so some changes may be from " + m.baseRefName()
	}
	m.diffNotice = notice + " · s: all changes"
}

// exitSinceReview restores the full PR diff. When the head moved while the
// scope was on, the PR diff may still be loading; DiffLoaded installs it.
func (m *PRDetailModel) exitSinceReview() {
	if !m.since.active {
		return
	}
	full := m.since.full
	m.since.active, m.since.full = false, nil
	m.leftPanel.ScopeLabel = ""
	if full != nil {
		m.setActiveDiff(full)
		return
	}
	m.Diff = nil
	m.leftPanel.Files = nil
}

// handleRangeDiffLoaded stores a range diff and, if asked for, shows it.
func (m *PRDetailModel) handleRangeDiffLoaded(msg cmds.RangeDiffLoaded) {
	s := &m.since
	if !m.matchesPR(msg.Host, msg.Repo, msg.Number) || msg.FromSHA != s.from || msg.ToSHA != s.to {
		return
	}
	s.loading = false
	enter := s.enterOnLoad || s.active
	s.enterOnLoad = false
	if msg.Err != nil {
		s.err = msg.Err
		if enter {
			m.exitSinceReview()
			m.diffNotice = sinceReviewErrNotice(msg.Err, msg.FromSHA)
		}
		return
	}
	rd := msg.Range
	s.rng = &rd
	if !enter {
		return
	}
	if rd.Rewritten || len(rd.Diff.Files) == 0 {
		m.exitSinceReview()
	}
	m.enterSinceReview()
}

func sinceReviewErrNotice(err error, from string) string {
	if errors.Is(err, rest.ErrNotFound) {
		return fmt.Sprintf("Your reviewed commit %s is no longer on GitHub", shortSHA(from))
	}
	return "Couldn't load changes since your review · s: retry"
}

// sinceReviewHidesDraft reports whether a draft can't be placed on the range
// diff: removed-line numbers there belong to the reviewed commit, not base.
func (m *PRDetailModel) sinceReviewHidesDraft(d domain.DraftInlineComment) bool {
	return m.since.active && (d.Side == "LEFT" || d.StartSide == "LEFT")
}

// sinceReviewCommentable reports whether the visual selection can take an
// inline comment. GitHub only accepts lines that are in the PR's own diff,
// so in the since-review scope both ends must be new-side lines that the
// full PR diff also has.
func (m *PRDetailModel) sinceReviewCommentable() bool {
	if !m.since.active {
		return true
	}
	if !m.validVisualState() || m.since.full == nil {
		return false
	}
	f := &m.Diff.Files[m.visual.FileIdx]
	h := &f.Hunks[m.visual.HunkIdx]
	for _, li := range []int{m.visual.StartLine, m.visual.EndLine} {
		path, line, side, ok := anchorForLine(f, &h.Lines[li])
		if !ok || side != "RIGHT" || !diffHasNewLine(m.since.full, path, line) {
			return false
		}
	}
	return true
}

// diffHasNewLine reports whether dm shows line of path on the new side.
func diffHasNewLine(dm *model.DiffModel, path string, line int) bool {
	for fi := range dm.Files {
		f := &dm.Files[fi]
		if f.NewPath != path {
			continue
		}
		for hi := range f.Hunks {
			for _, l := range f.Hunks[hi].Lines {
				if l.Kind != "deletion" && l.NewLine != nil && *l.NewLine == line {
					return true
				}
			}
		}
	}
	return false
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (m *PRDetailModel) baseRefName() string {
	if m.Summary.BaseRefName != "" {
		return m.Summary.BaseRefName
	}
	return "the base branch"
}

// sinceReviewPill renders the right end of the tab bar on the Diff tab: a
// nudge when there are pushes since the viewer's review, or the active
// scope. It returns the widest form that fits maxW, or "".
func (m *PRDetailModel) sinceReviewPill(maxW int) string {
	if m.activeTab != TabDiff || m.CommitMode {
		return ""
	}
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	sep := lipgloss.NewStyle().Foreground(th.Faint).Render(" · ")
	var forms []string
	if m.since.active && m.since.rng != nil {
		rd := m.since.rng
		scope := lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
		sha := shortSHA(rd.FromSHA)
		caveat := ""
		if rd.BaseMerged {
			caveat = sep + lipgloss.NewStyle().Foreground(th.Warning).Render("merged base")
		}
		back := th.HintKey.Render("s") + " " + th.HintDesc.Render("All")
		// Shrink by dropping the back hint, then "Since"; the caveat goes last.
		forms = []string{
			scope.Render("◆ Since "+sha) + caveat + sep + back,
			scope.Render("◆ Since "+sha) + caveat,
			scope.Render("◆ "+sha) + caveat,
			scope.Render("◆ " + sha),
		}
	} else if m.hasNewSinceReview() {
		desc := lipgloss.NewStyle().Foreground(th.Warning)
		n := m.since.rng.Commits
		forms = []string{
			th.HintKey.Render("s") + "  " + desc.Render(fmt.Sprintf("%d new since review", n)),
			th.HintKey.Render("s") + " " + desc.Render(fmt.Sprintf("%d new", n)),
		}
	}
	for _, f := range forms {
		if lipgloss.Width(f) <= maxW {
			return f
		}
	}
	return ""
}

// withSinceReviewPill right-aligns the since-review pill in a tab bar of
// width w, keeping a one-column right margin.
func (m *PRDetailModel) withSinceReviewPill(tabs string, w int) string {
	pill := m.sinceReviewPill(w - lipgloss.Width(tabs) - 3)
	if pill == "" {
		return tabs
	}
	gap := max(w-lipgloss.Width(tabs)-lipgloss.Width(pill)-1, 2)
	return tabs + strings.Repeat(" ", gap) + pill
}
