package shared

import (
	"context"
	"log/slog"
	"time"
)

// ErrorReporter ships unexpected failures — recovered panics and terminal job
// failures — to an external error tracker (Sentry). It is deliberately NOT a
// logging path: ordinary returned errors (validation, auth, 4xx) must never
// flow here, only the recovery/terminal paths, or the tracker drowns in noise.
//
// Implementations read trace_id/user_id from ctx for correlation; callers pass
// any extra tags as slog.Attr, reusing the shared.LogKey* vocabulary.
type ErrorReporter interface {
	// Capture reports err, tagged with the ctx correlation attributes plus the
	// given attrs. Best-effort and non-blocking — it must never fail a request.
	Capture(ctx context.Context, err error, attrs ...slog.Attr)
	// Flush blocks up to timeout for buffered events to be delivered. Call it
	// before process exit (incl. panic unwinding) so reports aren't lost.
	Flush(timeout time.Duration) bool
}

// NopReporter is the no-op reporter wired when no DSN is configured, so callers
// never nil-check and the feature stays safely inert without a Sentry account.
type NopReporter struct{}

func (NopReporter) Capture(context.Context, error, ...slog.Attr) {}

func (NopReporter) Flush(time.Duration) bool { return true }
