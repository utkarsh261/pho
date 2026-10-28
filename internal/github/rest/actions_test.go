package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/utkarsh261/pho/internal/domain"
)

func TestTailLinesCapsLongLines(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 2<<20) // longer than any buffer
	in := "first\r\n" + long + "\nlast"
	lines, truncated, err := tailLines(strings.NewReader(in), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated || len(lines) != 3 {
		t.Fatalf("got %d lines truncated=%v, want 3 lines", len(lines), truncated)
	}
	if lines[0] != "first" || lines[2] != "last" {
		t.Errorf("lines = %q / %q", lines[0], lines[2])
	}
	if len(lines[1]) != maxLogLineBytes {
		t.Errorf("long line kept %d bytes, want %d", len(lines[1]), maxLogLineBytes)
	}
}

func TestTailLinesCutKeepsValidUTF8(t *testing.T) {
	t.Parallel()
	// "é" is two bytes; a cut at an odd offset splits it.
	in := "a" + strings.Repeat("é", maxLogLineBytes) + "\n"
	lines, _, err := tailLines(strings.NewReader(in), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !utf8.ValidString(lines[0]) {
		t.Error("cut line is not valid UTF-8")
	}
}

func TestFetchJobLogTailOutlivesClientTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("start\n"))
		w.(http.Flusher).Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte("end\n"))
	}))
	defer server.Close()

	httpClient := server.Client()
	httpClient.Timeout = 50 * time.Millisecond
	client := &Client{BaseURL: server.URL, HTTPClient: httpClient, Token: "t"}
	lines, _, err := client.FetchJobLogTail(context.Background(), "o", "r", 1, 5)
	if err != nil {
		t.Fatalf("a slow body should not hit the client timeout: %v", err)
	}
	if strings.Join(lines, "|") != "start|end" {
		t.Errorf("lines = %q", lines)
	}
	if httpClient.Timeout != 50*time.Millisecond {
		t.Error("the shared client's timeout must not be changed")
	}
}

func TestFetchJobLogTailKeepsTokenOnSameHostRedirect(t *testing.T) {
	t.Parallel()
	// GitHub answers a request to a renamed repo's old path with a same-host
	// redirect; a private repo's log then still needs the token.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/old/proj/actions/jobs/9/logs" {
			http.Redirect(w, r, "/repositories/42/actions/jobs/9/logs", http.StatusMovedPermanently)
			return
		}
		if got := r.Header.Get("Authorization"); got != "token t" {
			w.WriteHeader(http.StatusNotFound) // what a private repo says without auth
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client(), Token: "t"}
	lines, _, err := client.FetchJobLogTail(context.Background(), "old", "proj", 9, 5)
	if err != nil {
		t.Fatalf("token dropped on a same-host redirect: %v", err)
	}
	if strings.Join(lines, "|") != "ok" {
		t.Errorf("lines = %q", lines)
	}
}

func TestFetchJobLogTailFollowsRedirectAndKeepsTail(t *testing.T) {
	t.Parallel()
	var log strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&log, "line %d\n", i)
	}
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization leaked to the log download host: %q", got)
		}
		_, _ = w.Write([]byte(log.String()))
	}))
	defer blob.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/jobs/77/logs" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			t.Errorf("expected Authorization=%q, got %q", "token test-token", got)
		}
		http.Redirect(w, r, blob.URL+"/signed", http.StatusFound)
	}))
	defer api.Close()

	client := &Client{BaseURL: api.URL, HTTPClient: api.Client(), Token: "test-token"}
	lines, truncated, err := client.FetchJobLogTail(context.Background(), "owner", "repo", 77, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !truncated {
		t.Error("expected truncated=true")
	}
	want := []string{"line 8", "line 9", "line 10"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want %q", lines, want)
	}
}

func TestFetchJobLogTailShortLogNotTruncated(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("a\nb\n"))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client(), Token: "t"}
	lines, truncated, err := client.FetchJobLogTail(context.Background(), "o", "r", 1, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated || strings.Join(lines, "|") != "a|b" {
		t.Errorf("got %q truncated=%v", lines, truncated)
	}
}

func TestFetchJobLogTailUnavailable(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		client := &Client{BaseURL: server.URL, HTTPClient: server.Client(), Token: "t"}
		_, _, err := client.FetchJobLogTail(context.Background(), "o", "r", 1, 5)
		server.Close()
		if !errors.Is(err, domain.ErrJobLogUnavailable) {
			t.Errorf("status %d: err = %v, want ErrJobLogUnavailable", status, err)
		}
	}
}

func TestRerunJob(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/repos/owner/repo/actions/jobs/77/rerun" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client(), Token: "t"}
	if err := client.RerunJob(context.Background(), "owner", "repo", 77); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRerunJobForbidden(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"This job is still running"}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTPClient: server.Client(), Token: "t"}
	err := client.RerunJob(context.Background(), "o", "r", 1)
	if err == nil || err.Error() != "This job is still running (status 403)" {
		t.Fatalf("err = %v, want GitHub's message", err)
	}
}
