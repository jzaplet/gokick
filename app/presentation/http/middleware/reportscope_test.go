package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type scopeKey struct{}

// scopingReporter marks the context so the test can assert the middleware called
// WithRequestScope and propagated its result downstream, and records the
// distributed-trace headers passed to ContinueTrace.
type scopingReporter struct {
	gotTrace   string
	gotBaggage string
}

func (*scopingReporter) Capture(context.Context, error, ...slog.Attr) {}
func (*scopingReporter) Flush(time.Duration) bool                     { return true }
func (*scopingReporter) WithRequestScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, "scoped")
}

func (r *scopingReporter) ContinueTrace(
	ctx context.Context,
	sentryTrace, baggage string,
) context.Context {
	r.gotTrace = sentryTrace
	r.gotBaggage = baggage

	return ctx
}

// ReportScopeMiddleware must establish the per-request reporting scope and
// propagate the returned context to the handler, so the breadcrumb trail is in
// place before any handler or recovery runs.
func TestReportScopeMiddleware_PropagatesScopedContext(t *testing.T) {
	t.Parallel()
	var seen any
	h := ReportScopeMiddleware(&scopingReporter{})(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = r.Context().Value(scopeKey{})
		}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if seen != "scoped" {
		t.Fatalf("handler must see the scoped context from WithRequestScope, got %v", seen)
	}
}

// The middleware must forward the frontend's distributed-trace headers
// (sentry-trace / baggage) to the reporter, so a backend error can be linked to
// the originating frontend trace.
func TestReportScopeMiddleware_ForwardsTraceHeaders(t *testing.T) {
	t.Parallel()
	rep := &scopingReporter{}
	h := ReportScopeMiddleware(rep)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("sentry-trace", "d49d9bf66f13450b81f65bc51cf49c03-a1b2c3d4e5f60718-1")
	req.Header.Set("baggage", "sentry-trace_id=d49d9bf66f13450b81f65bc51cf49c03")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if rep.gotTrace != "d49d9bf66f13450b81f65bc51cf49c03-a1b2c3d4e5f60718-1" {
		t.Fatalf("sentry-trace header not forwarded to reporter, got %q", rep.gotTrace)
	}
	if rep.gotBaggage != "sentry-trace_id=d49d9bf66f13450b81f65bc51cf49c03" {
		t.Fatalf("baggage header not forwarded to reporter, got %q", rep.gotBaggage)
	}
}
