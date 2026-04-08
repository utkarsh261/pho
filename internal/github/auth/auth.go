// Package auth resolves GitHub host profiles and tokens at startup.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	githubpkg "github.com/utk/git-term/internal/github"
)

// AuthService resolves GitHub host profiles and tokens at startup.
type AuthService interface {
	// ResolveHosts returns one profile per authenticated host found via `gh`.
	ResolveHosts(ctx context.Context) ([]githubpkg.GitHubHostProfile, error)

	// ResolveToken returns a token for a specific host, using the resolution order.
	// Caches result in memory for the session.
	ResolveToken(ctx context.Context, host string) (string, error)
}

// NewAuthService returns a new AuthService implementation.
func NewAuthService() AuthService {
	return &authService{
		tokenCache: make(map[string]string),
	}
}

type authService struct {
	mu         sync.Mutex
	tokenCache map[string]string
}

// ResolveToken resolves the token for a given host using the following order:
//  1. If host is "github.com" and GH_TOKEN env is set, use it.
//  2. If host is not "github.com" and GH_ENTERPRISE_TOKEN env is set, use it.
//  3. Run `gh auth token --hostname <host>` and use stdout (trimmed).
//  4. If all fail, return a descriptive error.
func (a *authService) ResolveToken(ctx context.Context, host string) (string, error) {
	// Check cache first.
	a.mu.Lock()
	if tok, ok := a.tokenCache[host]; ok {
		a.mu.Unlock()
		return tok, nil
	}
	a.mu.Unlock()

	// Resolution order.
	var token string

	if host == "github.com" {
		if tok := os.Getenv("GH_TOKEN"); tok != "" {
			token = tok
		}
	} else {
		if tok := os.Getenv("GH_ENTERPRISE_TOKEN"); tok != "" {
			token = tok
		}
	}

	if token == "" {
		// Shell out to gh auth token --hostname <host>.
		cmd := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", host)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("no GitHub token available for %s: run 'gh auth login --hostname %s'", host, host)
		}
		token = strings.TrimSpace(stdout.String())
	}

	if token == "" {
		return "", fmt.Errorf("no GitHub token available for %s: run 'gh auth login --hostname %s'", host, host)
	}

	// Cache the result.
	a.mu.Lock()
	a.tokenCache[host] = token
	a.mu.Unlock()

	return token, nil
}

// ghAuthStatusOutput is the JSON shape returned by `gh auth status --json hosts`.
type ghAuthStatusOutput struct {
	Hosts map[string]ghHostEntry `json:"hosts"`
}

type ghHostEntry struct {
	User        string `json:"user"`
	Token       string `json:"token"`
	GitProtocol string `json:"git_protocol"`
}

// ResolveHosts discovers all authenticated hosts via `gh auth status --json hosts`
// and returns a GitHubHostProfile for each one.
func (a *authService) ResolveHosts(ctx context.Context) ([]githubpkg.GitHubHostProfile, error) {
	cmd := exec.CommandContext(ctx, "gh", "auth", "status", "--json", "hosts")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh auth status failed: %s — run 'gh auth login' to authenticate", msg)
	}

	var status ghAuthStatusOutput
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return nil, fmt.Errorf("failed to parse gh auth status output: %w", err)
	}

	profiles := make([]githubpkg.GitHubHostProfile, 0, len(status.Hosts))
	for host, entry := range status.Hosts {
		token, err := a.ResolveToken(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolving token for host %s: %w", host, err)
		}
		profiles = append(profiles, githubpkg.GitHubHostProfile{
			Host:                   host,
			GraphQLURL:             githubpkg.DefaultGraphQLURL(host),
			RESTURL:                githubpkg.DefaultRESTURL(host),
			Token:                  token,
			ViewerLogin:            entry.User,
			SupportsTopLevelRollup: true,
			SupportsHeadRefOID:     true,
		})
	}

	return profiles, nil
}
