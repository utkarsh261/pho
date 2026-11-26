package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utkarsh261/pho/internal/config"
	"github.com/utkarsh261/pho/internal/domain"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Tabs(t *testing.T) {
	t.Run("loads", func(t *testing.T) {
		cfg, err := config.Load(writeConfig(t, `
[[dashboard.tabs]]
name = " Team "
query = "author:alice   author:bob"

[[dashboard.tabs]]
name = "Docs"
short = "D"
query = "label:docs"
`))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		want := []domain.SearchTab{
			{ID: "custom:team", Name: "Team", Query: "author:alice   author:bob"},
			{ID: "custom:docs", Name: "Docs", Short: "D", Query: "label:docs"},
		}
		if fmt.Sprint(cfg.Dashboard.Tabs) != fmt.Sprint(want) {
			t.Fatalf("Tabs = %+v, want %+v", cfg.Dashboard.Tabs, want)
		}
	})

	tab := func(name, query string) string {
		return fmt.Sprintf("[[dashboard.tabs]]\nname = %q\nquery = %q\n", name, query)
	}
	for _, tc := range []struct{ name, body, want string }{
		{"no name", tab("", "author:a"), "name is required"},
		{"no query", tab("Team", " "), "query is required"},
		{"builtin name", tab("Needs review", "author:a"), "built-in"},
		{"builtin id", tab("my_prs", "author:a"), "built-in"},
		{"duplicate", tab("Team", "author:a") + tab("team", "author:b"), "duplicate"},
		{"repo scope", tab("Team", "author:a repo:x/y"), "repo:"},
		{"org scope", tab("Team", "org:acme"), "repo:"},
		{"or", tab("Team", "label:bug OR label:urgent"), "OR, AND and parentheses"},
		{"parens", tab("Team", "(author:a)"), "OR, AND and parentheses"},
		{"issues", tab("Team", "is:issue label:bug"), "tabs list pull requests"},
		{"issue type", tab("Team", "label:bug type:issue"), "tabs list pull requests"},
		{"too many", tab("a", "q") + tab("b", "q") + tab("c", "q") + tab("d", "q") + tab("e", "q") + tab("f", "q"), "at most 5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want one containing %q", err, tc.want)
			}
		})
	}

	// "repository:" or "labels:" text isn't a scope qualifier.
	t.Run("lookalike qualifier", func(t *testing.T) {
		if _, err := config.Load(writeConfig(t, tab("Team", `label:user-facing in:title myrepo:x label:"needs (triage)" label:ORACLE`))); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})

	t.Run("docs example", func(t *testing.T) {
		text, err := os.ReadFile(filepath.Join("..", "..", "docs", "custom-tabs.md"))
		if err != nil {
			t.Fatal(err)
		}
		_, block, ok := strings.Cut(string(text), "```toml\n")
		if !ok {
			t.Fatal("no toml example")
		}
		block, _, _ = strings.Cut(block, "```")
		cfg, err := config.Load(writeConfig(t, block))
		if err != nil {
			t.Fatalf("example doesn't load: %v", err)
		}
		if len(cfg.Dashboard.Tabs) == 0 {
			t.Fatal("example has no tabs")
		}
		if _, ok := config.ResolveDefaultTab(cfg.Dashboard.DefaultTab, cfg.Dashboard.Tabs); !ok {
			t.Fatalf("example default_tab %q doesn't resolve", cfg.Dashboard.DefaultTab)
		}
	})
}

func TestResolveDefaultTab(t *testing.T) {
	tabs := []domain.SearchTab{{ID: domain.CustomTabID("Team"), Name: "Team"}}
	for _, tc := range []struct {
		value string
		want  domain.DashboardTab
		ok    bool
	}{
		{"", domain.TabMyPRs, true},
		{"needs_review", domain.TabNeedsReview, true},
		{" Involving ", domain.TabInvolving, true},
		{"team", "custom:team", true},
		{"nope", domain.TabMyPRs, false},
	} {
		got, ok := config.ResolveDefaultTab(tc.value, tabs)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ResolveDefaultTab(%q) = %q, %v; want %q, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}
