package graphql

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	githubpkg "github.com/utkarsh261/pho/internal/github"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no such host")
}

func TestQueryUnknownHostExplainsLogin(t *testing.T) {
	c := NewClient([]githubpkg.GitHubHostProfile{{Host: "github.com"}}, &http.Client{Transport: failingTransport{}}, nil)

	_, err := c.FetchViewer(context.Background(), "work.github.com")
	if err == nil || !strings.Contains(err.Error(), "gh is not logged in to work.github.com") {
		t.Fatalf("unknown host err = %v, want login hint", err)
	}

	_, err = c.FetchViewer(context.Background(), "github.com")
	if err == nil || strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("known host err = %v, want plain transport error", err)
	}
}
