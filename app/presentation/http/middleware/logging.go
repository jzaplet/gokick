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

			next.ServeHTTP(w, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http: request",
				append(shared.LogAttrs(r.Context()),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					shared.DurationMsAttr(time.Since(start)),
				)...)
		})
	}
}
