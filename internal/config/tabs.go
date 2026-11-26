package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/utkarsh261/pho/internal/domain"
)

type rawTab struct {
	Name  string `toml:"name"`
	Short string `toml:"short"`
	Query string `toml:"query"`
}

// scopeQualifier matches qualifiers that would widen a tab beyond the
// selected repo, which pho always adds itself.
var scopeQualifier = regexp.MustCompile(`(?i)(^|\s)-?(repo|org|user):`)

var issueQualifier = regexp.MustCompile(`(?i)(^|\s)(is|type):issue(\s|$)`)

var quoted = regexp.MustCompile(`"[^"]*"`)

// usesBoolSyntax spots OR/AND and grouping outside quotes. GitHub's GraphQL
// search reads them as plain words, so the tab would silently find nothing.
func usesBoolSyntax(query string) bool {
	for _, tok := range strings.Fields(quoted.ReplaceAllString(query, `""`)) {
		if tok == "OR" || tok == "AND" || strings.HasPrefix(tok, "(") || strings.HasSuffix(tok, ")") {
			return true
		}
	}
	return false
}

func parseTabs(raw []rawTab) ([]domain.SearchTab, error) {
	if len(raw) > domain.MaxSearchTabs {
		return nil, fmt.Errorf("[[dashboard.tabs]]: %d tabs, at most %d allowed", len(raw), domain.MaxSearchTabs)
	}
	reserved := map[string]bool{}
	for _, t := range domain.BuiltinTabs {
		reserved[string(t)] = true
	}
	for _, label := range []string{"my prs", "needs review", "involving", "all", "recent"} {
		reserved[label] = true
	}
	seen := map[string]bool{}
	tabs := make([]domain.SearchTab, 0, len(raw))
	for i, r := range raw {
		name := strings.TrimSpace(r.Name)
		// Keep spacing inside quoted values; only fold line breaks from
		// multi-line TOML strings.
		query := strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(r.Query))
		switch {
		case name == "":
			return nil, fmt.Errorf("[[dashboard.tabs]] #%d: name is required", i+1)
		case query == "":
			return nil, fmt.Errorf("[[dashboard.tabs]] %q: query is required", name)
		case reserved[strings.ToLower(name)]:
			return nil, fmt.Errorf("[[dashboard.tabs]] %q: name is taken by a built-in tab", name)
		case seen[strings.ToLower(name)]:
			return nil, fmt.Errorf("[[dashboard.tabs]] %q: duplicate name", name)
		case usesBoolSyntax(query):
			return nil, fmt.Errorf("[[dashboard.tabs]] %q: OR, AND and parentheses aren't supported; repeat a qualifier to match any of its values (author:a author:b)", name)
		case issueQualifier.MatchString(query):
			return nil, fmt.Errorf("[[dashboard.tabs]] %q: tabs list pull requests; drop is:issue/type:issue", name)
		case scopeQualifier.MatchString(query):
			return nil, fmt.Errorf("[[dashboard.tabs]] %q: drop repo:/org:/user:, tabs always search the selected repo", name)
		}
		seen[strings.ToLower(name)] = true
		tabs = append(tabs, domain.SearchTab{
			ID:    domain.CustomTabID(name),
			Name:  name,
			Short: strings.TrimSpace(r.Short),
			Query: query,
		})
	}
	return tabs, nil
}

// ResolveDefaultTab maps a default_tab value to a tab: a built-in id or a
// custom tab's name, matched case-insensitively.
func ResolveDefaultTab(value string, tabs []domain.SearchTab) (domain.DashboardTab, bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return domain.TabMyPRs, true
	}
	for _, t := range domain.BuiltinTabs {
		if string(t) == v {
			return t, true
		}
	}
	for _, t := range tabs {
		if strings.ToLower(t.Name) == v {
			return t.ID, true
		}
	}
	return domain.TabMyPRs, false
}
