// Package log provides a structured, file-based logger for git-term built on
// top of the stdlib log/slog package. Callers should pass *Logger as a
// dependency rather than relying on a package-global logger.
//
// Security note: never log token or secret values. This is a code contract;
// it is not enforced at runtime.
package log

import (
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
)

// Field name constants for structured log entries. Using these constants
// prevents callers from introducing typos in field names.
const (
	FieldRepo       = "repo"
	FieldJobKey     = "job_key"
	FieldCacheKey   = "cache_key"
	FieldDurationMS = "duration_ms"
	FieldStatusCode = "status_code"
	FieldHost       = "host"
	FieldPRNumber   = "pr_number"
	FieldFromCache  = "from_cache"
)

// Logger is the injectable logger for git-term services.
// Do not use a package-global logger. Pass Logger as a dependency.
type Logger struct {
	inner *slog.Logger
}

// New creates a Logger that writes to the given file path at the given level.
// If path is empty, writes to stderr.
// If the file cannot be opened, falls back to stderr without panicking.
// level must be one of: "debug", "info", "warn", "error". Defaults to "info".
func New(path string, level string) *Logger {
	var w io.Writer
	var handler slog.Handler

	lvl := parseLevel(level)

	if path == "" {
		w = os.Stderr
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	} else {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			// Fall back to stderr with a text handler so startup failures are
			// still visible to the operator.
			handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
		} else {
			handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: lvl})
		}
	}

	return &Logger{inner: slog.New(handler)}
}

// NewNop returns a logger that discards all output. Useful in tests.
func NewNop() *Logger {
	handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1})
	return &Logger{inner: slog.New(handler)}
}

// Debug logs at the DEBUG level with the given message and key-value pairs.
func (l *Logger) Debug(msg string, args ...any) {
	l.inner.Debug(msg, args...)
}

// Info logs at the INFO level with the given message and key-value pairs.
func (l *Logger) Info(msg string, args ...any) {
	l.inner.Info(msg, args...)
}

// Warn logs at the WARN level with the given message and key-value pairs.
func (l *Logger) Warn(msg string, args ...any) {
	l.inner.Warn(msg, args...)
}

// Error logs at the ERROR level with the given message and key-value pairs.
func (l *Logger) Error(msg string, args ...any) {
	l.inner.Error(msg, args...)
}

// With returns a new Logger with the given key-value pairs pre-attached to
// every subsequent log entry emitted by the returned logger.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{inner: l.inner.With(args...)}
}

// IsDebug returns true if the process was started with --debug or
// GIT_TERM_DEBUG=1. This is a pure read of os.Args and os.Getenv — no side
// effects.
func IsDebug() bool {
	if os.Getenv("GIT_TERM_DEBUG") == "1" {
		return true
	}
	return slices.Contains(os.Args[1:], "--debug")
}

// parseLevel maps a level string to the corresponding slog.Level value.
// Unrecognised values default to slog.LevelInfo.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
