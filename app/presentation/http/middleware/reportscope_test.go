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

// scopingReporter marks the context so the test can assert the middleware both
// called WithRequestScope and propagated its result downstream.
type scopingReporter struct{}

func (scopingReporter) Capture(context.Context, error, ...slog.Attr) {}
func (scopingReporter) Flush(time.Duration) bool                     { return true }
func (scopingReporter) WithRequestScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, "scoped")
}

// ReportScopeMiddleware must establish the per-request reporting scope and
// propagate the returned context to the handler, so the breadcrumb trail is in
// place before any handler or recovery runs.
func TestReportScopeMiddleware_PropagatesScopedContext(t *testing.T) {
	t.Parallel()
	var seen any
	h := ReportScopeMiddleware(scopingReporter{})(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = r.Context().Value(scopeKey{})
		}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if seen != "scoped" {
		t.Fatalf("handler must see the scoped context from WithRequestScope, got %v", seen)
	}
}
