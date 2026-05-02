package cmds

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/domain"
)

func TestCheckoutCommandSameRepo(t *testing.T) {
	name, args, branch, err := checkoutCommand("/workspace/repo", 42, "feature/foo", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "sh" {
		t.Errorf("expected command name 'sh', got %q", name)
	}
	if len(args) != 2 || args[0] != "-c" {
		t.Fatalf("expected sh -c, got %v", args)
	}
	wantSubstr := "git -C /workspace/repo fetch origin feature/foo && git -C /workspace/repo checkout feature/foo"
	if !strings.Contains(args[1], wantSubstr) {
		t.Errorf("expected command to contain %q, got %q", wantSubstr, args[1])
	}
	if branch != "feature/foo" {
		t.Errorf("expected local branch 'feature/foo', got %q", branch)
	}
}

func TestCheckoutCommandCrossRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	name, args, branch, err := checkoutCommand(dir, 42, "feature/foo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "sh" {
		t.Errorf("expected command name 'sh', got %q", name)
	}
	if len(args) != 2 || args[0] != "-c" {
		t.Fatalf("expected sh -c, got %v", args)
	}
	wantSubstr := "git -C " + dir + " fetch origin refs/pull/42/head && git -C " + dir + " checkout -b pr-42 FETCH_HEAD"
	if !strings.Contains(args[1], wantSubstr) {
		t.Errorf("expected command to contain %q, got %q", wantSubstr, args[1])
	}
	if branch != "pr-42" {
		t.Errorf("expected local branch 'pr-42', got %q", branch)
	}
}

func TestCheckoutCommandCrossRepoWithExistingBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Create local branches pr-42 and pr-42-2.
	runGit(t, dir, "checkout", "-b", "pr-42")
	runGit(t, dir, "checkout", "-b", "pr-42-1")
	runGit(t, dir, "checkout", "-b", "pr-42-2")
	runGit(t, dir, "checkout", "main") // back to main so pr-42 and pr-42-2 exist

	name, args, branch, err := checkoutCommand(dir, 42, "feature/foo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "sh" {
		t.Errorf("expected command name 'sh', got %q", name)
	}
	if branch != "pr-42-3" {
		t.Errorf("expected local branch 'pr-42-3', got %q", branch)
	}
	_ = args
}

func TestCheckoutCommandCrossRepoAllSuffixesExist(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	for i := 0; i <= 10; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%d", i)
		}
		runGit(t, dir, "checkout", "-b", "pr-99"+suffix)
	}
	runGit(t, dir, "checkout", "main")

	_, _, _, err := checkoutCommand(dir, 99, "feature/foo", true)
	if err == nil {
		t.Fatal("expected error when all suffixes exist, got nil")
	}
	if !strings.Contains(err.Error(), "pr-99") {
		t.Errorf("expected error to mention 'pr-99', got %v", err)
	}
}

func TestCheckoutBranchCmdNotLocalRepo(t *testing.T) {
	cmd := CheckoutBranchCmd(domain.Repository{LocalPath: ""}, 42, "feature/foo", false)
	msg := cmd()
	res, ok := msg.(CheckoutResult)
	if !ok {
		t.Fatalf("expected CheckoutResult, got %T", msg)
	}
	if res.Err == nil {
		t.Fatal("expected error for empty LocalPath")
	}
	if !strings.Contains(res.Err.Error(), "not a local repo") {
		t.Errorf("expected 'not a local repo' error, got %v", res.Err)
	}
}

func TestCheckoutBranchCmdNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	cmd := CheckoutBranchCmd(domain.Repository{LocalPath: dir}, 42, "feature/foo", false)
	msg := cmd()
	res, ok := msg.(CheckoutResult)
	if !ok {
		t.Fatalf("expected CheckoutResult, got %T", msg)
	}
	if res.Err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(res.Err.Error(), "not a git repo") {
		t.Errorf("expected 'not a git repo' error, got %v", res.Err)
	}
}

func TestCheckoutBranchCmdDirtyTreeAutoRecover(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := CheckoutBranchCmd(domain.Repository{LocalPath: dir}, 42, "feature/foo", false)
	msg := cmd()
	res, ok := msg.(CheckoutResult)
	if !ok {
		t.Fatalf("expected CheckoutResult, got %T", msg)
	}
	if res.Err == nil {
		t.Fatal("expected error because remote branch does not exist")
	}
	// Auto-recovery: stash should have been popped back onto original branch.
	content, err := os.ReadFile(filepath.Join(dir, "dirty.txt"))
	if err != nil {
		t.Fatalf("expected dirty.txt to be restored by stash pop: %v", err)
	}
	if string(content) != "x" {
		t.Fatalf("expected dirty.txt content 'x', got %q", string(content))
	}
}

func TestCheckoutBranchCmdDirtyTreeSuccess(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Set up remote branch.
	remoteDir := t.TempDir()
	initGitRepo(t, remoteDir)
	if err := os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, remoteDir, "add", ".")
	runGit(t, remoteDir, "commit", "-m", "init")
	runGit(t, remoteDir, "checkout", "-b", "feature/foo")
	if err := os.WriteFile(filepath.Join(remoteDir, "foo.txt"), []byte("foo"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, remoteDir, "add", ".")
	runGit(t, remoteDir, "commit", "-m", "foo")
	runGit(t, dir, "remote", "set-url", "origin", remoteDir)

	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := CheckoutBranchCmd(domain.Repository{LocalPath: dir}, 42, "feature/foo", false)
	msg := cmd()
	res, ok := msg.(CheckoutResult)
	if !ok {
		t.Fatalf("expected CheckoutResult, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Branch != "feature/foo" {
		t.Errorf("expected branch 'feature/foo', got %q", res.Branch)
	}
	// Dirty file should be stashed, not in working tree.
	if _, err := os.Stat(filepath.Join(dir, "dirty.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected dirty.txt to be stashed (not in working tree)")
	}
}

func TestCheckoutBranchCmdSameRepoSuccess(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Create a remote branch by making a second clone and pushing a branch.
	remoteDir := t.TempDir()
	initGitRepo(t, remoteDir)
	// Push a branch from remoteDir to dir's origin.
	if err := os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, remoteDir, "add", ".")
	runGit(t, remoteDir, "commit", "-m", "init")
	// Set dir's origin to remoteDir.
	runGit(t, dir, "remote", "set-url", "origin", remoteDir)
	// Push a branch to origin.
	runGit(t, remoteDir, "checkout", "-b", "feature/foo")
	if err := os.WriteFile(filepath.Join(remoteDir, "foo.txt"), []byte("foo"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, remoteDir, "add", ".")
	runGit(t, remoteDir, "commit", "-m", "foo")

	cmd := CheckoutBranchCmd(domain.Repository{LocalPath: dir}, 42, "feature/foo", false)
	msg := cmd()
	res, ok := msg.(CheckoutResult)
	if !ok {
		t.Fatalf("expected CheckoutResult, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Branch != "feature/foo" {
		t.Errorf("expected branch 'feature/foo', got %q", res.Branch)
	}
}

func TestCheckoutBranchCmdCrossRepoSuccess(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	// Create a bare remote with a PR ref.
	remoteDir := t.TempDir()
	initGitRepo(t, remoteDir)
	if err := os.WriteFile(filepath.Join(remoteDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, remoteDir, "add", ".")
	runGit(t, remoteDir, "commit", "-m", "init")
	// Create a fake PR ref.
	runGit(t, remoteDir, "update-ref", "refs/pull/7/head", "HEAD")
	// Point dir's origin to remoteDir.
	runGit(t, dir, "remote", "set-url", "origin", remoteDir)

	cmd := CheckoutBranchCmd(domain.Repository{LocalPath: dir}, 7, "fork/feature", true)
	msg := cmd()
	res, ok := msg.(CheckoutResult)
	if !ok {
		t.Fatalf("expected CheckoutResult, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Branch != "pr-7" {
		t.Errorf("expected branch 'pr-7', got %q", res.Branch)
	}
}

func TestHumanizeGitError(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"fatal: couldn't find remote ref feature/foo", "branch not found on origin"},
		{"Your local changes would be overwritten by checkout", "working tree dirty — stash or commit first"},
		{"Authentication failed for 'https://github.com/'", "network/auth error"},
		{"could not resolve host: github.com", "network/auth error"},
		{"some random git error", "git error: some random git error"},
		{strings.Repeat("a", 200), "git error: " + strings.Repeat("a", 100) + "…"},
	}
	for _, tt := range tests {
		err := parseGitError(tt.input)
		if err.Error() != tt.want {
			t.Errorf("parseGitError(%q) = %q, want %q", tt.input, err.Error(), tt.want)
		}
	}
}

// helpers

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".gitconfig"), []byte("[user]\n\tname = Test\n\temail = test@test.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "user.name", "Test").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", dir).CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	// Create initial commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	// Set main as default branch name for consistency.
	if out, err := exec.Command("git", "-C", dir, "branch", "-M", "main").CombinedOutput(); err != nil {
		t.Fatalf("git branch -M: %v\n%s", err, out)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", all...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
