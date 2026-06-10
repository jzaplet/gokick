package middleware

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// A panic inside a handler MUST be caught and converted to an error (never
// propagated up as a panic), and a stack trace MUST be logged — bus.md:
// "Zachytí panic, zaloguje stack trace". The panic path was otherwise
// untested: the only panic test (events_test.go) recovers inside the handler
// before RecoveryMiddleware ever sees it.
func TestRecoveryMiddleware_CatchesPanicAndLogsStack(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	mw := RecoveryMiddleware(logger)

	result, err := mw(
		context.Background(),
		"Boom",
		normalCmd{},
		func(context.Context) (any, error) {
			panic("kaboom")
		},
	)
	if err == nil {
		t.Fatal("panic must be converted to an error, not propagated")
	}
	if !strings.Contains(err.Error(), "panic in Boom") {
		t.Fatalf("error should name the command, got %v", err)
	}
	if result != nil {
		t.Fatalf("result must be nil on panic, got %v", result)
	}

	logged := buf.String()
	if !strings.Contains(logged, "panic recovered") {
		t.Fatalf("expected the panic to be logged, got %q", logged)
	}
	// debug.Stack() always emits a "goroutine N [running]:" header — its
	// presence proves a real stack trace (not just the panic value) was logged.
	if !strings.Contains(logged, "goroutine ") {
		t.Fatalf("expected a stack trace in the log, got %q", logged)
	}
}

// A handler that returns normally passes through untouched.
func TestRecoveryMiddleware_PassesThroughWhenNoPanic(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := RecoveryMiddleware(logger)

	got, err := mw(context.Background(), "Fine", normalCmd{}, func(context.Context) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("clean handler must not error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result passthrough: got %v want ok", got)
	}
}
