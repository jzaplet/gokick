package middleware

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"

	"gokick/app/domain/shared"
)

// startedRecorder flags whether the response has begun (WriteHeader, Write, or
// ReadFrom called), so RecoveryMiddleware knows whether it can still emit a clean
// 500 or must not corrupt a body already in flight. Stays transparent: Unwrap
// exposes the underlying writer to http.ResponseController (Flush/Hijack), and
// ReadFrom forwards so the embedded-SPA static file fast path survives the wrap.
type startedRecorder struct {
	http.ResponseWriter
	started bool
}

func (r *startedRecorder) WriteHeader(code int) {
	r.started = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *startedRecorder) Write(b []byte) (int, error) {
	r.started = true
	return r.ResponseWriter.Write(b)
}

func (r *startedRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.started = true
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(r.ResponseWriter, src)
}

func (r *startedRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// RecoveryMiddleware catches panics that escape an HTTP handler — anything the
// bus RecoveryMiddleware did not already recover (a panic before bus dispatch,
// or inside another middleware). It logs the panic with a stack trace, reports
// it to the error tracker, and returns a generic 500 so a panic never leaks a
// stack to the client or silently drops the connection.
//
// Limitation: a clean 500 is only possible if the response has NOT started. If
// the handler already wrote status/body before panicking, re-writing would
// corrupt the in-flight response (a superfluous WriteHeader appends the 500 JSON
// to a partial body), so this only logs + reports and leaves the truncated
// response to the client — the honest best we can do once bytes are on the wire.
//
// Wire it just inside TraceMiddleware so trace_id is already in ctx while it
// still wraps every other middleware and the handler.
func RecoveryMiddleware(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &startedRecorder{ResponseWriter: w}
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// http.ErrAbortHandler is the sanctioned way to abort a
				// response — let net/http handle it instead of logging noise.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				ctx := r.Context()
				logger.LogAttrs(ctx, slog.LevelError, "http: panic recovered",
					append(shared.LogAttrs(ctx),
						slog.String(shared.LogKeyMethod, r.Method),
						slog.String(shared.LogKeyPath, r.URL.Path),
						slog.String(logKeyIP, shared.ActorIPFromContext(ctx)),
						slog.Any(shared.LogKeyPanic, rec),
						slog.String(shared.LogKeyStack, string(debug.Stack())),
					)...)

				err := &shared.PanicError{
					Value:   rec,
					Message: fmt.Sprintf("http: panic in %s %s: %v", r.Method, r.URL.Path, rec),
				}
				// Method, request PATH (query string dropped) and User-Agent
				// always; the credential headers (Authorization, Cookie) when
				// present but MASKED here at the edge — so an operator can see the
				// header arrived without the secret ever reaching the error tracker.
				// Never the raw header set. The path is sent without its query
				// because a query parameter can carry a credential (?token=…) and
				// dropping it outright is the only leak-proof option — masking by
				// key name is best-effort and misses odd encodings. The Sentry
				// adapter turns these into event.Request; the resolved client IP
				// rides on ctx (SetUser).
				attrs := []slog.Attr{
					slog.String(shared.LogKeyMethod, r.Method),
					slog.String(shared.LogKeyURL, r.URL.EscapedPath()),
					slog.String(shared.LogKeyUserAgent, r.UserAgent()),
				}
				if v := r.Header.Get("Authorization"); v != "" {
					attrs = append(
						attrs,
						slog.String(
							shared.LogKeyAuthorization,
							shared.MaskHeaderValue("Authorization", v),
						),
					)
				}
				if v := r.Header.Get("Cookie"); v != "" {
					attrs = append(attrs,
						slog.String(shared.LogKeyCookie, shared.MaskHeaderValue("Cookie", v)))
				}
				reporter.Capture(ctx, err, attrs...)

				// Only write a clean 500 if nothing has gone out yet — otherwise a
				// re-write corrupts the in-flight response (see the doc comment).
				if rw.started {
					return
				}
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusInternalServerError)
				_, _ = rw.Write([]byte(`{"general":"internal server error"}`))
			}()
			next.ServeHTTP(rw, r)
		})
	}
}
