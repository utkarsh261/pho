package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/utkarsh261/pho/internal/domain"
)

const customTabOp = "graphql CustomTabPRsQuery"

const teamTabs = `
[[dashboard.tabs]]
name = "Team"
query = "author:alice author:bob"
`

// Two PRs per repo by the team; the rest stay the viewer's, so My PRs
// still lists them.
func teamAuthors(prs []domain.PullRequestSummary) {
	for i := range prs {
		switch i {
		case 0:
			prs[i].Author, prs[i].Title = "alice", "alice change"
		case 1:
			prs[i].Author, prs[i].Title = "bob", "bob change"
		}
	}
}

// A custom tab costs no request until it's opened, then reads its search
// from the cache when reopened.
func TestE2ECustomTabLoadsLazily(t *testing.T) {
	e := startCustom(t, customSetup{toml: teamTabs, editPRs: teamAuthors})
	if n := e.srv.Requests()[customTabOp]; n != 0 {
		t.Fatalf("startup ran %d custom tab searches: %s", n, e.srv.Summary())
	}

	e.p.Send(tea.KeyMsg{Type: tea.KeyTab}) // focus the PR list
	from := e.mark()
	e.press("l", "l", "l", "l", "l")
	e.waitText(from, "alice change")
	quiesce(e.srv, 200*time.Millisecond, 10*time.Second)
	if line := e.lastLineWith(from, "Team"); !strings.Contains(line, "Team 2") {
		t.Errorf("tab bar = %q, want Team 2", line)
	}
	searches := e.srv.Searches()
	last := searches[len(searches)-1]
	if !strings.HasPrefix(last, "repo:") || !strings.HasSuffix(last, "is:pr is:open author:alice author:bob sort:updated-desc") {
		t.Errorf("search = %q", last)
	}
	if n := e.srv.Requests()[customTabOp]; n != 1 {
		t.Fatalf("custom tab searches = %d, want 1", n)
	}

	from = e.mark()
	e.press("h", "l")
	e.waitText(from, "alice change")
	quiesce(e.srv, 200*time.Millisecond, 10*time.Second)
	if n := e.srv.Requests()[customTabOp]; n != 1 {
		t.Fatalf("reopening the tab searched again: %d searches", n)
	}
}

func TestE2ECustomTabDefault(t *testing.T) {
	e := startCustom(t, customSetup{toml: "[dashboard]\ndefault_tab = \"team\"\n" + teamTabs, editPRs: teamAuthors})
	e.waitText(0, "alice change")
	if n := e.srv.Requests()[customTabOp]; n != 1 {
		t.Fatalf("custom tab searches = %d, want 1: %s", n, e.srv.Summary())
	}
}
