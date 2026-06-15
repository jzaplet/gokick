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
	// WithRequestScope returns a context carrying a fresh per-request reporting
	// scope. Breadcrumbs (the structured-log lines emitted while handling the
	// request or job) then accumulate on that scope, so a later Capture on the
	// same context includes the trail leading up to the failure. Call it once at
	// the start of each request/job. The no-op reporter returns ctx unchanged.
	WithRequestScope(ctx context.Context) context.Context
	// ContinueTrace adopts a distributed-trace id propagated from the frontend
	// (the sentry-trace + baggage request headers the browser SDK sets) onto the
	// per-request scope, so an error captured downstream carries the SAME trace id
	// the frontend used — linking the frontend and backend events under one trace
	// in the tracker. It does NOT enable performance tracing: no spans/transactions
	// are sent. Empty headers (a non-browser caller) leave ctx unchanged; the
	// no-op reporter ignores it. Call it right after WithRequestScope on the HTTP
	// path.
	ContinueTrace(ctx context.Context, sentryTrace, baggage string) context.Context
	// Flush blocks up to timeout for buffered events to be delivered. Call it
	// before process exit (incl. panic unwinding) so reports aren't lost.
	Flush(timeout time.Duration) bool
}

// NopReporter is the no-op reporter wired when no DSN is configured, so callers
// never nil-check and the feature stays safely inert without a Sentry account.
type NopReporter struct{}

func (NopReporter) Capture(context.Context, error, ...slog.Attr) {}

func (NopReporter) WithRequestScope(ctx context.Context) context.Context { return ctx }

func (NopReporter) ContinueTrace(ctx context.Context, _, _ string) context.Context { return ctx }

func (NopReporter) Flush(time.Duration) bool { return true }

// PanicError wraps a recovered panic value so the error reporter can label it
// distinctly. A plain fmt.Errorf would type every panic as *errors.errorString
// in the tracker; carrying the original value lets the Sentry adapter set a
// meaningful exception type ("panic") and tag the concrete Go type of the panic
// (string vs runtime.Error, …). The two recovery middlewares (bus, HTTP) wrap
// recovered panics in this; ordinary returned errors and terminal job failures
// do NOT — they are not panics. This is a plain domain type, no Sentry import.
type PanicError struct {
	Value   any    // the recovered panic value (preserves the concrete type)
	Message string // formatted context, e.g. "http: panic in GET /x: <value>"
}

func (e *PanicError) Error() string { return e.Message }
