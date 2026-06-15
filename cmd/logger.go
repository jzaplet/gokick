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
func newLogger(format string, level slog.Level, sentryEnabled bool) *slog.Logger {
	h := newLogHandler(os.Stderr, format, level)
	if !sentryEnabled {
		// No DSN → breadcrumbs go nowhere, so skip the wrapper entirely rather
		// than probe ctx for a hub that can never be present on every log line.
		return slog.New(h)
	}
	return slog.New(breadcrumbHandler{Handler: h})
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
//
// groups/attrs mirror what logger.With / WithGroup bind: the wrapped handler
// applies them to the text log, but the breadcrumb is built here from the
// record, so without mirroring them a derived logger's bound fields (e.g. the
// worker's job_id / kind / attempts) would be absent from the breadcrumb trail.
type breadcrumbHandler struct {
	slog.Handler
	groups []string    // open groups, for namespacing keys in the flat Data map
	attrs  []slog.Attr // WithAttrs-bound attrs, keys already group-prefixed
}

func (h breadcrumbHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelInfo {
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			hub.AddBreadcrumb(h.recordToBreadcrumb(r), nil)
		}
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs / WithGroup re-wrap so the breadcrumb behaviour survives logger.With,
// AND accumulate the bound attrs/groups so they reach the breadcrumb too.
func (h breadcrumbHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	prefixed := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		prefixed = append(prefixed, slog.Attr{Key: h.prefixKey(a.Key), Value: a.Value})
	}
	return breadcrumbHandler{
		Handler: h.Handler.WithAttrs(attrs),
		groups:  h.groups,
		attrs:   append(append([]slog.Attr{}, h.attrs...), prefixed...),
	}
}

func (h breadcrumbHandler) WithGroup(name string) slog.Handler {
	return breadcrumbHandler{
		Handler: h.Handler.WithGroup(name),
		groups:  append(append([]string{}, h.groups...), name),
		attrs:   h.attrs,
	}
}

// prefixKey namespaces a key by the currently-open groups (slog nests every
// later attr under a WithGroup). Flattened with dots, since a breadcrumb's Data
// is a flat map. No groups → key unchanged, the common case here.
func (h breadcrumbHandler) prefixKey(key string) string {
	if len(h.groups) == 0 {
		return key
	}
	return strings.Join(h.groups, ".") + "." + key
}

func (h breadcrumbHandler) recordToBreadcrumb(r slog.Record) *sentry.Breadcrumb {
	data := make(map[string]any, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		data[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		data[h.prefixKey(a.Key)] = a.Value.Any()
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
