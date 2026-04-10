package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/utk/git-term/internal/domain"
)

func TestStatusBarFocusLoadingAndAuthError(t *testing.T) {
	t.Parallel()

	m := NewStatusBarModel()
	m.Focus = domain.FocusPRListPanel
	m.Loading = true
	m.Freshness = domain.FreshnessStale
	m.Errors = domain.ErrorState{
		Errors: []domain.AppError{{Kind: domain.ErrorKindAuth, Message: "gh auth login required"}},
	}
	m.SetRect(120)

	view := m.View()
	for _, want := range []string{
		"j/k: Navigate",
		"o: Open browser",
		"r: Refresh",
		"Tab: Next panel",
		"loading",
		"stale",
		"auth:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in %q", want, view)
		}
	}
}

func TestStatusBarRateLimitError(t *testing.T) {
	t.Parallel()

	reset := time.Date(2026, 4, 9, 15, 30, 0, 0, time.UTC)
	m := NewStatusBarModel()
	m.Focus = domain.FocusRepoPanel
	m.Errors = domain.ErrorState{
		Errors:         []domain.AppError{{Kind: domain.ErrorKindRateLimit, Message: "API limit reached"}},
		RateLimitReset: &reset,
	}
	m.SetRect(120)

	view := m.View()
	if !strings.Contains(view, "rate limit: retry at 2026-04-09 15:30 UTC") {
		t.Fatalf("expected rate limit reset message, got %q", view)
	}
}
