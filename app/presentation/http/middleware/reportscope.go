package middleware

import (
	"net/http"

	"gokick/app/domain/shared"
)

// ReportScopeMiddleware establishes a per-request error-reporting scope on the
// context (a fresh Sentry hub when Sentry is enabled, a no-op otherwise), so the
// log lines emitted while handling the request accumulate as breadcrumbs and
// ride along on any error captured downstream. Wire it BEFORE RecoveryMiddleware
// so a recovered panic carries the trail that led up to it.
func ReportScopeMiddleware(reporter shared.ErrorReporter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reporter.WithRequestScope(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
