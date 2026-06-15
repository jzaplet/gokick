package middleware

import (
	"gokick/app/domain/shared"
	"io"
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

// writerOnly hides a possible ReaderFrom on the wrapped writer so the io.Copy
// fallback below cannot recurse back into statusRecorder.ReadFrom.
type writerOnly struct{ io.Writer }

// ReadFrom keeps the static file server's pooled-buffer fast path: net/http's
// *response implements io.ReaderFrom, and wrapping it would otherwise mask that
// — forcing a fresh 32 KiB buffer per static response. Delegate to the
// underlying ReaderFrom (counting bytes) and fall back to io.Copy only if it has
// none. Status stays at the default 200 the same way a bare Write would.
func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(src)
		r.bytes += int(n)
		return n, err
	}
	n, err := io.Copy(writerOnly{r.ResponseWriter}, src)
	r.bytes += int(n)
	return n, err
}

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
