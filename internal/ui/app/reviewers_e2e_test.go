package app

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/utkarsh261/pho/internal/application/cmds"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/testutil"
	"github.com/utkarsh261/pho/internal/ui/views/dashboard"
)

var updateAppGolden = flag.Bool("update-app", false, "overwrite app golden files with current output")

var e2eAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func e2ePlainText(s string) string {
	return e2eAnsiRe.ReplaceAllString(s, "")
}

func reviewerFixture() domain.PRPreviewSnapshot {
	base := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	return domain.PRPreviewSnapshot{
		Repo:        "acme/api",
		Number:      42,
		Title:       "Fix auth token refresh",
		Author:      "octocat",
		State:       domain.PRStateOpen,
		BodyExcerpt: "Refreshes auth tokens.",
		Reviewers: []domain.PreviewReviewer{
			{Login: "alice", State: "APPROVED", SubmittedAt: base.Add(-2 * time.Hour)},
			{Login: "mac", State: "APPROVED", SubmittedAt: base.Add(-3 * time.Hour)},
			{Login: "dave", State: "CHANGES_REQUESTED", SubmittedAt: base.Add(-1 * time.Hour)},
			{Login: "carol", State: "COMMENTED", Body: "some notes", SubmittedAt: base.Add(-4 * time.Hour)},
		},
		Comments: []domain.PreviewComment{
			{ID: "c1", Login: "bob", Body: "thanks for the PR", CreatedAt: base.Add(-30 * time.Minute)},
		},
	}
}

// openPRDetailE2E drives dashboard → Enter, executes the returned load cmds
// against the stubbed service, and feeds the resulting msgs through the root
// Update loop, as bubbletea would.
func openPRDetailE2E(t *testing.T, m *Model, repo domain.Repository, summary domain.PullRequestSummary) {
	t.Helper()
	m.focus = domain.FocusPRListPanel
	_, _ = m.Update(dashboard.SelectPRMsg{
		Tab:     domain.TabMyPRs,
		Index:   0,
		Repo:    repo.FullName,
		Number:  summary.Number,
		Summary: summary,
	})
	_, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView() != domain.PrimaryViewPRDetail {
		t.Fatalf("expected PR detail view after Enter, got %s", m.currentView())
	}

	detailLoaded := false
	for _, msg := range flattenCmd(enterCmd) {
		switch msg.(type) {
		case cmds.PRDetailLoaded, cmds.DiffLoaded, cmds.CommitsLoaded:
			_, _ = m.Update(msg)
			if _, ok := msg.(cmds.PRDetailLoaded); ok {
				detailLoaded = true
			}
		}
	}
	if !detailLoaded {
		t.Fatal("expected PRDetailLoaded msg from the open-PR cmd batch")
	}
}

func checkAppGolden(t *testing.T, got, name string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name)
	if *updateAppGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update-app to generate)", goldenPath, err)
	}
	if got != string(data) {
		t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, string(data))
	}
}

// TestE2EPRDetailReviewerStrip locks the reviewer strip end to end: usernames,
// state icons, ordering, and the full-screen render.
func TestE2EPRDetailReviewerStrip(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/api")
	summary := pr(repo.FullName, 42, "Fix auth token refresh")
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{summary})
	m.deps.PR.(*stubPRService).detailResult = reviewerFixture()

	openPRDetailE2E(t, m, repo, summary)

	if m.prDetail == nil {
		t.Fatal("expected prDetail to be non-nil")
	}
	if m.prDetail.Detail == nil {
		t.Fatal("expected prDetail.Detail to be loaded")
	}

	view := m.View()
	if h := lipgloss.Height(m.prDetail.View()); h != m.prDetail.Height {
		t.Errorf("PR detail View height = %d, want %d (strip must not overflow the body)", h, m.prDetail.Height)
	}

	plain := e2ePlainText(view)
	for _, want := range []string{"Reviewers", "@alice", "@mac", "! @dave", "· @carol", "· @bob"} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected PR detail view to contain %q\nview:\n%s", want, plain)
		}
	}
	aliceIdx := strings.Index(plain, "@alice")
	daveIdx := strings.Index(plain, "! @dave")
	carolIdx := strings.Index(plain, "· @carol")
	if aliceIdx == -1 || daveIdx == -1 || carolIdx == -1 || aliceIdx > daveIdx || daveIdx > carolIdx {
		t.Errorf("expected reviewer order approved → changes → commented, got strip %q", stripReviewerLines(plain))
	}

	checkAppGolden(t, plain, "reviewers_pr_detail_w120.txt")
}

func stripReviewerLines(plain string) string {
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "Reviewers") {
			return strings.TrimSpace(line)
		}
	}
	return "<no Reviewers line>"
}

func TestE2EPRDetailNoReviewersUnchangedHeader(t *testing.T) {
	t.Parallel()

	repo := testutil.Repo("acme/empty")
	summary := pr(repo.FullName, 7, "No reviews yet")
	detail := domain.PRPreviewSnapshot{
		Repo:        "acme/empty",
		Number:      7,
		Title:       "No reviews yet",
		Author:      "octocat",
		State:       domain.PRStateOpen,
		BodyExcerpt: "Nothing to see.",
	}
	m := setupModelWithPRs(t, []domain.Repository{repo}, []domain.PullRequestSummary{summary})
	m.deps.PR.(*stubPRService).detailResult = detail

	openPRDetailE2E(t, m, repo, summary)

	if plain := e2ePlainText(m.View()); strings.Contains(plain, "Reviewers") {
		t.Errorf("expected no Reviewers line without reviews/comments\nview:\n%s", plain)
	}
}
