package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
	"github.com/utkarsh261/pho/internal/ui/keymap"
)

func nowPtr(t time.Time) *time.Time {
	return &t
}

func batch(cmdsOut ...tea.Cmd) tea.Cmd {
	filtered := make([]tea.Cmd, 0, len(cmdsOut))
	for _, cmd := range cmdsOut {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return tea.Batch(filtered...)
}

func isRootAction(action keymap.Action) bool {
	switch action.(type) {
	case keymap.ToggleCmdPalette, keymap.CloseCmdPalette, keymap.CycleFocus, keymap.TriggerRefresh, keymap.OpenBrowser, keymap.OpenPRDetail, keymap.SelectPR, keymap.Quit, keymap.OpenDashboardFilter, keymap.ToggleKeymapOverlay:
		return true
	default:
		return false
	}
}

func sameRepo(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func findRepoIndex(repos []domain.Repository, fullName string) int {
	for i := range repos {
		if sameRepo(repos[i].FullName, fullName) {
			return i
		}
	}
	return -1
}

func indexOfFocus(focus domain.FocusTarget) int {
	for i, candidate := range dashboardFocusCycle {
		if candidate == focus {
			return i
		}
	}
	return -1
}

func currentTabOrDefault(tab domain.DashboardTab) domain.DashboardTab {
	if strings.TrimSpace(string(tab)) == "" {
		return domain.TabMyPRs
	}
	return tab
}

func clampIndex(index, size int) int {
	if size <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= size {
		return size - 1
	}
	return index
}

func clampScroll(index, current, size int) int {
	if size <= 0 {
		return 0
	}
	if current < 0 {
		current = 0
	}
	if index < current {
		return index
	}
	if index >= current {
		return current
	}
	return current
}

func freshnessFor(err error) domain.Freshness {
	if err == nil {
		return domain.FreshnessFresh
	}
	return domain.FreshnessErrorStale
}

func repoHost(repo domain.Repository) string {
	if host := strings.TrimSpace(repo.Host); host != "" {
		return host
	}
	return "github.com"
}

func jobKey(repo, kind string) string {
	return strings.TrimSpace(repo) + ":" + kind
}

func isZeroDashboardSnapshot(s domain.DashboardSnapshot) bool {
	return strings.TrimSpace(s.Repo.FullName) == "" && len(s.PRs) == 0 && s.TotalCount == 0 && !s.Truncated
}

func isZeroInvolvingSnapshot(s domain.InvolvingSnapshot) bool {
	return strings.TrimSpace(s.Repo.FullName) == "" && len(s.PRs) == 0 && s.TotalCount == 0 && !s.Truncated
}

func isZeroPreviewSnapshot(s domain.PRPreviewSnapshot) bool {
	return strings.TrimSpace(s.Repo) == "" && s.Number == 0 && strings.TrimSpace(s.Title) == "" && strings.TrimSpace(s.BodyExcerpt) == ""
}

func derivedPreview(summary domain.PullRequestSummary) domain.PRPreviewSnapshot {
	return domain.PRPreviewSnapshot{
		Repo:           summary.Repo,
		Number:         summary.Number,
		Title:          summary.Title,
		BodyExcerpt:    "",
		Author:         summary.Author,
		State:          summary.State,
		IsDraft:        summary.IsDraft,
		CIStatus:       summary.CIStatus,
		ReviewDecision: summary.ReviewDecision,
		CreatedAt:      summary.CreatedAt,
		UpdatedAt:      summary.UpdatedAt,
	}
}

func browserURL(repo domain.Repository, fallbackRepo string, number int) string {
	fullName := strings.TrimSpace(repo.FullName)
	if fullName == "" {
		fullName = strings.TrimSpace(fallbackRepo)
	}
	host := repoHost(repo)
	if fullName == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/pull/%d", host, fullName, number)
}

func openURL(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("missing browser URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
