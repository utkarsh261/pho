package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
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
	m.SetRect(240)

	view := m.View()
	for _, want := range []string{
		"j/k: Navigate",
		"Tab: Next panel",
		"stale",
		"auth:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in %q", want, view)
		}
	}
	// Spinner uses ∙ (Points) or ⣾ (Dot) characters.
	if !strings.ContainsAny(view, "∙⣾") {
		t.Fatalf("expected spinner loading indicator in %q", view)
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
	m.SetRect(200)

	view := m.View()
	if !strings.Contains(view, "rate limit: retry at 2026-04-09 15:30 UTC") {
		t.Fatalf("expected rate limit reset message, got %q", view)
	}
}

func TestSetSearchStateTwoOfFive(t *testing.T) {
	t.Parallel()

	m := NewStatusBarModel()
	m.Focus = domain.FocusPRListPanel
	m.SetRect(120)
	m.SetSearchState("error", 2, 5)

	view := m.View()
	if !strings.Contains(view, "/ error  2/5 matches") {
		t.Fatalf("expected search state in status bar, got %q", view)
	}
}

func TestSetSearchStateZeroMatches(t *testing.T) {
	t.Parallel()

	m := NewStatusBarModel()
	m.Focus = domain.FocusPRListPanel
	m.SetRect(120)
	m.SetSearchState("xyzquux", 1, 0)

	view := m.View()
	if !strings.Contains(view, "/ xyzquux  0 matches") {
		t.Fatalf("expected zero-match text in status bar, got %q", view)
	}
}

func TestSetSearchStateLongQueryTruncated(t *testing.T) {
	t.Parallel()

	m := NewStatusBarModel()
	m.Focus = domain.FocusPRListPanel
	m.SetRect(60)

	long := "this-is-a-very-long-query-that-should-be-truncated-in-status"
	m.SetSearchState(long, 1, 3)
	view := m.View()

	if !strings.Contains(view, "…") {
		t.Fatalf("expected ellipsis in truncated query, got %q", view)
	}
	if strings.Contains(view, long) {
		t.Fatalf("expected full query to be truncated, got %q", view)
	}
}

func TestSetSearchStateClearedRestoresHelp(t *testing.T) {
	t.Parallel()

	m := NewStatusBarModel()
	m.Focus = domain.FocusPRListPanel
	m.SetRect(120)
	m.SetSearchState("error", 1, 5)
	m.SetSearchState("", 0, 0)

	view := m.View()
	if !strings.Contains(view, "j/k: Navigate") {
		t.Fatalf("expected default help text after clear, got %q", view)
	}
	if strings.Contains(view, "/ error") {
		t.Fatalf("expected search state to be cleared, got %q", view)
	}
}

func TestStatusBarNotice(t *testing.T) {
	t.Parallel()

	m := NewStatusBarModel()
	m.Focus = domain.FocusPRListPanel
	m.Errors = domain.ErrorState{
		Errors: []domain.AppError{{Kind: domain.ErrorKindNetwork, Message: "offline"}},
	}
	m.Notice = "✦ Updated to 1.2.0 | W: What's new"
	m.SetRect(240)

	view := m.View()
	hints := strings.Index(view, "j/k: Navigate")
	errAt := strings.Index(view, "network: offline")
	notice := strings.Index(view, "✦ Updated to 1.2.0")
	if hints < 0 || errAt < 0 || notice < 0 {
		t.Fatalf("missing a segment in %q", view)
	}
	if notice < errAt {
		t.Errorf("notice comes before the error, so a narrow terminal would cut the error first: %q", view)
	}

	// Narrow: the hints survive and the notice is what gets cut.
	m.Errors = domain.ErrorState{}
	m.SetRect(len("j/k: Navigate | h/l: Tab | Enter: Open | Tab: Next panel | ?: Keymap") + 8)
	view = m.View()
	if !strings.Contains(view, "?: Keymap") || strings.Contains(view, "What's new") {
		t.Errorf("narrow bar should keep hints and cut the notice: %q", view)
	}

	m.Notice = ""
	m.SetRect(240)
	if strings.Contains(m.View(), "Updated") {
		t.Error("empty notice still drawn")
	}
}
