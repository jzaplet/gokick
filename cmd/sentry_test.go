package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"gokick/app/domain/shared"

	"github.com/getsentry/sentry-go"
)

// Without a DSN the reporter must be the no-op so the app runs unchanged and
// safely without a Sentry account — the gating that makes the feature
// mergeable before any DSN exists.
func TestNewErrorReporter_NopWithoutDSN(t *testing.T) {
	t.Parallel()
	r, err := newErrorReporter("", "production", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.(shared.NopReporter); !ok {
		t.Fatalf("empty DSN must yield a NopReporter, got %T", r)
	}
}

// A malformed DSN must surface sentry.Init's error so a misconfiguration
// fails fast at startup rather than silently disabling tracking.
func TestNewErrorReporter_ErrorsOnMalformedDSN(t *testing.T) {
	t.Parallel()
	if _, err := newErrorReporter("not-a-valid-dsn", "production", "v1"); err == nil {
		t.Fatal("a malformed DSN must return an error from sentry.Init")
	}
}

// Integration check of the whole Capture path through sentry-go itself: a
// BeforeSend hook stashes the prepared event and returns nil, so nothing leaves
// the process (no network). This verifies what the helper unit tests cannot —
// that the two library-dependent behaviors actually land on the serialized
// event: the event processor on the *cloned* hub's scope populates
// event.Request, and SendDefaultPII keeps the explicitly-set User.IPAddress
// (sentry-go scrubs it otherwise). Without this, the first real test of the
// riskiest code would be the prod deploy.
//
// NOT parallel: sentry.Init binds the global hub, which Capture clones.
func TestCapture_EnrichesEventWithUserAndRequest(t *testing.T) {
	var captured *sentry.Event
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:            "https://public@example.com/1", // parseable; BeforeSend drops it
		SendDefaultPII: true,                           // mirrors production
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			captured = event
			return nil // drop — nothing is delivered
		},
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}

	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "u-7", Nickname: "alice", Role: "admin",
	})
	ctx = shared.ContextWithActorIP(ctx, "203.0.113.9")

	sentryReporter{}.Capture(ctx, errors.New("boom"),
		slog.String(shared.LogKeyMethod, "POST"),
		slog.String(shared.LogKeyURL, "/api/v1/x?q=1"),
		slog.String(shared.LogKeyUserAgent, "agent/1.0"),
	)
	sentry.Flush(2 * time.Second)

	if captured == nil {
		t.Fatal("BeforeSend never ran — Capture produced no event")
	}
	if captured.User.ID != "u-7" || captured.User.Username != "alice" {
		t.Fatalf("event user id/name: %+v", captured.User)
	}
	if captured.User.IPAddress != "203.0.113.9" {
		t.Fatalf("SendDefaultPII must keep the explicit IP, got %q", captured.User.IPAddress)
	}
	if captured.Request == nil {
		t.Fatal("event.Request must be populated by the cloned-scope event processor")
	}
	if captured.Request.Method != "POST" || captured.Request.URL != "/api/v1/x?q=1" {
		t.Fatalf("event request method/url: %+v", captured.Request)
	}
	if captured.Request.Headers["User-Agent"] != "agent/1.0" {
		t.Fatalf("event request User-Agent header: %+v", captured.Request.Headers)
	}
}

// A capture on a request-scoped ctx carries the breadcrumb trail — log lines
// emitted on that ctx (through the breadcrumb slog handler) ride along on the
// event, the way Symfony attaches the Monolog/Doctrine trail. Verified
// end-to-end with the real wrapped handler, and the SAME event also carries the
// panic shape (A + B land together).
//
// NOT parallel: sentry.Init binds the global hub.
func TestCapture_RequestScopeCarriesBreadcrumbs(t *testing.T) {
	var captured *sentry.Event
	if err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://public@example.com/1",
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			captured = event
			return nil
		},
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}

	r := sentryReporter{}
	ctx := r.WithRequestScope(context.Background())

	// A log on the request-scoped ctx becomes a breadcrumb (real wrapped handler).
	logger := slog.New(
		breadcrumbHandler{Handler: newLogHandler(io.Discard, "json", slog.LevelInfo)},
	)
	logger.LogAttrs(ctx, slog.LevelInfo, "bus: completed", slog.String("command", "DoThing"))

	// Then a panic is captured on the same ctx.
	r.Capture(ctx, &shared.PanicError{Value: "boom", Message: "bus: panic in DoThing: boom"})
	sentry.Flush(2 * time.Second)

	if captured == nil {
		t.Fatal("BeforeSend never ran")
	}
	var sawBreadcrumb bool
	for _, b := range captured.Breadcrumbs {
		if b.Message == "bus: completed" {
			sawBreadcrumb = true
		}
	}
	if !sawBreadcrumb {
		t.Error("the request-scoped log must appear as a breadcrumb on the captured event")
	}
	// Same event still carries the panic shape — A and B land on one event.
	if len(captured.Exception) == 0 ||
		captured.Exception[len(captured.Exception)-1].Type != "panic" {
		t.Errorf("event must also carry panic exception type, got %+v", captured.Exception)
	}
}

// A recovered panic must surface as Sentry exception type "panic" (not the
// generic *errors.errorString) and carry a panic.type tag with the value's Go
// type. Verified on the serialized event via BeforeSend. (The in-app marking
// that fixes the culprit is unit-tested in TestSetFrameInApp — a non-trimpath
// test binary can't reproduce the production in-app behaviour from its runtime
// stack, so it's pinned as a pure transform instead.)
//
// NOT parallel: sentry.Init binds the global hub.
func TestCapture_PanicExceptionType(t *testing.T) {
	var captured *sentry.Event
	if err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://public@example.com/1",
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			captured = event
			return nil
		},
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}

	sentryReporter{}.Capture(context.Background(), &shared.PanicError{
		Value:   "boom",
		Message: "http: panic in GET /x: boom",
	})
	sentry.Flush(2 * time.Second)

	if captured == nil {
		t.Fatal("BeforeSend never ran")
	}
	if len(captured.Exception) == 0 {
		t.Fatal("event has no exception")
	}
	if got := captured.Exception[len(captured.Exception)-1].Type; got != "panic" {
		t.Fatalf("exception type: got %q want %q", got, "panic")
	}
	if got := captured.Tags[tagPanicType]; got != "string" {
		t.Fatalf("panic.type tag: got %q want %q", got, "string")
	}
}

// setFrameInApp must mark gokick frames in-app and everything else (stdlib, the
// main/reporter package) not-in-app — EXCEPT our own reporting frames (Capture,
// the recovery middlewares), which stay not-in-app so the culprit resolves to
// the real origin. This is the production behaviour sentry-go's heuristic can't
// deliver under -trimpath (empty GOROOT), so it's pinned here as a pure
// transform on a synthetic event.
func TestSetFrameInApp(t *testing.T) {
	t.Parallel()
	event := sentry.NewEvent()
	event.Exception = []sentry.Exception{{
		Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{
			{Module: "net/http", Function: "HandlerFunc.ServeHTTP"},
			{
				Module:   "gokick/app/presentation/http/server",
				Function: "(*Server).registerRoutes.func1",
			},
			{
				Module:   "gokick/app/application/user/command",
				Function: "(*CreateUserHandler).Handle",
			},
			{
				Module:   "gokick/app/presentation/http/middleware",
				Function: "RecoveryMiddleware.func1.1",
			},
			{Module: "main", Function: "sentryReporter.Capture"},
			{Module: "main", Function: "sentryReporter.Capture.func1"},
		}},
	}}

	setFrameInApp(event)

	want := map[string]bool{
		"HandlerFunc.ServeHTTP":          false, // stdlib
		"(*Server).registerRoutes.func1": true,  // gokick app — the real origin
		"(*CreateUserHandler).Handle":    true,  // gokick app — a bus handler
		"RecoveryMiddleware.func1.1":     false, // our reporting frame
		"sentryReporter.Capture":         false, // our reporting frame
		"sentryReporter.Capture.func1":   false, // our reporting closure
	}
	for _, f := range event.Exception[0].Stacktrace.Frames {
		if got := f.InApp; got != want[f.Function] {
			t.Errorf("frame %q: in_app=%v want %v", f.Function, got, want[f.Function])
		}
	}
}

// An authenticated capture identifies the user — id, nickname, role and the
// resolved client IP — so Sentry can attribute and group by who hit the error.
func TestSentryUser_FromClaimsAndIP(t *testing.T) {
	t.Parallel()
	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "u-7", Nickname: "alice", Role: "admin",
	})
	user, ok := sentryUser(ctx, "203.0.113.9")
	if !ok {
		t.Fatal("authenticated capture must yield a user")
	}
	if user.ID != "u-7" || user.Username != "alice" || user.IPAddress != "203.0.113.9" {
		t.Fatalf("user: %+v", user)
	}
	if user.Data["role"] != "admin" {
		t.Fatalf("role should ride along in user data: %+v", user.Data)
	}
}

// An anonymous capture (no claims) still carries the IP, so even pre-auth panics
// are attributable to an origin.
func TestSentryUser_AnonymousWithIP(t *testing.T) {
	t.Parallel()
	user, ok := sentryUser(context.Background(), "203.0.113.9")
	if !ok {
		t.Fatal("an IP alone should still yield a user")
	}
	if user.ID != "" || user.IPAddress != "203.0.113.9" {
		t.Fatalf("user: %+v", user)
	}
}

// No claims and no IP (e.g. a job-worker capture) yields no user rather than an
// empty husk.
func TestSentryUser_NoneWhenAnonymousAndNoIP(t *testing.T) {
	t.Parallel()
	if _, ok := sentryUser(context.Background(), ""); ok {
		t.Fatal("no claims and no IP must yield no user")
	}
}

// The request is reconstructed from exactly the whitelisted attrs — method, URL,
// User-Agent — and the correlation attrs riding alongside are ignored.
func TestSentryRequest_FromWhitelistedAttrs(t *testing.T) {
	t.Parallel()
	req := sentryRequest([]slog.Attr{
		slog.String(shared.LogKeyTraceID, "t-1"),
		slog.String(shared.LogKeyMethod, "POST"),
		slog.String(shared.LogKeyURL, "/api/v1/x?q=1"),
		slog.String(shared.LogKeyUserAgent, "agent/1.0"),
	})
	if req == nil {
		t.Fatal("expected a request")
	}
	if req.Method != "POST" || req.URL != "/api/v1/x?q=1" {
		t.Fatalf("req: %+v", req)
	}
	if req.Headers["User-Agent"] != "agent/1.0" {
		t.Fatalf("headers: %+v", req.Headers)
	}
}

// A non-HTTP caller (job worker) passes no method → no Request, so event.Request
// stays unset rather than half-populated.
func TestSentryRequest_NilWithoutMethod(t *testing.T) {
	t.Parallel()
	if req := sentryRequest([]slog.Attr{slog.String(shared.LogKeyJobKind, "email")}); req != nil {
		t.Fatalf("no method must yield nil Request, got %+v", req)
	}
}

// Only method/url/user_agent are read; any other attr — even one that happens to
// carry a secret — is never copied into the Request. With no User-Agent the
// Headers map stays nil.
func TestSentryRequest_IgnoresNonWhitelistedAttrs(t *testing.T) {
	t.Parallel()
	req := sentryRequest([]slog.Attr{
		slog.String(shared.LogKeyMethod, "GET"),
		slog.String("authorization", "Bearer super-secret"),
	})
	if req == nil {
		t.Fatal("expected a request")
	}
	if req.Headers != nil {
		t.Fatalf("no User-Agent → Headers must stay nil, got %+v", req.Headers)
	}
}
