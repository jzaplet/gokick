package middleware

import (
	"gokick/app/domain/shared"
	"log/slog"
	"net/http"
	"time"
)

// HTTP-middleware-local structured-log keys (cross-cutting ones — method, path,
// url, user_agent — live in shared.LogKey*). sloglint's no-raw-keys forbids bare
// string keys.
const (
	logKeyIP     = "ip"
	logKeyStatus = "status"
	logKeyBytes  = "bytes"
)

// statusRecorder wraps http.ResponseWriter to capture the response status code
// and byte count for the access log. Unwrap lets http.ResponseController reach
// the underlying writer (Flush/Hijack) so the wrapper stays transparent.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Default 200: net/http sends 200 when a handler writes a body
			// without an explicit WriteHeader, so mirror that for the log.
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http: request",
				append(shared.LogAttrs(r.Context()),
					slog.String(shared.LogKeyMethod, r.Method),
					slog.String(shared.LogKeyPath, r.URL.Path),
					slog.String(logKeyIP, shared.ActorIPFromContext(r.Context())),
					slog.Int(logKeyStatus, rec.status),
					slog.Int(logKeyBytes, rec.bytes),
					shared.DurationMsAttr(time.Since(start)),
				)...)
		})
	}
}
