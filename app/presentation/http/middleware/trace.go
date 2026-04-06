package middleware

import (
	"gokick/app/domain/shared"
	"net/http"

	"github.com/google/uuid"
)

func TraceMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := r.Header.Get("X-Trace-Id")
			if traceID == "" {
				traceID = uuid.New().String()
			}

			ctx := shared.ContextWithTraceID(r.Context(), traceID)
			w.Header().Set("X-Trace-Id", traceID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
