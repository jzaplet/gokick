package middleware

import (
	"net/http"

	"gokick/app/domain/shared"
)

// IPMiddleware resolves the client IP through the shared IPExtractor
// (RemoteAddr by default, X-Real-IP only when APP_TRUST_PROXY_HEADERS
// is on) and stashes it in context. The audit middleware then stamps it
// onto every AuditRecord — keeping IP extraction in one place keeps
// rate-limiter and audit IP semantics from drifting apart.
func IPMiddleware(extract IPExtractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := shared.ContextWithActorIP(r.Context(), extract(r))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
