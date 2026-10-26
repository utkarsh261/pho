package prdetail

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// logLineKind classifies a GitHub Actions log line for highlighting.
type logLineKind int

const (
	logPlain logLineKind = iota
	logError
	logWarning
	logGroup   // a step or ##[group] header
	logCommand // a ##[command] echo
	logOmitted // the "earlier lines omitted" marker
)

type logLine struct {
	text string
	kind logLineKind
}

// errorContextLines is how many lines are kept above an error jumped to.
const errorContextLines = 3

// ciLogState is the job log shown in the right pane in place of the tabs.
type ciLogState struct {
	active     bool
	check      domain.PreviewCheckRow
	jobID      int64
	loading    bool
	err        error
	lines      []logLine
	errorLines []int // indices into lines, ascending
	errCursor  int   // index into errorLines; -1 before the first jump
	scroll     int
	requestID  uint64 // matches the CILogLoaded this view is waiting for
	// superseded is set when a refresh shows a newer job for this check
	// (after a re-run); the log shown is then the old job's.
	superseded bool
	// gone is set when the check left the refreshed list entirely; Enter
	// must not then load whatever check now sits under CICursor.
	gone bool
}

// ciRerunState is a job re-run being confirmed or requested. It is separate
// from ciLogState because a re-run can start from the sidebar and outlive
// the log view.
type ciRerunState struct {
	confirm  bool
	check    domain.PreviewCheckRow
	jobID    int64
	inFlight bool
	notice   string
	// noticeID tells a clear tick for an older notice to leave this one.
	noticeID uint64
}

const (
	// ciNoticeTTL is how long a re-run notice stays in the status bar.
	ciNoticeTTL = 4 * time.Second
	// ciRefreshDelay gives GitHub time to mark a re-run job as queued
	// before the checks are reloaded.
	ciRefreshDelay = 3 * time.Second
)

// actionsTimestamp is the RFC3339 timestamp Actions puts before every line.
var actionsTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z ?`)

// errorPrefixes mark lines that report a failure even without ##[error].
var errorPrefixes = []string{
	"Error:", "error:", "error[", "ERROR:", "ERROR ", "FAIL", "--- FAIL", "panic:", "fatal:", "Fatal:", "npm ERR!", "make: ***",
}

// parseLogLine cleans a raw Actions log line and classifies it. ok is false
// for lines that only carry markup (##[endgroup]) and should be hidden.
func parseLogLine(raw string) (line logLine, ok bool) {
	s := strings.TrimPrefix(raw, "\xef\xbb\xbf")
	s = actionsTimestamp.ReplaceAllString(s, "")
	s = sanitizeLogText(ansi.Strip(s))

	if marker, rest, found := cutMarker(s); found {
		switch marker {
		case "endgroup":
			return logLine{}, false
		case "group":
			return logLine{text: "▸ " + rest, kind: logGroup}, true
		case "error":
			return logLine{text: rest, kind: logError}, true
		case "warning":
			return logLine{text: rest, kind: logWarning}, true
		case "command":
			return logLine{text: rest, kind: logCommand}, true
		default: // notice, debug, section
			return logLine{text: rest}, true
		}
	}

	// The runner echoes commands it runs itself as "[command]..." (no ##).
	if rest, found := strings.CutPrefix(s, "[command]"); found {
		return logLine{text: "$ " + rest, kind: logCommand}, true
	}

	trimmed := strings.TrimLeft(s, " ")
	for _, p := range errorPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return logLine{text: s, kind: logError}, true
		}
	}
	return logLine{text: s}, true
}

// sanitizeLogText makes log text safe to draw. Progress output redraws a
// line with carriage returns, so only the text after the last one is kept, as
// a terminal would show it. Tabs become spaces and every other control
// character is dropped, so nothing in a workflow's output can move the cursor
// or hide text.
func sanitizeLogText(s string) string {
	s = strings.TrimRight(s, "\r")
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// cutMarker splits a "##[name]rest" workflow-command line.
func cutMarker(s string) (marker, rest string, ok bool) {
	if !strings.HasPrefix(s, "##[") {
		return "", "", false
	}
	end := strings.IndexByte(s, ']')
	if end < 0 {
		return "", "", false
	}
	return s[3:end], s[end+1:], true
}

// parseJobLog turns a log tail into display lines plus the error index.
func parseJobLog(log domain.JobLog) (lines []logLine, errorLines []int) {
	lines = make([]logLine, 0, len(log.Lines)+1)
	if log.Truncated {
		lines = append(lines, logLine{text: "… earlier lines omitted", kind: logOmitted})
	}
	for _, raw := range log.Lines {
		l, ok := parseLogLine(raw)
		if !ok {
			continue
		}
		if l.kind == logError {
			errorLines = append(errorLines, len(lines))
		}
		lines = append(lines, l)
	}
	return lines, errorLines
}

// selectedActionsCheck returns the highlighted check and its Actions job ID.
// ok is false when the cursor is off the list or the check is not from
// GitHub Actions.
func (m *PRDetailModel) selectedActionsCheck() (check domain.PreviewCheckRow, jobID int64, ok bool) {
	if m.leftPanel.CICursor < 0 || m.leftPanel.CICursor >= len(m.leftPanel.Checks) {
		return domain.PreviewCheckRow{}, 0, false
	}
	check = m.leftPanel.Checks[m.leftPanel.CICursor]
	jobID, ok = m.actionsJobID(check)
	return check, jobID, ok
}

// actionsJobID returns the Actions job behind check, if it belongs to this repo.
func (m *PRDetailModel) actionsJobID(check domain.PreviewCheckRow) (int64, bool) {
	return domain.ActionsJobID(check.URL, m.repoNames()...)
}

// jobRepo is the repo to send a job's API calls to. It keeps this PR's
// identity (FullName, which keys caches and matches replies to this view)
// but takes Owner and Name, which form the API path, from the check URL.
// After a rename those are GitHub's current name while the remote may still
// have the old one, and the old path would only answer with a redirect: one
// a POST (re-run) can't follow, and one the log download, which drops the
// token on redirects to other hosts, shouldn't depend on.
func (m *PRDetailModel) jobRepo(check domain.PreviewCheckRow) domain.Repository {
	repo := m.Repo
	if owner, name, ok := domain.ActionsRepo(check.URL, m.repoNames()...); ok {
		if repo.FullName == "" {
			repo.FullName = m.Summary.Repo
		}
		repo.Owner, repo.Name = owner, name
	}
	return repo
}

// repoNames lists every name this PR's repo is known by. Repo.FullName comes
// from the git remote, which may still use a name from before a rename;
// Detail.Repo is GitHub's current name, which Actions URLs use.
func (m *PRDetailModel) repoNames() []string {
	names := []string{m.Repo.FullName, m.Summary.Repo}
	if m.Detail != nil {
		names = append(names, m.Detail.Repo)
	}
	return names
}

// handleCIEnter opens the log of the highlighted Actions check, or the
// check's page in the browser for any other check.
func (m *PRDetailModel) handleCIEnter() tea.Cmd {
	check, jobID, ok := m.selectedActionsCheck()
	if !ok || m.PRService == nil {
		return m.emitOpenBrowserCI()
	}
	var tick tea.Cmd
	if !m.isLoading() {
		tick = m.spinner.Tick // the spinner only runs while something loads
	}
	m.ciSeq++
	m.ciLog = ciLogState{
		active:    true,
		check:     check,
		jobID:     jobID,
		loading:   true,
		errCursor: -1,
		requestID: m.ciSeq,
	}
	m.leftPanel.Focus = FocusContent
	m.leftPanel.CILogOpen = true
	return tea.Batch(tick, cmds.LoadCILogCmd(m.PRService, m.jobRepo(check), m.Summary.Number, jobID, m.ciSeq))
}

// reanchorCILog keeps the sidebar highlight on the open log's check after
// the checks reload: by URL while it is the same job, else by name. A match
// with a different job means the log shown has been superseded.
func (m *PRDetailModel) reanchorCILog() {
	if !m.ciLog.active {
		return
	}
	// Both are worked out afresh on every refresh: a check can come back,
	// with the same job or a newer one. They never hold together, so the
	// title, r and Enter all tell the same story.
	idx := m.findCILogCheck()
	m.ciLog.gone = idx < 0
	m.ciLog.superseded = false
	if m.ciLog.gone {
		m.leftPanel.CILogOpen = false // highlight nothing
		return
	}
	m.leftPanel.CILogOpen = true
	m.leftPanel.CICursor = idx
	m.ensureCIVisible()
	if jobID, ok := m.actionsJobID(m.leftPanel.Checks[idx]); !ok || jobID != m.ciLog.jobID {
		m.ciLog.superseded = true
	}
}

// ciLogGoneNotice is what Enter and r say once the log's check has left the
// PR's checks.
func (m *PRDetailModel) ciLogGoneNotice() tea.Cmd {
	return m.setCINotice(m.ciLog.check.Name + " is no longer on this PR | Esc: Back")
}

// findCILogCheck returns the index of the open log's check in the refreshed
// list, or -1. A re-run job keeps its run ID but gets a new job ID, so after
// the exact URL it matches run ID and name. Names alone are often shared (a
// workflow run on both push and pull_request), so a name only counts when
// exactly one check has it.
func (m *PRDetailModel) findCILogCheck() int {
	checks := m.leftPanel.Checks
	for i, c := range checks {
		if c.URL == m.ciLog.check.URL {
			return i
		}
	}
	repos := m.repoNames()
	if run, ok := domain.ActionsRunID(m.ciLog.check.URL, repos...); ok {
		for i, c := range checks {
			if r, ok := domain.ActionsRunID(c.URL, repos...); ok && r == run && c.Name == m.ciLog.check.Name {
				return i
			}
		}
	}
	idx := -1
	for i, c := range checks {
		if c.Name != m.ciLog.check.Name {
			continue
		}
		if idx >= 0 {
			return -1 // ambiguous
		}
		idx = i
	}
	return idx
}

// setCINotice shows a re-run notice and returns the tick that clears it.
func (m *PRDetailModel) setCINotice(text string) tea.Cmd {
	m.ciSeq++
	id := m.ciSeq
	m.ciRerun.notice = text
	m.ciRerun.noticeID = id
	pr := m.ciPR()
	return tea.Tick(ciNoticeTTL, func(time.Time) tea.Msg { return CINoticeClearMsg{pr: pr, id: id} })
}

// closeCILog leaves the log view. back returns focus to the Checks box.
func (m *PRDetailModel) closeCILog(back bool) {
	if !m.ciLog.active {
		return
	}
	m.ciLog = ciLogState{}
	m.leftPanel.CILogOpen = false
	if back && m.Width >= MinWidthForSidebar {
		m.leftPanel.Focus = FocusCI
	}
}

// startRerun asks to confirm a re-run of the given Actions job.
func (m *PRDetailModel) startRerun(check domain.PreviewCheckRow, jobID int64) {
	if m.PRService == nil || m.ciRerun.inFlight || m.isAnyActionInProgress() {
		return
	}
	m.ciRerun = ciRerunState{confirm: true, check: check, jobID: jobID}
}

// handleRerunConfirmKey handles y/n while a re-run is awaiting confirmation.
func (m *PRDetailModel) handleRerunConfirmKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y":
		m.ciRerun.confirm = false
		m.ciRerun.inFlight = true
		return cmds.RerunCIJobCmd(m.PRService, m.jobRepo(m.ciRerun.check), m.Summary.Number, m.ciRerun.jobID)
	case "n", "esc":
		m.ciRerun.confirm = false
	}
	return nil
}

// handleCILogKey handles keys while the log view is open. handled is false
// for keys that should fall through to the normal PR detail bindings.
func (m *PRDetailModel) handleCILogKey(msg tea.KeyMsg) (cmd tea.Cmd, handled bool) {
	key := msg.String()
	if key != "g" {
		defer func() { m.LastKey = "" }()
	}
	switch key {
	case "esc", "q":
		m.closeCILog(true)
	case "1", "2", "3", "4", "tab", "shift+tab":
		m.closeCILog(false)
		return nil, false
	case "?", "T", "L", "R", "ctrl+p":
		// ctrl+p may not open a picker (diff loading, no files), and the
		// picker can be cancelled; GoToFile closes the log once a file is
		// picked.
		return nil, false
	case "j", "down":
		m.scrollCILog(1)
	case "k", "up":
		m.scrollCILog(-1)
	case "ctrl+d":
		m.scrollCILog(m.ciLogViewportHeight() / 2)
	case "ctrl+u":
		m.scrollCILog(-(m.ciLogViewportHeight() / 2))
	case "g":
		if m.LastKey == "g" {
			m.ciLog.scroll = 0
			m.LastKey = ""
		} else {
			m.LastKey = "g"
		}
	case "G":
		m.ciLog.scroll = m.maxCILogScroll()
	case "n":
		m.jumpCIError(1)
	case "N":
		m.jumpCIError(-1)
	case "enter":
		if m.ciLog.gone {
			// CICursor now points at whatever check took its place.
			return m.ciLogGoneNotice(), true
		}
		return m.handleCIEnter(), true // reload, picking up a newer job
	case "r":
		if m.ciLog.gone {
			return m.ciLogGoneNotice(), true
		}
		if m.ciLog.superseded {
			return m.setCINotice("A newer run of " + m.ciLog.check.Name + " exists | Enter: Load it"), true
		}
		m.startRerun(m.ciLog.check, m.ciLog.jobID)
	case "o":
		if url := m.ciLog.check.URL; url != "" {
			return func() tea.Msg { return OpenBrowserCI{URL: url} }, true
		}
	}
	return nil, true
}

func (m *PRDetailModel) ciLogViewportHeight() int {
	// The log fills the body box's full inner height (two rows more than the
	// tabs use); its last row is the footer.
	return max(m.contentViewportHeight()+1, 1)
}

func (m *PRDetailModel) maxCILogScroll() int {
	return max(len(m.ciLog.lines)-m.ciLogViewportHeight(), 0)
}

func (m *PRDetailModel) scrollCILog(delta int) {
	m.ciLog.scroll = clamp(m.ciLog.scroll+delta, 0, m.maxCILogScroll())
}

// jumpCIError moves to the next (dir > 0) or previous error line, wrapping.
func (m *PRDetailModel) jumpCIError(dir int) {
	n := len(m.ciLog.errorLines)
	if n == 0 {
		return
	}
	switch {
	case m.ciLog.errCursor < 0 && dir > 0:
		m.ciLog.errCursor = 0
	case m.ciLog.errCursor < 0:
		m.ciLog.errCursor = n - 1
	default:
		m.ciLog.errCursor = (m.ciLog.errCursor + dir + n) % n
	}
	m.scrollToCIError()
}

func (m *PRDetailModel) scrollToCIError() {
	line := m.ciLog.errorLines[m.ciLog.errCursor]
	m.ciLog.scroll = clamp(line-errorContextLines, 0, m.maxCILogScroll())
}

// handleCILogLoaded fills the log view, landing on the first error.
func (m *PRDetailModel) handleCILogLoaded(msg cmds.CILogLoaded) {
	if !m.ciLog.active || msg.RequestID != m.ciLog.requestID || !m.matchesPR(msg.Host, msg.Repo, msg.Number) {
		return
	}
	m.ciLog.loading = false
	if msg.Err != nil {
		m.ciLog.err = msg.Err
		return
	}
	m.ciLog.lines, m.ciLog.errorLines = parseJobLog(msg.Log)
	if len(m.ciLog.errorLines) > 0 {
		m.ciLog.errCursor = 0
		m.scrollToCIError()
		return
	}
	m.ciLog.scroll = m.maxCILogScroll()
}

// handleCIRerun reports the re-run result and, once GitHub has had a moment
// to queue the job, refreshes the checks.
func (m *PRDetailModel) handleCIRerun(msg cmds.CIRerunMsg) tea.Cmd {
	if !m.matchesPR(msg.Host, msg.Repo, msg.Number) || !m.ciRerun.inFlight || msg.JobID != m.ciRerun.jobID {
		return nil
	}
	m.ciRerun.inFlight = false
	if msg.Err != nil {
		return m.setCINotice("Re-run failed: " + msg.Err.Error())
	}
	// The re-run is a new attempt with its own job, so the open log (and r
	// in it) would keep pointing at the old one.
	if m.ciLog.active && m.ciLog.jobID == msg.JobID {
		m.closeCILog(true)
	}
	pr := m.ciPR()
	return tea.Batch(
		m.setCINotice(fmt.Sprintf("Re-run requested for %s", m.ciRerun.check.Name)),
		tea.Tick(ciRefreshDelay, func(time.Time) tea.Msg { return CIRefreshMsg{pr: pr} }),
	)
}

// ciPRRef names the PR a delayed CI message belongs to, so it is dropped if
// a different PR is open by the time it arrives.
type ciPRRef struct {
	host, repo string
	number     int
}

func (m *PRDetailModel) ciPR() ciPRRef {
	return ciPRRef{host: m.Repo.Host, repo: m.Repo.FullName, number: m.Summary.Number}
}

func (m *PRDetailModel) isCIPR(pr ciPRRef) bool {
	return m.matchesPR(pr.host, pr.repo, pr.number)
}

// CINoticeClearMsg clears the re-run notice with the given ID after a short
// delay. The app routes it to PR detail even when another view is active, so
// a notice can't outlive its delay while you're elsewhere.
type CINoticeClearMsg struct {
	pr ciPRRef
	id uint64
}

// CIRefreshMsg reloads the checks after a re-run. Like CINoticeClearMsg it
// is routed whatever view is active.
type CIRefreshMsg struct{ pr ciPRRef }

// ciRerunPrompt is the re-run confirm or progress text, or "". It blocks
// other actions, so it outranks their leftover messages.
func (m *PRDetailModel) ciRerunPrompt() string {
	switch {
	case m.ciRerun.confirm:
		return fmt.Sprintf("Re-run %q? (y/n)", m.ciRerun.check.Name)
	case m.ciRerun.inFlight:
		return "Requesting re-run..."
	}
	return ""
}

// ciRerunHint is the re-run prompt, progress or result, or "".
func (m *PRDetailModel) ciRerunHint() string {
	if hint := m.ciRerunPrompt(); hint != "" {
		return hint
	}
	return m.ciRerun.notice
}

// ciStatusHint is the status bar text for the log view and the re-run
// result, or "". It ranks below other flows' prompts.
func (m *PRDetailModel) ciStatusHint() string {
	if m.ciRerun.notice != "" {
		return m.ciRerun.notice
	}
	if m.ciLog.active {
		return "j/k: Scroll | n/N: Next/prev error | r: Re-run | o: Browser | Esc: Back | ?"
	}
	return ""
}

// renderCILogTitle is the tab row while the log is shown.
func (m *PRDetailModel) renderCILogTitle(width int) string {
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	check := m.ciLog.check
	left := th.SectionTabActive.Render("● Log") + " " + th.Bold.Render(check.Name) +
		"  " + ciIconStyled(check, th) + " " + th.DimTxt.Render(strings.ToLower(check.State))

	var right []string
	switch {
	case m.ciLog.gone:
		right = append(right, "no longer on this PR")
	case m.ciLog.superseded:
		right = append(right, "newer run · Enter: load")
	}
	if n := len(m.ciLog.errorLines); n > 0 {
		pos := "–"
		if m.ciLog.errCursor >= 0 {
			pos = fmt.Sprint(m.ciLog.errCursor + 1)
		}
		right = append(right, fmt.Sprintf("error %s/%d", pos, n))
	}
	if len(m.ciLog.lines) > 0 && m.ciLog.lines[0].kind == logOmitted {
		right = append(right, "tail")
	}
	rightStr := th.MutedTxt.Render(strings.Join(right, "  "))
	gap := width - lipgloss.Width(left) - lipgloss.Width(rightStr) - 1
	if rightStr == "" || gap < 2 {
		return truncateText(left, width)
	}
	return left + strings.Repeat(" ", gap) + rightStr
}

// renderCILog draws the log body: height rows, the last one a footer.
func (m *PRDetailModel) renderCILog(height, width int) []string {
	th := m.theme
	if th == nil {
		th = theme.Default()
	}
	bodyH := max(height-1, 1)
	out := make([]string, 0, height)

	switch {
	case m.ciLog.loading:
		out = append(out, m.spinner.View()+" Loading log…")
	case errors.Is(m.ciLog.err, domain.ErrJobLogUnavailable):
		out = append(out, th.DimTxt.Render("Log not available yet (job running or expired) — o: open in browser"))
	case m.ciLog.err != nil:
		out = append(out, th.Deletions.Render("Couldn't load log: "+m.ciLog.err.Error()))
	case len(m.ciLog.lines) == 0:
		out = append(out, th.DimTxt.Render("Log is empty"))
	default:
		scroll := clamp(m.ciLog.scroll, 0, max(len(m.ciLog.lines)-bodyH, 0))
		current := -1
		if m.ciLog.errCursor >= 0 {
			current = m.ciLog.errorLines[m.ciLog.errCursor]
		}
		for i := scroll; i < len(m.ciLog.lines) && len(out) < bodyH; i++ {
			if i == current {
				// Marked like the diff cursor, so n/N visibly moves even when
				// every error fits on screen and the scroll can't change.
				line := th.PrimaryTxt.Render("▎") + styleLogLine(m.ciLog.lines[i], th, width-1)
				out = append(out, th.FillRow(th.Highlight, width, line))
				continue
			}
			out = append(out, styleLogLine(m.ciLog.lines[i], th, width))
		}
	}
	for len(out) < bodyH {
		out = append(out, "")
	}

	footer := th.RenderHints("r: Re-run | n/N: Next/prev error | o: Browser | Esc: Back")
	if hint := m.ciRerunHint(); hint != "" {
		footer = th.PrimaryTxt.Render(hint)
	}
	return append(out, truncateText(footer, width))
}

func styleLogLine(l logLine, th *theme.Theme, width int) string {
	text := truncateText(l.text, width)
	switch l.kind {
	case logError:
		return th.Deletions.Render(text)
	case logWarning:
		return th.CIPending.Render(text)
	case logGroup:
		return th.Bold.Render(text)
	case logCommand, logOmitted:
		return th.DimTxt.Render(text)
	}
	return text
}

// ChecksActive reports whether the Checks box has focus or a job log is shown.
func (m *PRDetailModel) ChecksActive() bool {
	return m.ciLog.active || m.leftPanel.Focus == FocusCI
}
