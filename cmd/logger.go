package main

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// newLogger builds the process-wide structured logger written to stderr.
// Format and level are explicit parameters (not read from the environment
// here) so the constructor stays a pure, unit-testable unit; main resolves
// them from APP_LOG_FORMAT / APP_LOG_LEVEL.
//
// This is the single seam where the application constructs a logger. When
// OpenTelemetry is introduced, an otelslog bridge (or a fan-out handler that
// both prints locally and exports via OTLP) wraps the handler built here —
// no other code path creates a *slog.Logger.
func newLogger(format string, level slog.Level) *slog.Logger {
	return slog.New(newLogHandler(os.Stderr, format, level))
}

// newLogHandler isolates handler construction so it can be tested against an
// in-memory writer. "text" selects the human-readable handler (handy for local
// `make serve`); anything else — including the empty default — is JSON, which
// is what log aggregators ingest.
func newLogHandler(w io.Writer, format string, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(strings.TrimSpace(format), "text") {
		return slog.NewTextHandler(w, opts)
	}
	return slog.NewJSONHandler(w, opts)
}

// parseLogLevel maps a level name to slog.Level, defaulting to Info for empty
// or unrecognized input so a typo degrades to a sane verbosity rather than
// silencing logs.
func parseLogLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
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
