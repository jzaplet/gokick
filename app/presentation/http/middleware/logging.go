package middleware

import (
	"gokick/app/domain/shared"
	"log/slog"
	"net/http"
	"time"
)

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			traceID := shared.TraceIDFromContext(r.Context())

			next.ServeHTTP(w, r)

			logger.Info("http: request",
				"trace_id", traceID,
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start),
			)
		})
	}
}
