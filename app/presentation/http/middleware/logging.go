package middleware

import (
	"gokick/app/domain/shared"
	"log/slog"
	"net/http"
	"time"
)

// HTTP-middleware-local structured-log keys (cross-cutting ones live in
// shared.LogKey*). sloglint's no-raw-keys forbids bare string keys.
const (
	logKeyMethod = "method"
	logKeyPath   = "path"
	logKeyIP     = "ip"
)

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			next.ServeHTTP(w, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http: request",
				append(shared.LogAttrs(r.Context()),
					slog.String(logKeyMethod, r.Method),
					slog.String(logKeyPath, r.URL.Path),
					slog.String(logKeyIP, shared.ActorIPFromContext(r.Context())),
					shared.DurationMsAttr(time.Since(start)),
				)...)
		})
	}
}
