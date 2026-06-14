package main

import (
	"context"
	"log/slog"
	"time"

	"gokick/app/domain/shared"

	"github.com/getsentry/sentry-go"
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

	hub := sentry.CurrentHub().Clone()
	hub.ConfigureScope(func(scope *sentry.Scope) {
		for _, a := range all {
			scope.SetTag(a.Key, a.Value.String())
		}
		if user, ok := sentryUser(ctx, ip); ok {
			scope.SetUser(user)
		}
		if req := sentryRequest(all); req != nil {
			// event.Request has no direct scope setter for a pre-built
			// *sentry.Request (SetRequest takes an *http.Request, which we
			// deliberately don't carry into the domain interface), so set it
			// via an event processor on this cloned scope.
			scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
				event.Request = req
				return event
			})
		}
	})
	hub.CaptureException(err)
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
