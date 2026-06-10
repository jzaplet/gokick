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
	}); err != nil {
		return nil, err
	}
	return &sentryReporter{}, nil
}

type sentryReporter struct{}

// Capture reports err to Sentry, tagged with the ctx correlation attributes
// (trace_id, user_id) plus any extra attrs the caller passes. A cloned hub per
// call keeps tag scopes isolated across concurrent requests.
func (sentryReporter) Capture(ctx context.Context, err error, attrs ...slog.Attr) {
	if err == nil {
		return
	}
	hub := sentry.CurrentHub().Clone()
	hub.ConfigureScope(func(scope *sentry.Scope) {
		for _, a := range append(shared.LogAttrs(ctx), attrs...) {
			scope.SetTag(a.Key, a.Value.String())
		}
	})
	hub.CaptureException(err)
}

func (sentryReporter) Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}
