package prdetail

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

const actionsCheckURL = "https://github.com/acme/api/actions/runs/10/job/77"

// ciLogPRService records job log and re-run calls on top of the
// update-branch fake.
type ciLogPRService struct {
	updateBranchPRService
	log         domain.JobLog
	logErr      error
	logCalls    int
	rerunCalls  int
	rerunJobID  int64
	rerunErrOut error
	logRepo     domain.Repository // the repo the last API call was made for
	rerunRepo   domain.Repository
}

func (s *ciLogPRService) LoadJobLog(_ context.Context, repo domain.Repository, _ int64) (domain.JobLog, error) {
	s.logCalls++
	s.logRepo = repo
	return s.log, s.logErr
}

func (s *ciLogPRService) RerunJob(_ context.Context, repo domain.Repository, _ int, jobID int64) error {
	s.rerunCalls++
	s.rerunJobID = jobID
	s.rerunRepo = repo
	return s.rerunErrOut
}

func newCILogModel(svc *ciLogPRService) *PRDetailModel {
	m := newUpdateModel("CLEAN", domain.PRStateOpen)
	m.PRService = svc
	m.DetailLoading, m.DiffLoading, m.leftPanel.Loading = false, false, false
	m.leftPanel.Checks = []domain.PreviewCheckRow{
		{Name: "test (ubuntu)", State: "FAILURE", URL: actionsCheckURL},
		{Name: "codecov/patch", State: "PENDING", URL: "https://app.codecov.io/gh/acme/api/pull/42"},
	}
	m.leftPanel.Focus = FocusCI
	m.SetTheme(theme.Default())
	return m
}

// collectMsgs runs cmd and every command batched under it.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collectMsgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func press(m *PRDetailModel, key string) (*PRDetailModel, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	return m.Update(msg)
}

// openLog presses Enter on the Actions check and feeds back the loaded log.
func openLog(t *testing.T, m *PRDetailModel) *PRDetailModel {
	t.Helper()
	m, cmd := press(m, "enter")
	for _, msg := range collectMsgs(cmd) {
		if loaded, ok := msg.(cmds.CILogLoaded); ok {
			m, _ = m.Update(loaded)
			return m
		}
	}
	t.Fatal("Enter on an Actions check did not load its log")
	return m
}

func TestParseLogLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		want     string
		wantKind logLineKind
		wantOK   bool
	}{
		{"2025-01-02T03:04:05.1234567Z ok  pkg 0.1s", "ok  pkg 0.1s", logPlain, true},
		{"2025-01-02T03:04:05.1234567Z ##[group]Run go test ./...", "▸ Run go test ./...", logGroup, true},
		{"2025-01-02T03:04:05.1234567Z ##[endgroup]", "", logPlain, false},
		{"2025-01-02T03:04:05Z ##[error]Process completed with exit code 1.", "Process completed with exit code 1.", logError, true},
		{"##[warning]Node 16 is deprecated", "Node 16 is deprecated", logWarning, true},
		{"\x1b[31m--- FAIL: TestX (0.31s)\x1b[0m", "--- FAIL: TestX (0.31s)", logError, true},
		{"    panic: boom", "    panic: boom", logError, true},
		{"\xef\xbb\xbf2025-01-02T03:04:05Z first", "first", logPlain, true},
		{"errors.go compiled", "errors.go compiled", logPlain, true},
		{"2025-01-02T03:04:05Z [command]/usr/bin/git version", "$ /usr/bin/git version", logCommand, true},
		{"make: *** [Makefile:116: test-unit-race] Error 1", "make: *** [Makefile:116: test-unit-race] Error 1", logError, true},
		{"Pulling 10%\rPulling 100%\r", "Pulling 100%", logPlain, true},
		{"Error: tests failed\rAll checks passed", "All checks passed", logPlain, true},
		{"bell\a back\bspace\x00nul\tend", "bell backspacenul    end", logPlain, true},
	}
	for _, tt := range tests {
		got, ok := parseLogLine(tt.raw)
		if ok != tt.wantOK {
			t.Errorf("parseLogLine(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			continue
		}
		if ok && (got.text != tt.want || got.kind != tt.wantKind) {
			t.Errorf("parseLogLine(%q) = %q/%d, want %q/%d", tt.raw, got.text, got.kind, tt.want, tt.wantKind)
		}
	}
}

func TestParseJobLogIndexesErrorsAfterOmittedMarker(t *testing.T) {
	t.Parallel()
	lines, errs := parseJobLog(domain.JobLog{
		Lines:     []string{"a", "##[endgroup]", "Error: one", "b", "##[error]two"},
		Truncated: true,
	})
	if lines[0].kind != logOmitted {
		t.Fatalf("first line = %+v, want omitted marker", lines[0])
	}
	if len(errs) != 2 || lines[errs[0]].text != "Error: one" || lines[errs[1]].text != "two" {
		t.Errorf("error index %v over %+v", errs, lines)
	}
}

func TestEnterOnActionsCheckOpensLogAtFirstError(t *testing.T) {
	t.Parallel()
	raw := make([]string, 0, 100)
	for range 60 {
		raw = append(raw, "noise")
	}
	raw = append(raw, "--- FAIL: TestSenderRetries", "more", "##[error]Process completed with exit code 1.")
	for range 37 {
		raw = append(raw, "cleanup")
	}
	svc := &ciLogPRService{log: domain.JobLog{Lines: raw}}
	m := openLog(t, newCILogModel(svc))

	if !m.ciLog.active || m.leftPanel.Focus != FocusContent {
		t.Fatalf("log view not open: active=%v focus=%v", m.ciLog.active, m.leftPanel.Focus)
	}
	if svc.logCalls != 1 {
		t.Errorf("LoadJobLog calls = %d, want 1", svc.logCalls)
	}
	if want := 60 - errorContextLines; m.ciLog.scroll != want {
		t.Errorf("scroll = %d, want %d (first error with context)", m.ciLog.scroll, want)
	}
	view := stripANSI(m.View())
	for _, want := range []string{"● Log", "test (ubuntu)", "--- FAIL: TestSenderRetries", "error 1/2", "Esc Back"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}

	m, _ = press(m, "n")
	if m.ciLog.errCursor != 1 {
		t.Errorf("after n errCursor = %d, want 1", m.ciLog.errCursor)
	}
	m, _ = press(m, "n")
	if m.ciLog.errCursor != 0 {
		t.Errorf("n should wrap to the first error, got %d", m.ciLog.errCursor)
	}
	m, _ = press(m, "N")
	if m.ciLog.errCursor != 1 {
		t.Errorf("N should wrap to the last error, got %d", m.ciLog.errCursor)
	}

	m, _ = press(m, "esc")
	if m.ciLog.active || m.leftPanel.Focus != FocusCI || m.leftPanel.CICursor != 0 {
		t.Errorf("esc: active=%v focus=%v cursor=%d", m.ciLog.active, m.leftPanel.Focus, m.leftPanel.CICursor)
	}
}

func TestEnterOnOtherCheckOpensBrowser(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{}
	m := newCILogModel(svc)
	m.leftPanel.CICursor = 1
	m, cmd := press(m, "enter")
	if m.ciLog.active || svc.logCalls != 0 {
		t.Fatal("a non-Actions check must not open the log view")
	}
	msgs := collectMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v", msgs)
	}
	if got, ok := msgs[0].(OpenBrowserCI); !ok || got.URL != m.leftPanel.Checks[1].URL {
		t.Errorf("msg = %#v, want OpenBrowserCI for the check URL", msgs[0])
	}
}

func TestLogUnavailableShowsHint(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{logErr: domain.ErrJobLogUnavailable}
	m := openLog(t, newCILogModel(svc))
	if view := stripANSI(m.View()); !strings.Contains(view, "Log not available yet") {
		t.Errorf("view missing unavailable hint:\n%s", view)
	}
}

func TestStaleLogIsIgnored(t *testing.T) {
	t.Parallel()
	m := newCILogModel(&ciLogPRService{})
	m, _ = press(m, "enter")
	m, _ = m.Update(cmds.CILogLoaded{Repo: "acme/api", Number: 42, JobID: 999, Log: domain.JobLog{Lines: []string{"x"}}})
	if !m.ciLog.loading || len(m.ciLog.lines) != 0 {
		t.Error("a log for another job must not replace the open one")
	}
}

func TestRerunFromLogConfirmsThenRefreshes(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{log: domain.JobLog{Lines: []string{"##[error]boom"}}}
	m := openLog(t, newCILogModel(svc))

	m, _ = press(m, "r")
	if !m.ciRerun.confirm || !strings.Contains(m.StatusHint(), `Re-run "test (ubuntu)"? (y/n)`) {
		t.Fatalf("r should ask to confirm, hint=%q", m.StatusHint())
	}
	m, cmd := press(m, "y")
	msgs := collectMsgs(cmd)
	if svc.rerunCalls != 1 || svc.rerunJobID != 77 || len(msgs) != 1 {
		t.Fatalf("rerun calls=%d job=%d msgs=%#v", svc.rerunCalls, svc.rerunJobID, msgs)
	}

	before := m.detailRequestID
	m, _ = m.Update(msgs[0])
	if m.ciRerun.inFlight || !strings.Contains(m.StatusHint(), "Re-run requested") {
		t.Errorf("after re-run: inFlight=%v hint=%q", m.ciRerun.inFlight, m.StatusHint())
	}
	if m.detailRequestID != before {
		t.Error("the checks should reload after a delay, not straight away")
	}
	m, _ = m.Update(CIRefreshMsg{pr: m.ciPR()})
	if m.detailRequestID != before+1 {
		t.Error("the delayed refresh should reload the PR detail")
	}
}

func TestRerunNoticeYieldsToLaterPrompts(t *testing.T) {
	t.Parallel()
	m := newCILogModel(&ciLogPRService{})
	m.setCINotice("Re-run requested for test (ubuntu)")
	m.updateStep = updateStepConfirm
	if hint := m.StatusHint(); !strings.Contains(hint, "Update branch #42") {
		t.Errorf("hint = %q, want the update-branch prompt", hint)
	}
}

func TestStaleLoadOfSameJobIsIgnored(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{log: domain.JobLog{Lines: []string{"first"}}}
	m := newCILogModel(svc)
	m, firstCmd := press(m, "enter")
	m, _ = press(m, "esc")
	m, _ = press(m, "enter") // reopen the same job before the first load lands
	for _, msg := range collectMsgs(firstCmd) {
		if loaded, ok := msg.(cmds.CILogLoaded); ok {
			m, _ = m.Update(loaded)
		}
	}
	if !m.ciLog.loading || len(m.ciLog.lines) != 0 {
		t.Error("the first request's response must not fill the reopened view")
	}
}

func TestRefreshReanchorsOpenLogCheck(t *testing.T) {
	t.Parallel()
	m := openLog(t, newCILogModel(&ciLogPRService{log: domain.JobLog{Lines: []string{"x"}}}))
	rerun := domain.PreviewCheckRow{Name: "test (ubuntu)", State: "QUEUED", URL: "https://github.com/acme/api/actions/runs/11/job/88"}
	detail := *m.Detail
	detail.Checks = []domain.PreviewCheckRow{{Name: "lint", State: "SUCCESS", URL: "https://github.com/acme/api/actions/runs/11/job/80"}, rerun}
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "acme/api", Number: 42, Detail: detail})

	if m.leftPanel.CICursor != 1 || !m.leftPanel.CILogOpen {
		t.Errorf("cursor = %d open=%v, want the re-run check highlighted", m.leftPanel.CICursor, m.leftPanel.CILogOpen)
	}
	if !m.ciLog.superseded || !strings.Contains(stripANSI(m.View()), "newer run") {
		t.Error("the log should be marked as superseded by the newer job")
	}
	m, _ = press(m, "r")
	if m.ciRerun.confirm || !strings.Contains(m.StatusHint(), "A newer run") {
		t.Errorf("r on a superseded log should point at the newer run, hint=%q", m.StatusHint())
	}
	m, _ = press(m, "enter")
	if m.ciLog.jobID != 88 || m.ciLog.superseded {
		t.Errorf("Enter should load the newer job, got job %d superseded=%v", m.ciLog.jobID, m.ciLog.superseded)
	}
}

// sharedNameModel has the log of the push run's "ci" open, next to a
// pull_request run's "ci".
func sharedNameModel(t *testing.T) *PRDetailModel {
	t.Helper()
	m := newCILogModel(&ciLogPRService{log: domain.JobLog{Lines: []string{"x"}}})
	m.leftPanel.Checks = []domain.PreviewCheckRow{
		{Name: "ci", State: "SUCCESS", URL: "https://github.com/acme/api/actions/runs/20/job/90"},
		{Name: "ci", State: "FAILURE", URL: actionsCheckURL}, // run 10, job 77
	}
	m.leftPanel.CICursor = 1
	return openLog(t, m)
}

func refreshChecks(m *PRDetailModel, checks ...domain.PreviewCheckRow) *PRDetailModel {
	detail := *m.Detail
	detail.Checks = checks
	m, _ = m.Update(cmds.PRDetailLoaded{Repo: "acme/api", Number: 42, Detail: detail})
	return m
}

func TestReanchorMatchesRerunByRunNotSharedName(t *testing.T) {
	t.Parallel()
	m := sharedNameModel(t)
	// The push job was re-run: same run, new job ID, now listed second.
	m = refreshChecks(m,
		domain.PreviewCheckRow{Name: "ci", State: "SUCCESS", URL: "https://github.com/acme/api/actions/runs/20/job/90"},
		domain.PreviewCheckRow{Name: "ci", State: "QUEUED", URL: "https://github.com/acme/api/actions/runs/10/job/99"},
	)
	if m.leftPanel.CICursor != 1 || !m.ciLog.superseded {
		t.Errorf("cursor = %d superseded=%v, want the re-run of run 10", m.leftPanel.CICursor, m.ciLog.superseded)
	}
}

func TestEnterOnVanishedCheckLoadsNothing(t *testing.T) {
	t.Parallel()
	m := sharedNameModel(t)
	// Run 10 is gone and "ci" is ambiguous, so nothing matches.
	m = refreshChecks(m,
		domain.PreviewCheckRow{Name: "ci", State: "SUCCESS", URL: "https://github.com/acme/api/actions/runs/20/job/90"},
		domain.PreviewCheckRow{Name: "ci", State: "SUCCESS", URL: "https://github.com/acme/api/actions/runs/30/job/91"},
	)
	if m.leftPanel.CILogOpen || !m.ciLog.gone {
		t.Fatalf("open=%v gone=%v, want the check treated as gone", m.leftPanel.CILogOpen, m.ciLog.gone)
	}
	// The returned command is only the notice's clear tick; running it
	// would just wait out the TTL.
	m, _ = press(m, "enter")
	if m.ciLog.jobID != 77 || !strings.Contains(m.StatusHint(), "no longer on this PR") {
		t.Errorf("job = %d hint = %q, want the old log kept and a notice", m.ciLog.jobID, m.StatusHint())
	}

	// The title and r agree with Enter instead of pointing at a newer run.
	if m.ciLog.superseded || strings.Contains(stripANSI(m.View()), "newer run") {
		t.Error("a vanished check must not be reported as having a newer run")
	}
	m, _ = press(m, "r")
	if m.ciRerun.confirm || !strings.Contains(m.StatusHint(), "no longer on this PR") {
		t.Errorf("r on a vanished check: confirm=%v hint=%q", m.ciRerun.confirm, m.StatusHint())
	}

	// The same job comes back: nothing is gone or superseded, and r works.
	m = refreshChecks(m, domain.PreviewCheckRow{Name: "ci", State: "FAILURE", URL: actionsCheckURL})
	if m.ciLog.gone || m.ciLog.superseded || !m.leftPanel.CILogOpen {
		t.Fatalf("after the check returned: gone=%v superseded=%v open=%v", m.ciLog.gone, m.ciLog.superseded, m.leftPanel.CILogOpen)
	}
	m, _ = press(m, "r")
	if !m.ciRerun.confirm {
		t.Errorf("r should ask to re-run once the check is back, hint=%q", m.StatusHint())
	}
}

func TestCtrlPInLogShowsWhyNoPicker(t *testing.T) {
	t.Parallel()
	m := openLog(t, newCILogModel(&ciLogPRService{log: domain.JobLog{Lines: []string{"x"}}}))
	m.Diff, m.DiffLoading = nil, true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if got := m.StatusHint(); got != "Diff still loading" {
		t.Errorf("hint = %q, want the ctrl+p notice", got)
	}
}

func TestOtherRepoActionsCheckOpensBrowser(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{}
	m := newCILogModel(svc)
	m.leftPanel.Checks[0].URL = "https://github.com/someone/else/actions/runs/1/job/2"
	m, _ = press(m, "enter")
	if m.ciLog.active || svc.logCalls != 0 {
		t.Error("another repo's Actions job must not be treated as this PR's")
	}
}

func TestRerunSuccessClosesLog(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{log: domain.JobLog{Lines: []string{"##[error]boom"}}}
	m := openLog(t, newCILogModel(svc))
	m, _ = press(m, "r")
	m, cmd := press(m, "y")
	m, _ = m.Update(collectMsgs(cmd)[0])
	if m.ciLog.active || m.leftPanel.Focus != FocusCI {
		t.Errorf("log still open for the old job: active=%v focus=%v", m.ciLog.active, m.leftPanel.Focus)
	}
}

func TestStaleNoticeClearKeepsNewerNotice(t *testing.T) {
	t.Parallel()
	m := newCILogModel(&ciLogPRService{})
	rerun := func() tea.Msg {
		m.ciRerun = ciRerunState{inFlight: true, jobID: 77, check: m.leftPanel.Checks[0], noticeID: m.ciRerun.noticeID}
		var clear tea.Msg
		for _, msg := range collectMsgs(m.handleCIRerun(cmds.CIRerunMsg{Repo: "acme/api", Number: 42, JobID: 77})) {
			if c, ok := msg.(CINoticeClearMsg); ok {
				clear = c
			}
		}
		return clear
	}
	first := rerun()
	second := rerun()
	m, _ = m.Update(first)
	if m.ciRerun.notice == "" {
		t.Fatal("the first re-run's clear tick wiped the second re-run's notice")
	}
	m, _ = m.Update(second)
	if m.ciRerun.notice != "" {
		t.Errorf("notice = %q, want cleared", m.ciRerun.notice)
	}
}

func TestCtrlPKeepsLogWhenNoPickerOpens(t *testing.T) {
	t.Parallel()
	m := openLog(t, newCILogModel(&ciLogPRService{log: domain.JobLog{Lines: []string{"##[error]boom"}}}))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if !m.ciLog.active {
		t.Error("ctrl+p closed the log although no picker opened")
	}
}

func TestCurrentErrorIsMarked(t *testing.T) {
	t.Parallel()
	// Both errors fit on screen, so n can't change the scroll.
	svc := &ciLogPRService{log: domain.JobLog{Lines: []string{"start", "##[error]first boom", "middle", "##[error]second boom", "end"}}}
	m := openLog(t, newCILogModel(svc))
	marked := func() string {
		for _, l := range strings.Split(stripANSI(m.View()), "\n") {
			if strings.Contains(l, "▎") && strings.Contains(l, "boom") {
				return l
			}
		}
		return ""
	}
	if l := marked(); !strings.Contains(l, "first boom") {
		t.Fatalf("marked line = %q, want the first error", l)
	}
	m, _ = press(m, "n")
	if l := marked(); !strings.Contains(l, "second boom") {
		t.Errorf("after n marked line = %q, want the second error", l)
	}
}

func TestRerunDeclinedAndFailure(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{rerunErrOut: errors.New("job is still running")}
	m := newCILogModel(svc)

	m, _ = press(m, "r") // from the sidebar
	m, _ = press(m, "n")
	if m.ciRerun.confirm || svc.rerunCalls != 0 {
		t.Fatal("n should cancel without re-running")
	}

	m, _ = press(m, "r")
	m, cmd := press(m, "y")
	before := m.detailRequestID
	for _, msg := range collectMsgs(cmd) {
		m, _ = m.Update(msg)
	}
	if !strings.Contains(m.StatusHint(), "Re-run failed: job is still running") {
		t.Errorf("hint = %q", m.StatusHint())
	}
	if m.detailRequestID != before {
		t.Error("a failed re-run should not reload the PR detail")
	}
}

func TestRerunIgnoredForNonActionsCheck(t *testing.T) {
	t.Parallel()
	m := newCILogModel(&ciLogPRService{})
	m.leftPanel.CICursor = 1
	m, _ = press(m, "r")
	if m.ciRerun.confirm {
		t.Error("r on a non-Actions check should do nothing")
	}
}

func TestDelayedMessagesForAnotherPRAreIgnored(t *testing.T) {
	t.Parallel()
	m := newCILogModel(&ciLogPRService{})
	m.setCINotice("Re-run requested for test (ubuntu)")
	other := ciPRRef{repo: "acme/api", number: 7}

	m, _ = m.Update(CINoticeClearMsg{pr: other, id: m.ciRerun.noticeID})
	if m.ciRerun.notice == "" {
		t.Error("another PR's clear tick wiped this PR's notice")
	}
	before := m.detailRequestID
	m, _ = m.Update(CIRefreshMsg{pr: other})
	if m.detailRequestID != before {
		t.Error("another PR's refresh tick reloaded this PR")
	}
}

func TestActionsCheckOfRenamedRepoOpensLog(t *testing.T) {
	t.Parallel()
	svc := &ciLogPRService{log: domain.JobLog{Lines: []string{"##[error]boom"}}}
	m := newCILogModel(svc)
	// The local remote still says acme/api; GitHub now calls it acme/api-v2.
	m.Detail.Repo = "acme/api-v2"
	m.leftPanel.Checks[0].URL = "https://github.com/acme/api-v2/actions/runs/10/job/77"
	m = openLog(t, m)
	if !m.ciLog.active || m.ciLog.jobID != 77 || len(m.ciLog.lines) == 0 {
		t.Fatalf("active=%v job=%d lines=%d, want the renamed repo's job log", m.ciLog.active, m.ciLog.jobID, len(m.ciLog.lines))
	}
	// The API path uses GitHub's current name, so there is no rename
	// redirect; FullName stays the PR's, so the reply still matches.
	if r := svc.logRepo; r.Owner != "acme" || r.Name != "api-v2" || r.FullName != "acme/api" {
		t.Errorf("log fetched for %+v, want owner/name acme/api-v2 and FullName acme/api", r)
	}

	m, _ = press(m, "r")
	m, cmd := press(m, "y")
	collectMsgs(cmd)
	if r := svc.rerunRepo; r.Owner != "acme" || r.Name != "api-v2" {
		t.Errorf("re-run sent for %+v, want acme/api-v2", r)
	}
}
