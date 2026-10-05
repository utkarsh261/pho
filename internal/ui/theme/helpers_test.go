package theme

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestRenderHintsFormatsKeyDescPairs(t *testing.T) {
	t.Parallel()
	th := Default()
	got := th.RenderHints("Tab: Switch | Esc / q: Back   1/2/3/4: Tabs")
	if want := "Tab Switch  ·  Esc / q Back  ·  1/2/3/4 Tabs"; plain(got) != want {
		t.Errorf("plain = %q, want %q", plain(got), want)
	}
	if !strings.Contains(got, th.HintKey.Render("Tab")) {
		t.Errorf("expected key styled with HintKey, got %q", got)
	}
}

func TestRenderHintsKeepsPromptsReadable(t *testing.T) {
	t.Parallel()
	th := Default()
	for _, prompt := range []string{
		"Close #9? (y/n)",
		"Merge #9: [s]quash [r]ebase [M]erge [esc]cancel",
		"Checking mergeability...",
		"Failed: GraphQL: Could not resolve to a node with the global id",
		"fatal: not a git repository (or any of the parent directories)",
		"Failed: rate limited | retry in 30s",
	} {
		got := th.RenderHints(prompt)
		if plain(got) != prompt {
			t.Errorf("prompt %q rendered as %q", prompt, plain(got))
		}
		if got != th.StatusHelp.Render(prompt) {
			t.Errorf("prompt %q should use StatusHelp, got %q", prompt, got)
		}
	}
	for in, want := range map[string]string{
		"Tab: Switch | Space: Visual | 1/2/3/4: Tabs | R: Refresh | /: Search | ?": "Tab Switch  ·  Space Visual  ·  1/2/3/4 Tabs  ·  R Refresh  ·  / Search  ·  ?",
		"Tab: Next field   ←/→: Change option   Ctrl+S: Create PR   Esc: Cancel":   "Tab Next field  ·  ←/→ Change option  ·  Ctrl+S Create PR  ·  Esc Cancel",
		"2 inline drafts belong to the previous head | D: Discard drafts":          "2 inline drafts belong to the previous head  ·  D Discard drafts",
		"Search: foo  (1/3)  | Enter: commit  | Esc: clear":                        "Search: foo  (1/3)  ·  Enter commit  ·  Esc clear",
	} {
		if got := plain(th.RenderHints(in)); got != want {
			t.Errorf("RenderHints(%q) = %q, want %q", in, got, want)
		}
	}
	if got := plain(th.RenderHints("Esc: Dismiss")); got != "Esc Dismiss" {
		t.Errorf("single key hint = %q, want %q", got, "Esc Dismiss")
	}
	if th.RenderHints("   ") != "" {
		t.Error("blank hint should render empty")
	}
}

func TestFillBgKeepsBackgroundAcrossResetsAndWidth(t *testing.T) {
	t.Parallel()
	th := Default()
	styled := lipgloss.NewStyle().Foreground(th.Primary).Render("abc") + " tail"
	got := FillBg(th.Highlight, 20, styled)
	if w := lipgloss.Width(got); w != 20 {
		t.Errorf("width = %d, want 20", w)
	}
	if long := FillBg(th.Highlight, 5, strings.Repeat("x", 40)); lipgloss.Width(long) != 5 {
		t.Errorf("long input not truncated to 5: %q", plain(long))
	}
	if FillBg(th.Highlight, 0, "x") != "" {
		t.Error("zero width should render empty")
	}
	// With colour enabled, every reset inside s must be followed by the bg again.
	probe := lipgloss.NewStyle().Background(th.Highlight).Render("x")
	if i := strings.Index(probe, "x"); i > 0 {
		bg := probe[:i]
		for _, part := range strings.Split(got, "\x1b[0m")[1:] {
			if part != "" && !strings.HasPrefix(part, bg) {
				t.Errorf("background not restored after reset: %q", part)
			}
		}
	}
}
