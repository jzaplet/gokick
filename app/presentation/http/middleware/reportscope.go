package middleware

import (
	"net/http"

	"gokick/app/domain/shared"
)

// Distributed-trace headers the frontend's Sentry SDK sets on same-origin API
// calls. The reporter adopts the trace id from them so a backend error links to
// the originating frontend trace. http.Header.Get is case-insensitive; the
// lower-case form mirrors the wire spelling.
const (
	sentryTraceHeader = "sentry-trace"
	baggageHeader     = "baggage"
)

// ReportScopeMiddleware establishes a per-request error-reporting scope on the
// context (a fresh Sentry hub when Sentry is enabled, a no-op otherwise), so the
// log lines emitted while handling the request accumulate as breadcrumbs and
// ride along on any error captured downstream. It also continues the frontend's
// distributed trace (sentry-trace / baggage headers) onto that scope, so a
// backend error carries the same trace id as the frontend event — linking the
// two in Sentry. Wire it BEFORE RecoveryMiddleware so a recovered panic carries
// both the breadcrumb trail and the trace.
func ReportScopeMiddleware(reporter shared.ErrorReporter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reporter.WithRequestScope(r.Context())
			ctx = reporter.ContinueTrace(
				ctx,
				r.Header.Get(sentryTraceHeader),
				r.Header.Get(baggageHeader),
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
