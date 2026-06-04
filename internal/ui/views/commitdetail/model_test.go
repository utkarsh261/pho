package commitdetail

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/application/cmds"
	diffmodel "github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/theme"
	"github.com/utkarsh261/pho/internal/ui/views/prdetail"
)

var updateCommitDetailGolden = flag.Bool("update-commitdetail", false, "overwrite commitdetail golden files")

var cdAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripCommitDetailANSI(s string) string {
	return cdAnsiRe.ReplaceAllString(s, "")
}

func checkCommitDetailGolden(t *testing.T, got, name string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name)
	if *updateCommitDetailGolden {
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
		t.Fatalf("read golden %s: %v (run with -update-commitdetail to generate)", goldenPath, err)
	}
	if got != string(data) {
		t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, string(data))
	}
}

func makeTestCommit() domain.Commit {
	return domain.Commit{
		SHA:             "abc1234def5678",
		MessageHeadline: "feat: add user authentication",
		AuthorName:      "Alice Smith",
		AuthorLogin:     "alice",
		CommittedAt:     time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	}
}

func makeTestDiffModel() *diffmodel.DiffModel {
	return &diffmodel.DiffModel{
		Files: []diffmodel.DiffFile{
			{
				OldPath:   "auth.go",
				NewPath:   "auth.go",
				Status:    "modified",
				Additions: 5,
				Deletions: 2,
				Hunks: []diffmodel.DiffHunk{
					{
						Header: "@@ -1,5 +1,8 @@",
						Lines: []diffmodel.DiffLine{
							{Kind: "context", Raw: " package main"},
							{Kind: "addition", Raw: "+import \"context\""},
							{Kind: "context", Raw: " "},
							{Kind: "deletion", Raw: "-func login() {"},
							{Kind: "addition", Raw: "+func login(ctx context.Context) {"},
							{Kind: "context", Raw: " \t// TODO"},
						},
					},
				},
			},
			{
				OldPath:   "utils.go",
				NewPath:   "utils.go",
				Status:    "modified",
				Additions: 1,
				Deletions: 0,
				Hunks: []diffmodel.DiffHunk{
					{
						Header: "@@ -10,3 +10,4 @@",
						Lines: []diffmodel.DiffLine{
							{Kind: "context", Raw: " func helper() {"},
							{Kind: "addition", Raw: "+\treturn nil"},
						},
					},
				},
			},
		},
		Stats: diffmodel.DiffStats{TotalFiles: 2},
	}
}

func TestNewModelInitializesNoLoading(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, domain.Commit{SHA: "abc123"}, nil)
	if m.Inner().DiffLoading {
		t.Fatal("expected DiffLoading=false when PRService is nil")
	}
}

func TestInitReturnsCommand(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, domain.Commit{SHA: "abc123"}, &fakePRService{})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected non-nil Init cmd")
	}
	if !m.Inner().DiffLoading {
		t.Fatal("expected DiffLoading=true after Init")
	}
}

func TestCommitDiffLoadedSetsDiff(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, domain.Commit{SHA: "abc123"}, &fakePRService{})
	m.SetTheme(theme.Default())

	next, _ := m.Update(cmds.CommitDiffLoaded{
		Repo: "owner/repo",
		SHA:  "abc123",
		Diff: diffmodel.DiffModel{
			Files: []diffmodel.DiffFile{
				{OldPath: "a.go", NewPath: "a.go", Status: "modified"},
			},
		},
	})
	m = next
	if m.Inner().DiffLoading {
		t.Fatal("expected DiffLoading=false after CommitDiffLoaded")
	}
	if m.Inner().Diff == nil {
		t.Fatal("expected Diff to be set")
	}
	if len(m.Inner().Diff.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(m.Inner().Diff.Files))
	}
}

func TestEscEmitsBackToPRDetail(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, domain.Commit{SHA: "abc123"}, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for Esc")
	}
	msg := cmd()
	_, ok := msg.(prdetail.BackToPRDetail)
	if !ok {
		t.Fatalf("expected BackToPRDetail, got %T", msg)
	}
}

func TestOEmitsOpenBrowserCommit(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, domain.Commit{SHA: "abc123"}, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for o")
	}
	msg := cmd()
	openMsg, ok := msg.(prdetail.OpenBrowserCommit)
	if !ok {
		t.Fatalf("expected OpenBrowserCommit, got %T", msg)
	}
	if openMsg.SHA != "abc123" {
		t.Errorf("expected SHA=abc123, got %q", openMsg.SHA)
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

func TestCommitDetailGoldenLoading(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, makeTestCommit(), nil)
	m.SetTheme(theme.Default())
	inner := m.Inner()
	inner.Width = 100
	inner.Height = 24
	inner.DiffLoading = true
	inner.SetDiffFiles([]diffmodel.DiffFile{
		{OldPath: "auth.go", NewPath: "auth.go", Status: "modified"},
	})

	got := stripCommitDetailANSI(m.View())
	checkCommitDetailGolden(t, got, "commitdetail_loading.txt")
}

func TestCommitDetailGoldenEmptyDiff(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, makeTestCommit(), nil)
	m.SetTheme(theme.Default())
	inner := m.Inner()
	inner.Width = 100
	inner.Height = 24
	inner.DiffLoading = false
	m.SetDiff(&diffmodel.DiffModel{Files: nil, Stats: diffmodel.DiffStats{}})
	inner.SetDiffFiles(nil)

	got := stripCommitDetailANSI(m.View())
	checkCommitDetailGolden(t, got, "commitdetail_empty_diff.txt")
}

func TestCommitDetailGoldenWithDiff(t *testing.T) {
	t.Parallel()
	m := NewModel(domain.Repository{FullName: "owner/repo"}, makeTestCommit(), nil)
	m.SetTheme(theme.Default())
	inner := m.Inner()
	inner.Width = 100
	inner.Height = 24
	inner.DiffLoading = false
	m.SetDiff(makeTestDiffModel())
	inner.SetDiffFiles(inner.Diff.Files)

	got := stripCommitDetailANSI(m.View())
	checkCommitDetailGolden(t, got, "commitdetail_with_diff.txt")
}

func TestCommitDetailGoldenWidths(t *testing.T) {
	widths := []int{79, 100, 120}
	for _, w := range widths {
		w := w
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			t.Parallel()
			m := NewModel(domain.Repository{FullName: "owner/repo"}, makeTestCommit(), nil)
			m.SetTheme(theme.Default())
			inner := m.Inner()
			inner.Width = w
			inner.Height = 24
			inner.DiffLoading = false
			m.SetDiff(makeTestDiffModel())
			inner.SetDiffFiles(inner.Diff.Files)

			got := stripCommitDetailANSI(m.View())
			checkCommitDetailGolden(t, got, fmt.Sprintf("commitdetail_with_diff_w%d.txt", w))
		})
	}
}
