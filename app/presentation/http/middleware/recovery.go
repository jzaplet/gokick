package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"gokick/app/domain/shared"
)

// HTTP-recovery-local log keys (method/path/url/user_agent live in
// shared.LogKey*, shared with the Sentry adapter).
const (
	logKeyPanic = "panic"
	logKeyStack = "stack"
)

// RecoveryMiddleware catches panics that escape an HTTP handler — anything the
// bus RecoveryMiddleware did not already recover (a panic before bus dispatch,
// or inside another middleware). It logs the panic with a stack trace, reports
// it to the error tracker, and returns a generic 500 so a panic never leaks a
// stack to the client or silently drops the connection.
//
// Wire it just inside TraceMiddleware so trace_id is already in ctx while it
// still wraps every other middleware and the handler.
func RecoveryMiddleware(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						slog.Any(logKeyPanic, rec),
						slog.String(logKeyStack, string(debug.Stack())),
					)...)

				// Whitelist only: method, full URL and User-Agent. NEVER the raw
				// header set — it carries Authorization and Cookie, which must not
				// reach the error tracker. The Sentry adapter turns these into
				// event.Request; the resolved client IP rides on ctx (SetUser).
				err := fmt.Errorf("http: panic in %s %s: %v", r.Method, r.URL.Path, rec)
				reporter.Capture(ctx, err,
					slog.String(shared.LogKeyMethod, r.Method),
					slog.String(shared.LogKeyURL, r.URL.String()),
					slog.String(shared.LogKeyUserAgent, r.UserAgent()),
				)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"general":"internal server error"}`))
			}()
			next.ServeHTTP(w, r)
		})
	}
}
