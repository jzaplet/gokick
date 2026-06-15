package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gokick/app/domain/shared"

	"github.com/getsentry/sentry-go"
)

const (
	// panicExceptionType replaces the generic *errors.errorString that a wrapped
	// panic would otherwise surface as the Sentry exception type.
	panicExceptionType = "panic"
	// tagPanicType records the concrete Go type of the recovered value (string,
	// runtime.Error, …) so you can tell a nil-deref from a panic("msg").
	tagPanicType = "panic.type"
)

// newErrorReporter builds the process-wide error reporter. With an empty DSN it
// returns a no-op, so the app runs unchanged without a Sentry account. Built in
// cmd (like the logger) and injected as shared.ErrorReporter, which keeps the
// sentry-go import out of the layered app/ tree.
func newErrorReporter(dsn, environment, release string) (shared.ErrorReporter, error) {
	if dsn == "" {
		return shared.NopReporter{}, nil
	}
	if environment == "" {
		environment = "development"
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:           dsn,
		Environment:   environment,
		Release:       release,
		EnableTracing: false, // scope A: errors & panics only, no performance tracing
		// We attach the user's IP and email explicitly (the resolved client IP
		// from ctx, CF-Connecting-IP behind Cloudflare; the email from the auth
		// claims). sentry-go drops a user-supplied IPAddress / Email unless PII
		// sending is on, so this must be true for them to reach Sentry. It does
		// NOT auto-collect request data: no HTTP integration is installed, and
		// event.Request is built from a fixed set of attrs (sentryRequest) with
		// credential headers masked at the edge. BeforeSend masks once more.
		SendDefaultPII: true,
		// Last-line guard: mask any credential header that reaches a serialized
		// event — even one a future SDK integration might attach — so no raw
		// Authorization/Cookie can ever leave the process.
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			maskRequestHeaders(event)
			return event
		},
		// The slog→breadcrumb bridge forwards the whole structured-log stream
		// (no whitelist), so scrub secret-keyed values off every breadcrumb.
		BeforeBreadcrumb: func(breadcrumb *sentry.Breadcrumb, _ *sentry.BreadcrumbHint) *sentry.Breadcrumb {
			scrubBreadcrumb(breadcrumb)
			return breadcrumb
		},
	}); err != nil {
		return nil, err
	}
	return &sentryReporter{}, nil
}

type sentryReporter struct{}

// WithRequestScope binds a fresh per-request hub to ctx so breadcrumbs (the
// log lines emitted while handling the request/job, via the breadcrumb slog
// handler) accumulate on it and ride along on a later Capture from the same
// ctx. A clone keeps each request's trail isolated from concurrent requests.
func (sentryReporter) WithRequestScope(ctx context.Context) context.Context {
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub().Clone())
}

// ContinueTrace adopts the frontend's distributed-trace id (from the sentry-trace
// + baggage headers the browser SDK sets) onto the per-request hub's scope, so a
// downstream Capture emits the SAME trace id the frontend used and Sentry links
// the two events under one trace. We build the span via ContinueFromHeaders
// (which parses the trace id) and attach it to the scope ourselves: StartSpan
// only auto-sets the scope span when EnableTracing is on, and we keep that OFF
// (scope A — no span/transaction is ever sent, this is purely error linking).
// sentry-go's shouldContinueTrace adopts the id for a same-org frontend (our
// case); a cross-org or missing header just leaves a fresh local trace, so it
// degrades safely. Empty header → unchanged ctx (a non-browser caller).
func (sentryReporter) ContinueTrace(
	ctx context.Context,
	sentryTrace, baggage string,
) context.Context {
	if sentryTrace == "" {
		return ctx
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return ctx
	}
	span := sentry.StartSpan(ctx, "http.server", sentry.ContinueFromHeaders(sentryTrace, baggage))
	hub.Scope().SetSpan(span)

	return ctx
}

// Capture reports err to Sentry. Beyond the ctx correlation attrs (trace_id,
// user_id) and any caller-supplied attrs — attached as searchable tags, with
// secret-keyed values masked — it enriches the event with:
//   - User: id / nickname / role / email from the auth claims in ctx, plus the
//     resolved client IP, so Sentry can attribute and group by who hit the error.
//   - Request: method / URL / User-Agent plus the credential headers
//     (Authorization, Cookie) when present — masked, so an operator sees they
//     arrived without the secret leaking. Only for HTTP-originated captures; a
//     job-worker capture has no method and gets no Request.
//   - Fingerprint: for a job-worker capture, split by job kind so each failing
//     handler is its own Sentry issue (its unwound stack is otherwise identical).
//
// A cloned hub per call keeps scopes isolated across concurrent requests.
func (sentryReporter) Capture(ctx context.Context, err error, attrs ...slog.Attr) {
	if err == nil {
		return
	}
	all := append(shared.LogAttrs(ctx), attrs...)
	ip := shared.ActorIPFromContext(ctx)
	var panicErr *shared.PanicError
	isPanic := errors.As(err, &panicErr)

	// Prefer the per-request hub (it carries the breadcrumb trail set by
	// WithRequestScope); fall back to a fresh clone outside any request scope.
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	// WithScope finalizes on a temporary scope cloned from the hub's — so the
	// accumulated breadcrumbs are included — without leaking this request's
	// tags/processor onto the shared hub across captures.
	hub.WithScope(func(scope *sentry.Scope) {
		var jobKind string
		for _, a := range all {
			// Authorization/Cookie are represented (masked) in event.Request
			// below, not as a tag. Every other attr becomes a searchable tag —
			// masked when the key looks secret, so the open attr seam can't
			// egress a credential.
			if a.Key == shared.LogKeyAuthorization || a.Key == shared.LogKeyCookie {
				continue
			}
			if a.Key == shared.LogKeyJobKind {
				jobKind = a.Value.String()
			}
			scope.SetTag(a.Key, shared.MaskLogValue(a.Key, a.Value.String()))
		}
		if isPanic {
			scope.SetTag(tagPanicType, fmt.Sprintf("%T", panicErr.Value))
		}
		if user, ok := sentryUser(ctx, ip); ok {
			scope.SetUser(user)
		}
		req := sentryRequest(all)
		// One processor finalizes the event: the reconstructed request (no
		// direct scope setter for a pre-built *sentry.Request), the per-kind
		// fingerprint for worker captures, the in-app demotion of our own
		// reporting frames so the culprit is the real origin, and the "panic"
		// exception type.
		scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if req != nil {
				event.Request = req
			}
			if jobKind != "" {
				event.Fingerprint = []string{"{{ default }}", "job:" + jobKind}
			}
			setFrameInApp(event)
			if isPanic {
				setExceptionType(event, panicExceptionType)
			}
			return event
		})
		hub.CaptureException(err)
	})
}

// setFrameInApp assigns the in-app flag for every stack frame, so Sentry's
// culprit and title resolve to the real origin instead of this reporter. We set
// it ourselves because sentry-go's own heuristic is unusable in the production
// build: under -trimpath with a stripped binary runtime.GOROOT() is empty, so
// its goRoot prefix check (strings.HasPrefix(path, goRoot)) matches every frame
// and marks them ALL not-in-app — leaving no in-app frame, so the culprit falls
// back to the topmost one (sentryReporter.Capture). A frame is in-app when it
// belongs to the gokick module, EXCEPT our own reporting frames (this Capture
// and the recovery middlewares), which must never be the culprit. Module names
// survive the -s -w strip (pclntab), so this works in the production image.
func setFrameInApp(event *sentry.Event) {
	for ei := range event.Exception {
		st := event.Exception[ei].Stacktrace
		if st == nil {
			continue
		}
		for fi := range st.Frames {
			f := &st.Frames[fi]
			inApp := strings.HasPrefix(f.Module, "gokick")
			if strings.Contains(f.Function, "sentryReporter.Capture") ||
				strings.Contains(f.Function, "RecoveryMiddleware") {
				inApp = false
			}
			f.InApp = inApp
		}
	}
}

// setExceptionType relabels the event's exception type (e.g. "panic"), replacing
// the *errors.errorString a wrapped error would otherwise surface as the type.
func setExceptionType(event *sentry.Event, typ string) {
	for ei := range event.Exception {
		event.Exception[ei].Type = typ
	}
}

// sentryUser builds the Sentry user from the auth claims in ctx (when the
// request is authenticated) plus the resolved client IP. Returns ok=false only
// for an anonymous capture with no IP at all (e.g. a job worker), so the event
// carries no user rather than an empty one.
func sentryUser(ctx context.Context, ip string) (sentry.User, bool) {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		if ip == "" {
			return sentry.User{}, false
		}
		return sentry.User{IPAddress: ip}, true
	}
	user := sentry.User{
		ID:        claims.UserID,
		Username:  claims.Nickname,
		Email:     claims.Email,
		IPAddress: ip,
	}
	if claims.Role != "" {
		user.Data = map[string]string{"role": claims.Role}
	}
	return user, true
}

// sentryRequest reconstructs the Sentry request from the fixed set of attrs the
// caller passed (method / url / user_agent / authorization / cookie) — never the
// live *http.Request header map. The credential headers are masked here a second
// time (MaskHeaderValue) regardless of what the caller passed, so even a caller
// that forwarded a raw value cannot leak it. Returns nil when no method is
// present (a non-HTTP caller), so event.Request stays unset rather than
// half-populated.
func sentryRequest(attrs []slog.Attr) *sentry.Request {
	var method, url string
	headers := map[string]string{}
	add := func(name, value string) {
		if value != "" {
			headers[name] = shared.MaskHeaderValue(name, value)
		}
	}
	for _, a := range attrs {
		switch a.Key {
		case shared.LogKeyMethod:
			method = a.Value.String()
		case shared.LogKeyURL:
			url = a.Value.String()
		case shared.LogKeyUserAgent:
			add("User-Agent", a.Value.String())
		case shared.LogKeyAuthorization:
			add("Authorization", a.Value.String())
		case shared.LogKeyCookie:
			add("Cookie", a.Value.String())
		}
	}
	if method == "" {
		return nil
	}
	req := &sentry.Request{Method: method, URL: url}
	if len(headers) > 0 {
		req.Headers = headers
	}
	return req
}

// maskRequestHeaders is the last-line guard on a serialized event: it masks
// every sensitive header value on event.Request and drops the raw Cookies
// string. event.Request is built from our own attrs today (already masked), but
// this also covers anything a future SDK integration might attach — there is no
// path by which a raw Authorization/Cookie leaves the process.
func maskRequestHeaders(event *sentry.Event) {
	if event.Request == nil {
		return
	}
	event.Request.Headers = shared.MaskSensitiveHeaders(event.Request.Headers)
	if event.Request.Cookies != "" {
		event.Request.Cookies = shared.MaskedValue
	}
}

// scrubBreadcrumb masks secret-looking values on a breadcrumb. The slog→
// breadcrumb bridge forwards the whole structured-log stream, so unlike the
// request reconstruction it has no whitelist — a stray secret-keyed log attr
// would otherwise ride along verbatim.
func scrubBreadcrumb(b *sentry.Breadcrumb) {
	for k, v := range b.Data {
		if s, ok := v.(string); ok {
			b.Data[k] = shared.MaskLogValue(k, s)
		}
	}
}

func (sentryReporter) Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}
