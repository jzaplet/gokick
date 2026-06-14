package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
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
	return slog.New(breadcrumbHandler{Handler: newLogHandler(os.Stderr, format, level)})
}

// breadcrumbHandler wraps a slog.Handler so every INFO+ record also becomes a
// Sentry breadcrumb on the per-request hub carried in ctx (set by
// ErrorReporter.WithRequestScope). With no such hub — Sentry disabled, or a log
// outside any request scope — it is a pure pass-through. This is the seam that
// turns the structured-log stream into the trail that rides along on a captured
// error, the way Symfony attaches Monolog/Doctrine breadcrumbs.
//
// It lives in cmd/ (the sole place allowed to import the Sentry sink) and wraps
// the handler from newLogHandler, so no app/ logging call site changes — only
// logs emitted with the ctx form (LogAttrs(ctx, …)) carry the hub and breadcrumb.
type breadcrumbHandler struct {
	slog.Handler
}

func (h breadcrumbHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelInfo {
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			hub.AddBreadcrumb(recordToBreadcrumb(r), nil)
		}
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs / WithGroup re-wrap so the breadcrumb behaviour survives logger.With.
func (h breadcrumbHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return breadcrumbHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h breadcrumbHandler) WithGroup(name string) slog.Handler {
	return breadcrumbHandler{Handler: h.Handler.WithGroup(name)}
}

func recordToBreadcrumb(r slog.Record) *sentry.Breadcrumb {
	data := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		data[a.Key] = a.Value.Any()
		return true
	})
	return &sentry.Breadcrumb{
		Category:  "log",
		Message:   r.Message,
		Level:     slogToSentryLevel(r.Level),
		Data:      data,
		Timestamp: r.Time,
	}
}

func slogToSentryLevel(l slog.Level) sentry.Level {
	switch {
	case l >= slog.LevelError:
		return sentry.LevelError
	case l >= slog.LevelWarn:
		return sentry.LevelWarning
	default:
		return sentry.LevelInfo
	}
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
