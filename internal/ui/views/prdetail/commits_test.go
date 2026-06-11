package prdetail

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
)

var updateCommitsGolden = flag.Bool("update-commits", false, "overwrite commits golden files")

var commitsAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripCommitsANSI(s string) string {
	return commitsAnsiRe.ReplaceAllString(s, "")
}

func TestCommitsTabSwitchTriggersLoading(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.PRService = &fakePRService{}
	m.SetTheme(theme.Default())

	m = pressKey(m, "4")
	if m.activeTab != TabCommits {
		t.Fatalf("expected activeTab=TabCommits, got %d", m.activeTab)
	}
	if !m.commitsLoading {
		t.Fatal("expected commitsLoading=true after switching to Commits tab")
	}
}

func TestCommitsTabAlreadyLoadedDoesNotRefetch(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.PRService = &fakePRService{}
	m.SetTheme(theme.Default())
	m.commitsLoaded = true
	m.commits = []domain.Commit{
		{SHA: "abc1234", MessageHeadline: "Test commit"},
	}

	m = pressKey(m, "4")
	if m.commitsLoading {
		t.Fatal("expected commitsLoading=false when commits already loaded")
	}
}

func TestCommitsTabJMovesCursorDown(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = []domain.Commit{
		{SHA: "abc1234", MessageHeadline: "First"},
		{SHA: "def5678", MessageHeadline: "Second"},
		{SHA: "ghi9abc", MessageHeadline: "Third"},
	}
	m.commitCursor = 0
	m.leftPanel.Focus = FocusContent

	m = pressKey(m, "j")
	if m.commitCursor != 1 {
		t.Fatalf("expected commitCursor=1 after j, got %d", m.commitCursor)
	}
}

func TestCommitsTabKMovesCursorUp(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = []domain.Commit{
		{SHA: "abc1234", MessageHeadline: "First"},
		{SHA: "def5678", MessageHeadline: "Second"},
	}
	m.commitCursor = 1
	m.leftPanel.Focus = FocusContent

	m = pressKey(m, "k")
	if m.commitCursor != 0 {
		t.Fatalf("expected commitCursor=0 after k, got %d", m.commitCursor)
	}
}

func TestCommitsTabEnterEmitsOpenCommitDetail(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = []domain.Commit{
		{SHA: "abc1234", MessageHeadline: "Test commit"},
	}
	m.commitCursor = 0
	m.leftPanel.Focus = FocusContent

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for Enter on commit")
	}
	msg := cmd()
	openMsg, ok := msg.(OpenCommitDetail)
	if !ok {
		t.Fatalf("expected OpenCommitDetail, got %T", msg)
	}
	if openMsg.Commit.SHA != "abc1234" {
		t.Errorf("expected SHA=abc1234, got %q", openMsg.Commit.SHA)
	}
}

func TestCommitsTabEmptyState(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = nil

	lines := m.renderCommitsTab(0, 10, 80)
	found := false
	for _, line := range lines {
		if strContains(line, "No commits") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'No commits' in rendered output, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestCommitsTabHandlesCommitsLoaded(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoading = true

	next, _ := m.Update(cmds.CommitsLoaded{
		Repo:   "owner/repo",
		Number: 42,
		Commits: []domain.Commit{
			{SHA: "abc1234", MessageHeadline: "Test commit"},
		},
	})
	m = next
	if m.commitsLoading {
		t.Fatal("expected commitsLoading=false after CommitsLoaded")
	}
	if len(m.commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(m.commits))
	}
}

type fakePRService struct {
	postReviewCommentCalled        bool
	submitReviewWithCommentsCalled bool
}

func (f *fakePRService) LoadDetail(ctx context.Context, repo domain.Repository, number int, force bool) (domain.PRPreviewSnapshot, bool, error) {
	return domain.PRPreviewSnapshot{}, false, nil
}
func (f *fakePRService) LoadDiff(ctx context.Context, repo domain.Repository, number int, headSHA string, force bool) (diffmodel.DiffModel, bool, error) {
	return diffmodel.DiffModel{}, false, nil
}
func (f *fakePRService) LoadPRCommits(ctx context.Context, repo domain.Repository, number int, force bool) ([]domain.Commit, error) {
	return nil, nil
}
func (f *fakePRService) LoadCommitDiff(ctx context.Context, repo domain.Repository, sha string, force bool) (diffmodel.DiffModel, error) {
	return diffmodel.DiffModel{}, nil
}
func (f *fakePRService) PostComment(ctx context.Context, repo domain.Repository, prID string, body string) error {
	return nil
}
func (f *fakePRService) PostCommentReply(_ context.Context, _ domain.Repository, _, _, _ string) error {
	return nil
}
func (f *fakePRService) PostReviewComment(ctx context.Context, repo domain.Repository, prID string, body string) error {
	f.postReviewCommentCalled = true
	return nil
}
func (f *fakePRService) PostThreadReply(_ context.Context, _ domain.Repository, _, _ string) error {
	return nil
}
func (f *fakePRService) ApprovePR(ctx context.Context, repo domain.Repository, prID string, body string) error {
	return nil
}
func (f *fakePRService) SubmitReviewWithComments(ctx context.Context, repo domain.Repository, prID, body, event string, comments []domain.DraftInlineComment) error {
	f.submitReviewWithCommentsCalled = true
	return nil
}
func (f *fakePRService) SaveDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string, drafts []domain.DraftInlineComment) error {
	return nil
}
func (f *fakePRService) LoadDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) ([]domain.DraftInlineComment, error) {
	return nil, nil
}
func (f *fakePRService) DeleteDraftComments(ctx context.Context, repo domain.Repository, number int, headSHA string) error {
	return nil
}
func (f *fakePRService) MergePR(ctx context.Context, repo domain.Repository, number int, prID string, headRefOID string, method string) error {
	return nil
}
func (f *fakePRService) CheckMergeable(ctx context.Context, repo domain.Repository, number int) (domain.MergeableState, error) {
	return domain.MergeableState{}, nil
}
func (f *fakePRService) ClosePR(ctx context.Context, repo domain.Repository, number int, prID string) error {
	return nil
}
func (f *fakePRService) ReopenPR(ctx context.Context, repo domain.Repository, number int, prID string) error {
	return nil
}

func (f *fakePRService) UpdatePR(_ context.Context, _ domain.Repository, _ int, _ string, _ string, _ string) error {
	return nil
}
func (f *fakePRService) CreatePR(_ context.Context, _ domain.CreatePRParams) (domain.PullRequestSummary, error) {
	return domain.PullRequestSummary{}, nil
}
func (f *fakePRService) FetchRepoInfo(_ context.Context, _ domain.Repository) (domain.RepoInfo, error) {
	return domain.RepoInfo{}, nil
}

func strContains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func checkCommitsGolden(t *testing.T, got, name string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name)
	if *updateCommitsGolden {
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
		t.Fatalf("read golden %s: %v (run with -update-commits to generate)", goldenPath, err)
	}
	if got != string(data) {
		t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, string(data))
	}
}

func makeTestCommits() []domain.Commit {
	base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	return []domain.Commit{
		{
			SHA:             "abc1234def5678",
			MessageHeadline: "feat: add user authentication",
			AuthorName:      "Alice Smith",
			AuthorLogin:     "alice",
			CommittedAt:     base,
		},
		{
			SHA:             "def5678ghi9abc",
			MessageHeadline: "fix: resolve nil pointer in parser",
			AuthorName:      "Bob Jones",
			AuthorLogin:     "bob",
			CommittedAt:     base.Add(-2 * time.Hour),
		},
		{
			SHA:             "ghi9abcjkl0def",
			MessageHeadline: "docs: update README with examples",
			AuthorName:      "Carol White",
			AuthorLogin:     "carol",
			CommittedAt:     base.Add(-24 * time.Hour),
		},
	}
}

func TestCommitsTabGoldenEmpty(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = nil

	lines := m.renderCommitsTab(0, 8, 80)
	got := stripCommitsANSI(strings.Join(lines, "\n"))
	checkCommitsGolden(t, got, "commits_empty.txt")
}

func TestCommitsTabGoldenLoading(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoading = true

	lines := m.renderCommitsTab(0, 8, 80)
	got := stripCommitsANSI(strings.Join(lines, "\n"))
	checkCommitsGolden(t, got, "commits_loading.txt")
}

func TestCommitsTabGoldenNormal(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = makeTestCommits()
	m.commitCursor = 0

	lines := m.renderCommitsTab(0, 10, 80)
	got := stripCommitsANSI(strings.Join(lines, "\n"))
	checkCommitsGolden(t, got, "commits_normal.txt")
}

func TestCommitsTabGoldenSelectedSecond(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = makeTestCommits()
	m.commitCursor = 1

	lines := m.renderCommitsTab(0, 10, 80)
	got := stripCommitsANSI(strings.Join(lines, "\n"))
	checkCommitsGolden(t, got, "commits_selected_second.txt")
}

func TestCommitsTabGoldenScrolled(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = makeTestCommits()
	m.commitCursor = 2
	// Each commit is 3 rows; scroll so only the last commit is fully visible.
	m.ContentScroll = 3

	lines := m.renderCommitsTab(m.ContentScroll, 6, 80)
	got := stripCommitsANSI(strings.Join(lines, "\n"))
	checkCommitsGolden(t, got, "commits_scrolled.txt")
}

func TestCommitsTabGoldenWidths(t *testing.T) {
	widths := []int{60, 80, 120}
	for _, w := range widths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := makePRDetail(w+10, 30, nil, nil)
			m.SetTheme(theme.Default())
			m.activeTab = TabCommits
			m.commitsLoaded = true
			m.commits = makeTestCommits()
			m.commitCursor = 0

			lines := m.renderCommitsTab(0, 10, w)
			got := stripCommitsANSI(strings.Join(lines, "\n"))
			checkCommitsGolden(t, got, fmt.Sprintf("commits_normal_w%d.txt", w))
		})
	}
}

func TestHandleRefreshOnCommitsTabFiresCommitsReload(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.PRService = &fakePRService{}
	m.SetTheme(theme.Default())
	m.activeTab = TabCommits
	m.commitsLoaded = true
	m.commits = []domain.Commit{{SHA: "abc1234", MessageHeadline: "Test commit"}}

	_, cmd := m.handleRefresh()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from handleRefresh on commits tab")
	}
	if !m.commitsLoading {
		t.Fatal("expected commitsLoading=true after handleRefresh on commits tab")
	}
}

func TestHandleCommitRefreshFiresCommitDiffReload(t *testing.T) {
	t.Parallel()
	m := makePRDetail(100, 30, nil, nil)
	m.PRService = &fakePRService{}
	m.SetTheme(theme.Default())
	m.CommitMode = true
	m.Commit = domain.Commit{SHA: "abc1234", MessageHeadline: "Test commit"}
	m.Diff = &diffmodel.DiffModel{HeadSHA: "old-sha"}

	_, cmd := m.handleCommitRefresh()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from handleCommitRefresh")
	}
	if !m.DiffLoading {
		t.Fatal("expected DiffLoading=true after handleCommitRefresh")
	}
}
