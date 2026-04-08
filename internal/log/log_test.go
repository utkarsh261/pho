package log_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/utk/git-term/internal/log"
)

// logPath creates a temporary log file path inside t.TempDir().
func logPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// readLines returns all non-empty lines from a file.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readLines: %v", err)
	}
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// parseJSONLine unmarshals a single JSON log line into a map.
func parseJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("parseJSONLine: %v\nline: %s", err, line)
	}
	return m
}

// Test 1: Debug messages must NOT appear when the logger level is "info".
func TestDebugSuppressedAtInfoLevel(t *testing.T) {
	path := logPath(t, "suppress.log")
	l := log.New(path, "info")
	l.Debug("should not appear")

	lines := readLines(t, path)
	if len(lines) != 0 {
		t.Errorf("expected no log lines, got %d: %v", len(lines), lines)
	}
}

// Test 2: Debug messages MUST appear when the logger level is "debug".
func TestDebugWrittenAtDebugLevel(t *testing.T) {
	path := logPath(t, "debug.log")
	l := log.New(path, "debug")
	l.Debug("debug visible")

	lines := readLines(t, path)
	if len(lines) == 0 {
		t.Fatal("expected at least one log line, got none")
	}
	m := parseJSONLine(t, lines[0])
	if m["msg"] != "debug visible" {
		t.Errorf("unexpected msg field: %v", m["msg"])
	}
}

// Test 3: Info messages must appear with the correct message field.
func TestInfoWrittenAtInfoLevel(t *testing.T) {
	path := logPath(t, "info.log")
	l := log.New(path, "info")
	l.Info("hello world")

	lines := readLines(t, path)
	if len(lines) == 0 {
		t.Fatal("expected at least one log line, got none")
	}
	m := parseJSONLine(t, lines[0])
	if m["msg"] != "hello world" {
		t.Errorf("unexpected msg field: %v", m["msg"])
	}
}

// Test 4: Structured fields must round-trip through the JSON log file.
func TestStructuredFieldsRoundTrip(t *testing.T) {
	path := logPath(t, "fields.log")
	l := log.New(path, "info")
	l.Info("pr fetched", log.FieldRepo, "myorg/repo")

	lines := readLines(t, path)
	if len(lines) == 0 {
		t.Fatal("expected at least one log line, got none")
	}
	m := parseJSONLine(t, lines[0])
	if m[log.FieldRepo] != "myorg/repo" {
		t.Errorf("expected repo=myorg/repo, got %v", m[log.FieldRepo])
	}
}

// Test 5: With() must attach its fields to every subsequent log entry.
func TestWithInheritsFields(t *testing.T) {
	path := logPath(t, "with.log")
	base := log.New(path, "info")
	child := base.With(log.FieldHost, "github.com")

	child.Info("first message")
	child.Info("second message")

	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}
	for i, line := range lines {
		m := parseJSONLine(t, line)
		if m[log.FieldHost] != "github.com" {
			t.Errorf("line %d: expected host=github.com, got %v", i, m[log.FieldHost])
		}
	}
}

// Test 6: New with an unwritable path must not panic and must return a
// working logger (falling back to stderr).
func TestFallbackOnBadPath(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("New panicked: %v", r)
		}
	}()

	l := log.New("/nonexistent/deep/path/log.txt", "info")
	if l == nil {
		t.Fatal("New returned nil")
	}
	// The logger writes to stderr; just confirm it doesn't panic.
	l.Info("fallback message")
}

// Test 7: NewNop must not panic and must produce no output.
func TestNewNop(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewNop panicked: %v", r)
		}
	}()

	l := log.NewNop()
	if l == nil {
		t.Fatal("NewNop returned nil")
	}
	l.Debug("nop debug")
	l.Info("nop info")
	l.Warn("nop warn")
	l.Error("nop error")
}

// Test 8: IsDebug must return false by default, and true when
// GIT_TERM_DEBUG=1 is set.
func TestIsDebug(t *testing.T) {
	// Ensure the env var is unset for the default case.
	t.Setenv("GIT_TERM_DEBUG", "")
	if log.IsDebug() {
		t.Error("IsDebug should be false when GIT_TERM_DEBUG is not set")
	}

	t.Setenv("GIT_TERM_DEBUG", "1")
	if !log.IsDebug() {
		t.Error("IsDebug should be true when GIT_TERM_DEBUG=1")
	}
}
