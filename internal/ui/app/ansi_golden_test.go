package app

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

// These goldens keep colour escape codes, so any change to what the default
// theme paints fails here even when the plain-text goldens still match.
//
// Run with -update-ansi to regenerate them:
//
//	go test ./internal/ui/app/... -run TestANSIGolden -update-ansi
var updateANSIGolden = flag.Bool("update-ansi", false, "overwrite colour (ANSI) golden files")

// Colour profiles a screen can be rendered under, by golden directory name.
var ansiProfiles = []struct {
	name    string
	profile termenv.Profile
}{
	{"TrueColor", termenv.TrueColor},
	{"ANSI256", termenv.ANSI256},
	{"ANSI", termenv.ANSI},
	{"Ascii", termenv.Ascii},
}

type ansiScreen struct {
	name string
	// profiles marks screens that are also rendered below truecolor.
	profiles bool
	build    func(t *testing.T) *Model
}

func ansiScreens() []ansiScreen {
	return []ansiScreen{
		{"dashboard", true, func(t *testing.T) *Model { return ansiDashboard(t, 120, 40) }},
		{"dashboard_w80", false, func(t *testing.T) *Model { return ansiDashboard(t, 80, 30) }},
		{"dashboard_loading", false, ansiDashboardLoading},
		{"dashboard_stale", false, func(t *testing.T) *Model { return ansiDashboardLoaded(t, true, nil) }},
		{"dashboard_error", false, func(t *testing.T) *Model {
			return ansiDashboardLoaded(t, false, errors.New("graphql: rate limited"))
		}},
		{"cmd_palette", true, func(t *testing.T) *Model {
			m := ansiDashboard(t, 120, 40)
			_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			return m
		}},
		{"repo_pick", false, func(t *testing.T) *Model {
			m := ansiDashboard(t, 120, 40)
			m.openRepoPickOverlay(123, []domain.Repository{testutil.Repo("acme/alpha"), testutil.Repo("acme/beta")})
			return m
		}},
		{"keymap_overlay", false, func(t *testing.T) *Model {
			m := ansiDashboard(t, 120, 40)
			ansiKeys(m, "?")
			return m
		}},
		{"create_pr", false, func(t *testing.T) *Model {
			m := ansiDashboard(t, 120, 40)
			ansiKeys(m, "n")
			return m
		}},
		{"prdetail_desc", false, func(t *testing.T) *Model { return ansiPRDetail(t, "1") }},
		{"prdetail_desc_files_focus", false, func(t *testing.T) *Model {
			m := ansiPRDetail(t, "1")
			_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			return m
		}},
		{"prdetail_diff", false, func(t *testing.T) *Model { return ansiPRDetail(t, "2") }},
		{"prdetail_diff_search", true, func(t *testing.T) *Model {
			m := ansiPRDetail(t, "2")
			ansiKeys(m, "/", "line")
			return m
		}},
		{"prdetail_diff_since_review", false, func(t *testing.T) *Model {
			m := ansiPRDetailSinceReview(t)
			ansiKeys(m, "s")
			return m
		}},
		{"prdetail_comments", true, func(t *testing.T) *Model { return ansiPRDetail(t, "3") }},
		{"prdetail_close_prompt", false, func(t *testing.T) *Model { return ansiPRDetail(t, "x") }},
		{"prdetail_commits", false, func(t *testing.T) *Model { return ansiPRDetail(t, "4") }},
	}
}

// TestANSIGolden renders each screen with colours kept. It changes the global
// lipgloss colour profile, so it must not run in parallel.
func TestANSIGolden(t *testing.T) {
	for _, sc := range ansiScreens() {
		t.Run(sc.name, func(t *testing.T) {
			withColorProfile(t, termenv.TrueColor)
			got := sc.build(t).View()
			checkANSIGolden(t, got, filepath.Join("pho-default", "TrueColor", sc.name+".ansi"))
		})
	}
}

// TestANSIGoldenProfiles renders a subset of screens under the lower colour
// profiles, pinning how the default theme degrades on 256- and 16-colour
// terminals and with colour turned off.
func TestANSIGoldenProfiles(t *testing.T) {
	for _, sc := range ansiScreens() {
		if !sc.profiles {
			continue
		}
		for _, p := range ansiProfiles[1:] {
			t.Run(sc.name+"/"+p.name, func(t *testing.T) {
				withColorProfile(t, p.profile)
				got := sc.build(t).View()
				if p.profile == termenv.Ascii && strings.Contains(got, "\x1b[") {
					t.Errorf("Ascii render of %s contains escape codes", sc.name)
				}
				checkANSIGolden(t, got, filepath.Join("pho-default", p.name, sc.name+".ansi"))
			})
		}
	}
}

// ansiThemeName is the theme screens are built with; tests that render a
// preset set it for their duration.
var ansiThemeName = theme.DefaultName

// ansiTheme builds the current theme. Build reads the colour profile, so
// call it after withColorProfile.
func ansiTheme(t testing.TB) *theme.Theme {
	t.Helper()
	th, ok := theme.Get(ansiThemeName)
	if !ok {
		t.Fatalf("theme %q not registered", ansiThemeName)
	}
	return th
}

// TestANSIGoldenPresets renders the profile screens with every preset under
// every colour profile.
func TestANSIGoldenPresets(t *testing.T) {
	for _, name := range theme.Names() {
		if name == theme.DefaultName {
			continue
		}
		for _, sc := range ansiScreens() {
			if !sc.profiles {
				continue
			}
			for _, p := range ansiProfiles {
				t.Run(name+"/"+sc.name+"/"+p.name, func(t *testing.T) {
					prev := ansiThemeName
					ansiThemeName = name
					t.Cleanup(func() { ansiThemeName = prev })
					withColorProfile(t, p.profile)

					got := sc.build(t).View()
					switch {
					case p.profile == termenv.Ascii && strings.Contains(got, "\x1b["):
						t.Errorf("Ascii render contains escape codes")
					case p.profile == termenv.ANSI && sc.name == "dashboard" && !strings.Contains(got, "\x1b[7m"):
						t.Errorf("16-colour render has no reverse-video cursor row")
					}
					checkANSIGolden(t, got, filepath.Join(name, p.name, sc.name+".ansi"))
				})
			}
		}
	}
}

func withColorProfile(t *testing.T, p termenv.Profile) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(p)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func checkANSIGolden(t *testing.T, got, name string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", "ansi", name)
	if *updateANSIGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update-ansi to generate)", path, err)
	}
	if got != string(want) {
		t.Errorf("colour golden mismatch for %s\ngot (escapes shown):\n%q\nwant:\n%q", name, got, string(want))
	}
}

// ansiKeys feeds each argument to the model: single-rune strings and named
// keys as one key press, longer strings as typed text.
func ansiKeys(m *Model, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		default:
			for _, r := range k {
				_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				drainCmds(m, cmd)
			}
			continue
		}
		_, cmd := m.Update(msg)
		drainCmds(m, cmd)
	}
}

// ansiDashboardPRs covers every state pill, CI and review icon.
func ansiDashboardPRs(repo string) []domain.PullRequestSummary {
	open := pr(repo, 101, "Fix login flow on Safari")
	failing := pr(repo, 102, "Add retry to webhook sender")
	failing.CIStatus = domain.CIStatusFailure
	failing.ReviewDecision = domain.ReviewDecisionChangesRequested
	pending := pr(repo, 103, "Bump golang.org/x/net")
	pending.CIStatus = domain.CIStatusPending
	pending.ReviewDecision = domain.ReviewDecisionReviewRequired
	draft := pr(repo, 104, "WIP: new settings page")
	draft.IsDraft = true
	draft.CIStatus = domain.CIStatusNone
	draft.ReviewDecision = domain.ReviewDecisionNone
	return []domain.PullRequestSummary{open, failing, pending, draft}
}

func ansiPreview(repo string, number int) domain.PRPreviewSnapshot {
	p := deeplinkPreviewFixture()
	p.Repo = repo
	p.Number = number
	p.Title = "Fix login flow on Safari"
	p.BodyExcerpt = "## Why\n\nThe login button did **nothing** on Safari.\n\n- clears the stale `session` cookie\n- adds a [regression test](https://example.com)\n\n> Safari only."
	p.Checks = append(p.Checks, domain.PreviewCheckRow{Name: "ci/lint", State: "FAILURE"})
	return p
}

func ansiDashboard(t *testing.T, w, h int) *Model {
	t.Helper()
	repo := testutil.Repo("acme/alpha")
	prs := ansiDashboardPRs(repo.FullName)
	m := setupModelWithPRs(t, []domain.Repository{repo}, prs)
	m.SetTheme(ansiTheme(t))
	dash := m.deps.Dashboard.(*stubDashboardService)
	for _, p := range prs {
		dash.previewByPR[previewKey(repo.FullName, p.Number)] = ansiPreview(repo.FullName, p.Number)
	}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	drainCmds(m, cmd)
	_, _ = m.Update(cmdsPreviewLoaded(repo.FullName, 101, ansiPreview(repo.FullName, 101), false, nil))
	return m
}

func ansiDashboardLoading(t *testing.T) *Model {
	t.Helper()
	repo := testutil.Repo("acme/alpha")
	m := newTestModel([]domain.Repository{repo}, nil)
	m.SetTheme(ansiTheme(t))
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	return m
}

func ansiDashboardLoaded(t *testing.T, fromCache bool, err error) *Model {
	t.Helper()
	repo := testutil.Repo("acme/alpha")
	snap := dashboardSnapshot(repo, ansiDashboardPRs(repo.FullName)...)
	m := newTestModel([]domain.Repository{repo}, map[string]domain.DashboardSnapshot{repo.FullName: snap})
	m.SetTheme(ansiTheme(t))
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, _ = m.Update(cmdsReposDiscovered([]domain.Repository{repo}))
	_, _ = m.Update(cmdsDashboardLoaded(repo.FullName, snap, fromCache, err))
	return m
}

// ansiPRDetailFixture has several comment authors, a resolved and an
// unresolved review thread, and a diff with word-level changes.
func ansiPRDetailFixture() (domain.PRPreviewSnapshot, diffmodel.DiffModel) {
	base := fixedNow()
	p := ansiPreview("acme/alpha", 123)
	p.Number = 123
	p.Reviewers = []domain.PreviewReviewer{
		{Login: "alice", State: "APPROVED", SubmittedAt: base.Add(-2 * time.Hour)},
		{Login: "dave", State: "CHANGES_REQUESTED", Body: "Please cover the cookie path.", SubmittedAt: base.Add(-time.Hour)},
	}
	p.Comments = []domain.PreviewComment{
		{ID: "c1", Login: "bob", Body: "Thanks, this has bitten us twice.", CreatedAt: base.Add(-3 * time.Hour)},
		{ID: "c2", Login: "carol", Body: "Can we backport this?", CreatedAt: base.Add(-90 * time.Minute)},
	}
	p.ReviewThreads = []domain.PreviewReviewThread{
		{ID: "t1", Path: "web/login.go", Line: 2, Comments: []domain.PreviewThreadComment{
			{ID: "tc1", Login: "dave", Body: "Should this also clear `csrf`?", CreatedAt: base.Add(-time.Hour)},
		}},
		{ID: "t2", Path: "web/login.go", Line: 3, IsResolved: true, ResolvedBy: "octocat", Comments: []domain.PreviewThreadComment{
			{ID: "tc2", Login: "alice", Body: "nit: rename", CreatedAt: base.Add(-2 * time.Hour)},
		}},
	}

	d := deeplinkDiffFixture()
	d.Files[0].Hunks[0].Lines = []diffmodel.DiffLine{
		{Kind: "context", Raw: " func login() {"},
		{Kind: "deletion", Raw: "-\tclearCookie(old line)"},
		{Kind: "addition", Raw: "+\tclearCookie(new line)"},
		{Kind: "addition", Raw: "+\tclearSession()"},
		{Kind: "context", Raw: " }"},
	}
	d.Files[0].DisplayRows = 6
	return p, d
}

func ansiPRDetail(t testing.TB, tab string) *Model {
	t.Helper()
	detail, diff := ansiPRDetailFixture()
	repo := testutil.Repo("acme/alpha")
	prSvc := &stubPRService{detailResult: detail, diffResult: diff, commits: []domain.Commit{
		{SHA: "a1b2c3d4e5", MessageHeadline: "Clear stale session cookie", AuthorLogin: "octocat", CommittedAt: fixedNow().Add(-3 * time.Hour)},
		{SHA: "f6e5d4c3b2", MessageHeadline: "Add Safari regression test", AuthorLogin: "octocat", CommittedAt: fixedNow().Add(-2 * time.Hour)},
	}}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{repo}, prSvc)
	m.SetTheme(ansiTheme(t))
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view, got %s", m.currentView())
	}
	ansiKeys(m, tab)
	return m
}

// ansiPRDetailSinceReview opens the fixture PR on the Diff tab after the
// viewer (octocat) reviewed an earlier commit and one line changed since.
func ansiPRDetailSinceReview(t *testing.T) *Model {
	t.Helper()
	detail, diff := ansiPRDetailFixture()
	detail.HeadRefOID = "f6e5d4c3b2a1"
	detail.Reviewers = append(detail.Reviewers, domain.PreviewReviewer{
		Login: "octocat", State: "COMMENTED", CommitSHA: "a1b2c3d4e5f6", SubmittedAt: fixedNow().Add(-4 * time.Hour),
	})
	since := deeplinkDiffFixture()
	since.Files[0].Hunks[0].Lines = []diffmodel.DiffLine{
		{Kind: "context", Raw: " func login() {"},
		{Kind: "context", Raw: " \tclearCookie(new line)"},
		{Kind: "addition", Raw: "+\tclearSession()"},
		{Kind: "context", Raw: " }"},
	}
	since.Files[0].DisplayRows = 5
	repo := testutil.Repo("acme/alpha")
	prSvc := &stubPRService{detailResult: detail, diffResult: diff, rangeDiff: diffmodel.RangeDiff{
		Diff: since, FromSHA: "a1b2c3d4e5f6", ToSHA: detail.HeadRefOID, Commits: 1,
	}}
	m := newDeepLinkTestModel(t, 123, []domain.Repository{repo}, prSvc)
	m.SetTheme(ansiTheme(t))
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drainCmds(m, m.Init())
	ansiKeys(m, "2")
	return m
}

// BenchmarkViewPaint compares a diff frame with and without the painted
// background (pho-default paints none; catppuccin-mocha does).
func BenchmarkViewPaint(b *testing.B) {
	for _, name := range []string{theme.DefaultName, "catppuccin-mocha", "catppuccin-mocha/unpainted"} {
		b.Run(name, func(b *testing.B) {
			name, unpainted := strings.CutSuffix(name, "/unpainted")
			prevTheme, prevProfile := ansiThemeName, lipgloss.ColorProfile()
			ansiThemeName = name
			lipgloss.SetColorProfile(termenv.TrueColor)
			b.Cleanup(func() {
				ansiThemeName = prevTheme
				lipgloss.SetColorProfile(prevProfile)
			})
			m := ansiPRDetail(b, "2")
			m.deps.TerminalBackground = unpainted
			b.ResetTimer()
			for range b.N {
				_ = m.View()
			}
		})
	}
}
