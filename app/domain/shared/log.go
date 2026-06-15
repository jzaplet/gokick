package shared

import (
	"context"
	"log/slog"
	"time"
)

// Standardized structured-log attribute keys. Call sites reference these
// constants instead of bare strings so the field vocabulary stays consistent
// across layers — that consistency is what makes logs queryable in aggregators
// like Loki/Grafana. Keep this list to genuinely cross-cutting keys; single-use
// component-local keys (addr, slot, nickname, …) stay as literals at their one
// call site.
const (
	LogKeyTraceID    = "trace_id"
	LogKeyUserID     = "user_id"
	LogKeyCommand    = "command"
	LogKeyDurationMs = "duration_ms"
	LogKeyRetryInMs  = "retry_in_ms"
	LogKeyError      = "error"
	LogKeyEvent      = "event"
	LogKeyJobKind    = "job_kind"
)

// HTTP request keys. Cross-cutting because they travel from the presentation
// layer (access log + panic log) into the error reporter: RecoveryMiddleware
// passes these to ErrorReporter.Capture, and the Sentry adapter reconstructs
// event.Request from exactly this set. The credential-bearing headers
// (Authorization, Cookie) ARE forwarded — so an operator can see the header
// arrived — but their value is masked at the edge via MaskHeaderValue, so the
// secret itself never reaches the error tracker (e.g. "Bearer ==MASKED=="). The
// adapter masks again defensively. Keeping the keys here lets producer and
// consumer agree on the vocabulary without one importing the other.
const (
	LogKeyMethod        = "method"
	LogKeyPath          = "path"
	LogKeyURL           = "url"
	LogKeyUserAgent     = "user_agent"
	LogKeyAuthorization = "authorization"
	LogKeyCookie        = "cookie"
)

// LogAttrs returns the request-scoped correlation attributes carried in ctx:
// trace_id (when present) and user_id (when the request is authenticated, i.e.
// claims are in ctx). It is the single source of correlation attributes, so an
// OpenTelemetry bridge would extend this one function with span_id without
// touching any call site.
//
// user_id appears wherever claims are in ctx: the bus middleware and anything
// downstream of AuthMiddleware. The global HTTP access log runs *before* auth,
// so its line carries trace_id only — the matching bus line carries user_id,
// and the two join on trace_id.
//
// Compose it with slog's attribute API — the LogAttrs *method* takes
// []slog.Attr natively, so no []any conversion is needed:
//
//	attrs := append(shared.LogAttrs(ctx), slog.String(shared.LogKeyCommand, name))
//	logger.LogAttrs(ctx, slog.LevelInfo, "bus: completed", attrs...)
//
// LogAttrs allocates a fresh slice on every call, so callers may append to the
// result freely without aliasing a previous log line's attributes.
func LogAttrs(ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, 2)
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		attrs = append(attrs, slog.String(LogKeyTraceID, traceID))
	}
	if claims := ClaimsFromContext(ctx); claims != nil && claims.UserID != "" {
		attrs = append(attrs, slog.String(LogKeyUserID, claims.UserID))
	}
	return attrs
}

// MillisAttr renders a duration as a fractional-millisecond number under the
// given key. Microsecond precision is preserved, so a sub-millisecond operation
// logs as e.g. 0.333 rather than rounding to 0. Use DurationMsAttr for the
// canonical "duration_ms" field.
func MillisAttr(key string, d time.Duration) slog.Attr {
	return slog.Float64(key, float64(d.Microseconds())/1000.0)
}

// DurationMsAttr is MillisAttr bound to the canonical duration_ms key, used for
// "how long did this take" measurements across bus, HTTP, worker and scheduler.
func DurationMsAttr(d time.Duration) slog.Attr {
	return MillisAttr(LogKeyDurationMs, d)
}
