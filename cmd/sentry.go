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
		// We attach the user's IP explicitly (the resolved client IP from
		// ctx, CF-Connecting-IP behind Cloudflare). sentry-go drops a
		// user-supplied IPAddress unless PII sending is on, so this must be
		// true for the IP to reach Sentry. It does NOT auto-collect anything:
		// no HTTP integration is installed, and event.Request is built from a
		// fixed whitelist below — there is no path that scrapes raw headers.
		SendDefaultPII: true,
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

// Capture reports err to Sentry. Beyond the ctx correlation attrs (trace_id,
// user_id) and any caller-supplied attrs — all attached as searchable tags — it
// enriches the event with:
//   - User: id / nickname / role from the auth claims in ctx, plus the resolved
//     client IP, so Sentry can attribute and group by who hit the error.
//   - Request: method / URL / User-Agent, reconstructed from the whitelisted
//     attrs the caller passed (never the raw header set — that carries
//     Authorization and Cookie). Only present for HTTP-originated captures; a
//     job-worker capture has no method and gets no Request.
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
		for _, a := range all {
			scope.SetTag(a.Key, a.Value.String())
		}
		if isPanic {
			scope.SetTag(tagPanicType, fmt.Sprintf("%T", panicErr.Value))
		}
		if user, ok := sentryUser(ctx, ip); ok {
			scope.SetUser(user)
		}
		req := sentryRequest(all)
		// One processor finalizes the event: the reconstructed request (no
		// direct scope setter for a pre-built *sentry.Request), the in-app
		// demotion of our own reporting frames so the culprit is the real
		// origin, and the "panic" exception type.
		scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if req != nil {
				event.Request = req
			}
			demoteReportingFrames(event)
			if isPanic {
				setExceptionType(event, panicExceptionType)
			}
			return event
		})
		hub.CaptureException(err)
	})
}

// demoteReportingFrames marks gokick's own error-reporting frames (this Capture
// method and the recovery middlewares) as not-in-app, so Sentry's culprit and
// title resolve to the first real frame — the actual panic origin — instead of
// sentryReporter.Capture. The sentry-generated stack already reaches the origin;
// this only fixes which frame counts as "the location". Function names survive
// the -s -w strip (pclntab), so it works in the production image.
func demoteReportingFrames(event *sentry.Event) {
	for ei := range event.Exception {
		st := event.Exception[ei].Stacktrace
		if st == nil {
			continue
		}
		for fi := range st.Frames {
			fn := st.Frames[fi].Function
			if strings.Contains(fn, "sentryReporter.Capture") ||
				strings.Contains(fn, "RecoveryMiddleware") {
				st.Frames[fi].InApp = false
			}
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
		IPAddress: ip,
	}
	if claims.Role != "" {
		user.Data = map[string]string{"role": claims.Role}
	}
	return user, true
}

// sentryRequest reconstructs the Sentry request from the whitelisted attrs the
// caller passed (method / url / user_agent). It is built from this fixed set on
// purpose — never the live *http.Request header map, which carries Authorization
// and Cookie. Returns nil when no method is present (a non-HTTP caller), so
// event.Request stays unset rather than half-populated.
func sentryRequest(attrs []slog.Attr) *sentry.Request {
	var method, url, userAgent string
	for _, a := range attrs {
		switch a.Key {
		case shared.LogKeyMethod:
			method = a.Value.String()
		case shared.LogKeyURL:
			url = a.Value.String()
		case shared.LogKeyUserAgent:
			userAgent = a.Value.String()
		}
	}
	if method == "" {
		return nil
	}
	req := &sentry.Request{Method: method, URL: url}
	if userAgent != "" {
		req.Headers = map[string]string{"User-Agent": userAgent}
	}
	return req
}

func (sentryReporter) Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}
