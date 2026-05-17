package middleware

import (
	"gokick/app/domain/shared"
	"net/http"
	"regexp"

	"github.com/google/uuid"
)

// Only accept inbound trace IDs that look like opaque correlation tokens
// (UUID, ULID, hex digest, etc). Anything else — including newlines and
// control bytes — would let an attacker inject log lines or spoof a
// trace ID owned by another tenant, so we replace it with a fresh UUID.
var traceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func TraceMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := r.Header.Get("X-Trace-Id")
			if !traceIDPattern.MatchString(traceID) {
				traceID = uuid.New().String()
			}

			ctx := shared.ContextWithTraceID(r.Context(), traceID)
			w.Header().Set("X-Trace-Id", traceID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
