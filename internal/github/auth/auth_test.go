package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
}

// fakeGH writes a shell script to a temp dir and prepends it to PATH.
// The responses map keys are space-joined arg strings (e.g. "auth token --hostname github.com").
// Returns a call counter map (key = arg string) and a cleanup func.
func fakeGH(t *testing.T, responses map[string]fakeResponse) (callCount map[string]*int) {
	t.Helper()

	dir := t.TempDir()
	callCount = make(map[string]*int)

	// Create a counter file directory.
	counterDir := filepath.Join(dir, "counters")
	if err := os.MkdirAll(counterDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write response files and build the script.
	var cases strings.Builder
	for args, resp := range responses {
		// Sanitize key for use as filename.
		key := strings.ReplaceAll(args, " ", "_")
		key = strings.ReplaceAll(key, "-", "_")
		key = strings.ReplaceAll(key, "/", "_")

		stdoutFile := filepath.Join(dir, key+".stdout")
		stderrFile := filepath.Join(dir, key+".stderr")
		exitFile := filepath.Join(dir, key+".exit")
		counterFile := filepath.Join(counterDir, key+".count")

		if err := os.WriteFile(stdoutFile, []byte(resp.stdout), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stderrFile, []byte(resp.stderr), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exitFile, fmt.Appendf(nil, "%d", resp.exitCode), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(counterFile, []byte("0"), 0o644); err != nil {
			t.Fatal(err)
		}

		n := new(int)
		callCount[args] = n

		fmt.Fprintf(&cases, `  "%s")
    KEY="%s"
    ;;
`, args, key)
	}

	script := fmt.Sprintf(`#!/bin/sh
ARGS="$*"
KEY=""
case "$ARGS" in
%s
esac

if [ -z "$KEY" ]; then
  echo "fake gh: unrecognised args: $ARGS" >&2
  exit 1
fi

COUNTER_FILE="%s/${KEY}.count"
COUNT=$(cat "$COUNTER_FILE" 2>/dev/null || echo 0)
COUNT=$((COUNT + 1))
echo "$COUNT" > "$COUNTER_FILE"

cat "%s/${KEY}.stdout" 2>/dev/null
cat "%s/${KEY}.stderr" >&2 2>/dev/null
exit $(cat "%s/${KEY}.exit" 2>/dev/null || echo 0)
`, cases.String(), counterDir, dir, dir, dir)

	scriptPath := filepath.Join(dir, "gh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Prepend our dir to PATH.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	// Set up a goroutine to update callCount from files after the test.
	t.Cleanup(func() {
		for args := range responses {
			key := strings.ReplaceAll(args, " ", "_")
			key = strings.ReplaceAll(key, "-", "_")
			key = strings.ReplaceAll(key, "/", "_")
			counterFile := filepath.Join(counterDir, key+".count")
			data, err := os.ReadFile(counterFile)
			if err == nil {
				var n int
				fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &n)
				*callCount[args] = n
			}
		}
	})

	return callCount
}

// readCallCount reads the counter for a specific args key from the counter files.
func readCallCount(t *testing.T, callCount map[string]*int, args string) int {
	t.Helper()
	// Force cleanup to run by triggering the counter reads inline.
	// Since counters are in temp files, we need to read them directly.
	// The cleanup func will update them, but we need them now.
	// We'll use a helper that reads directly from the file via the pointer.
	if p, ok := callCount[args]; ok {
		return *p
	}
	return 0
}

// flushCallCounts forces reading of counter files from disk.
// We achieve this by re-reading from the counter dir stored in the fakeGH closure.
// Since we can't easily access the dir, we use a different approach: wrap fakeGH
// to expose a flush function.

// fakeGHWithFlush is an enhanced version that returns a flush func.
func fakeGHWithFlush(t *testing.T, responses map[string]fakeResponse) (callCount map[string]*int, flush func()) {
	t.Helper()

	dir := t.TempDir()
	counterDir := filepath.Join(dir, "counters")
	if err := os.MkdirAll(counterDir, 0o755); err != nil {
		t.Fatal(err)
	}

	callCount = make(map[string]*int)
	pointers := make(map[string]*int)

	var cases strings.Builder
	for args, resp := range responses {
		key := strings.ReplaceAll(args, " ", "_")
		key = strings.ReplaceAll(key, "-", "_")
		key = strings.ReplaceAll(key, "/", "_")

		stdoutFile := filepath.Join(dir, key+".stdout")
		stderrFile := filepath.Join(dir, key+".stderr")
		exitFile := filepath.Join(dir, key+".exit")
		counterFile := filepath.Join(counterDir, key+".count")

		if err := os.WriteFile(stdoutFile, []byte(resp.stdout), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stderrFile, []byte(resp.stderr), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exitFile, fmt.Appendf(nil, "%d", resp.exitCode), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(counterFile, []byte("0"), 0o644); err != nil {
			t.Fatal(err)
		}

		n := new(int)
		callCount[args] = n
		pointers[args] = n

		fmt.Fprintf(&cases, `  "%s")
    KEY="%s"
    ;;
`, args, key)
	}

	script := fmt.Sprintf(`#!/bin/sh
ARGS="$*"
KEY=""
case "$ARGS" in
%s
esac

if [ -z "$KEY" ]; then
  echo "fake gh: unrecognised args: $ARGS" >&2
  exit 1
fi

COUNTER_FILE="%s/${KEY}.count"
COUNT=$(cat "$COUNTER_FILE" 2>/dev/null || echo 0)
COUNT=$((COUNT + 1))
echo "$COUNT" > "$COUNTER_FILE"

cat "%s/${KEY}.stdout" 2>/dev/null
cat "%s/${KEY}.stderr" >&2 2>/dev/null
exit $(cat "%s/${KEY}.exit" 2>/dev/null || echo 0)
`, cases.String(), counterDir, dir, dir, dir)

	scriptPath := filepath.Join(dir, "gh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	flush = func() {
		for args := range responses {
			key := strings.ReplaceAll(args, " ", "_")
			key = strings.ReplaceAll(key, "-", "_")
			key = strings.ReplaceAll(key, "/", "_")
			counterFile := filepath.Join(counterDir, key+".count")
			data, err := os.ReadFile(counterFile)
			if err == nil {
				var n int
				fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &n)
				*pointers[args] = n
			}
		}
	}

	return callCount, flush
}

// Test 1: GH_TOKEN env used for github.com
func TestResolveToken_GHToken(t *testing.T) {
	// Set up fake gh that should NOT be called.
	fakeGH(t, map[string]fakeResponse{})

	t.Setenv("GH_TOKEN", "tok123")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")

	svc := NewAuthService()
	tok, err := svc.ResolveToken(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "tok123" {
		t.Fatalf("expected tok123, got %q", tok)
	}
}

// Test 2: GH_ENTERPRISE_TOKEN env used for GHE
func TestResolveToken_GHEnterpriseToken(t *testing.T) {
	fakeGH(t, map[string]fakeResponse{})

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "tok456")

	svc := NewAuthService()
	tok, err := svc.ResolveToken(context.Background(), "ghe.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "tok456" {
		t.Fatalf("expected tok456, got %q", tok)
	}
}

// Test 3: Falls back to `gh auth token`
func TestResolveToken_FallbackToGH(t *testing.T) {
	fakeGH(t, map[string]fakeResponse{
		"auth token --hostname github.com": {
			stdout:   "tok789\n",
			exitCode: 0,
		},
	})

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")

	svc := NewAuthService()
	tok, err := svc.ResolveToken(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "tok789" {
		t.Fatalf("expected tok789, got %q", tok)
	}
}

// Test 4: Error when gh fails
func TestResolveToken_GHFails(t *testing.T) {
	fakeGH(t, map[string]fakeResponse{
		"auth token --hostname github.com": {
			stderr:   "not logged in\n",
			exitCode: 1,
		},
	})

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")

	svc := NewAuthService()
	_, err := svc.ResolveToken(context.Background(), "github.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("expected error to contain 'gh auth login', got: %v", err)
	}
}

// Test 5: ResolveHosts parses JSON (single host)
func TestResolveHosts_SingleHost(t *testing.T) {
	statusJSON := `{"hosts":{"github.com":[{"login":"alice","active":true}]}}`
	fakeGH(t, map[string]fakeResponse{
		"auth status --json hosts": {
			stdout:   statusJSON,
			exitCode: 0,
		},
		"auth token --hostname github.com": {
			stdout:   "mytoken\n",
			exitCode: 0,
		},
	})

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")

	svc := NewAuthService()
	profiles, err := svc.ResolveHosts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	p := profiles[0]
	if p.Host != "github.com" {
		t.Errorf("expected Host=github.com, got %q", p.Host)
	}
	if p.ViewerLogin != "alice" {
		t.Errorf("expected ViewerLogin=alice, got %q", p.ViewerLogin)
	}
	if p.GraphQLURL != "https://api.github.com/graphql" {
		t.Errorf("unexpected GraphQLURL: %q", p.GraphQLURL)
	}
	if p.RESTURL != "https://api.github.com" {
		t.Errorf("unexpected RESTURL: %q", p.RESTURL)
	}
	if p.Token != "mytoken" {
		t.Errorf("expected Token=mytoken, got %q", p.Token)
	}
}

// Test 6: ResolveHosts multi-host
func TestResolveHosts_MultiHost(t *testing.T) {
	statusJSON := `{"hosts":{
		"github.com":[{"login":"alice","active":true}],
		"ghe.corp.com":[{"login":"bob","active":true}]
	}}`
	fakeGH(t, map[string]fakeResponse{
		"auth status --json hosts": {
			stdout:   statusJSON,
			exitCode: 0,
		},
		"auth token --hostname github.com": {
			stdout:   "token_alice\n",
			exitCode: 0,
		},
		"auth token --hostname ghe.corp.com": {
			stdout:   "token_bob\n",
			exitCode: 0,
		},
	})

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")

	svc := NewAuthService()
	profiles, err := svc.ResolveHosts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	byHost := make(map[string]struct{ login, token, graphql, rest string })
	for _, p := range profiles {
		byHost[p.Host] = struct{ login, token, graphql, rest string }{
			p.ViewerLogin, p.Token, p.GraphQLURL, p.RESTURL,
		}
	}

	ghEntry, ok := byHost["github.com"]
	if !ok {
		t.Fatal("missing github.com profile")
	}
	if ghEntry.login != "alice" {
		t.Errorf("expected alice, got %q", ghEntry.login)
	}
	if ghEntry.graphql != "https://api.github.com/graphql" {
		t.Errorf("unexpected graphql URL: %q", ghEntry.graphql)
	}

	gheEntry, ok := byHost["ghe.corp.com"]
	if !ok {
		t.Fatal("missing ghe.corp.com profile")
	}
	if gheEntry.login != "bob" {
		t.Errorf("expected bob, got %q", gheEntry.login)
	}
	if gheEntry.graphql != "https://ghe.corp.com/api/graphql" {
		t.Errorf("unexpected GHE graphql URL: %q", gheEntry.graphql)
	}
	if gheEntry.rest != "https://ghe.corp.com/api/v3" {
		t.Errorf("unexpected GHE REST URL: %q", gheEntry.rest)
	}
}

// Test 7: Token cached after first resolve
func TestResolveToken_CachedAfterFirstResolve(t *testing.T) {
	callCount, flush := fakeGHWithFlush(t, map[string]fakeResponse{
		"auth token --hostname github.com": {
			stdout:   "cached_token\n",
			exitCode: 0,
		},
	})

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")

	svc := NewAuthService()

	tok1, err := svc.ResolveToken(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	tok2, err := svc.ResolveToken(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if tok1 != "cached_token" || tok2 != "cached_token" {
		t.Fatalf("expected cached_token, got %q and %q", tok1, tok2)
	}

	flush()
	_ = readCallCount(t, callCount, "auth token --hostname github.com")
	count := *callCount["auth token --hostname github.com"]
	if count != 1 {
		t.Errorf("expected gh to be called exactly once, got %d", count)
	}
}
