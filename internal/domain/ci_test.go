package domain

import "testing"

func TestActionsJobID(t *testing.T) {
	tests := []struct {
		url    string
		repo   string
		want   int64
		wantOK bool
	}{
		{"https://github.com/acme/hooks/actions/runs/123/job/456", "acme/hooks", 456, true},
		{"https://github.com/Acme/Hooks/actions/runs/123/job/456", "acme/hooks", 456, true},
		{"https://ghe.example.com/acme/hooks/actions/runs/1/job/99?pr=7", "acme/hooks", 99, true},
		{"https://github.com/other/repo/actions/runs/123/job/456", "acme/hooks", 0, false},
		{"https://ci.example.com/proxy/acme/hooks/actions/runs/1/job/2", "acme/hooks", 0, false},
		{"https://app.codecov.io/gh/acme/hooks/pull/7", "acme/hooks", 0, false},
		{"https://github.com/acme/hooks/actions/runs/123", "acme/hooks", 0, false},
		{"", "acme/hooks", 0, false},
		{"https://github.com/acme/hooks/actions/runs/1/job/2", "", 0, false},
	}
	for _, tt := range tests {
		got, ok := ActionsJobID(tt.url, tt.repo)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ActionsJobID(%q, %q) = %d, %v; want %d, %v", tt.url, tt.repo, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestActionsIDsAcceptAnyRepoName(t *testing.T) {
	// A renamed repo: the remote has the old name, GitHub's URLs the new one.
	u := "https://github.com/acme/webhooks/actions/runs/7/job/8"
	if id, ok := ActionsJobID(u, "acme/hooks", "acme/webhooks"); !ok || id != 8 {
		t.Errorf("ActionsJobID = %d, %v; want 8, true", id, ok)
	}
	if id, ok := ActionsRunID(u, "acme/hooks", "acme/webhooks"); !ok || id != 7 {
		t.Errorf("ActionsRunID = %d, %v; want 7, true", id, ok)
	}
	if owner, name, ok := ActionsRepo(u, "acme/hooks", "acme/webhooks"); !ok || owner != "acme" || name != "webhooks" {
		t.Errorf("ActionsRepo = %q, %q, %v; want acme, webhooks, true", owner, name, ok)
	}
	if _, _, ok := ActionsRepo(u, "acme/hooks"); ok {
		t.Error("ActionsRepo accepted a URL for a repo it wasn't given")
	}
}
