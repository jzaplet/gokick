package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gokick/app/domain/shared"
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
	mw := RecoveryMiddleware(logger, shared.NopReporter{})

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
	mw := RecoveryMiddleware(logger, shared.NopReporter{})

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

type captureReporter struct{ errs []error }

func (c *captureReporter) Capture(_ context.Context, err error, _ ...slog.Attr) {
	c.errs = append(c.errs, err)
}
func (*captureReporter) Flush(time.Duration) bool { return true }

func (*captureReporter) WithRequestScope(ctx context.Context) context.Context { return ctx }

// A recovered panic is reported to the error tracker exactly once, in addition
// to being logged. The no-report-on-returned-error half is pinned by
// TestRecoveryMiddleware_DoesNotReportReturnedError below.
func TestRecoveryMiddleware_ReportsPanicOnce(t *testing.T) {
	t.Parallel()
	rep := &captureReporter{}
	mw := RecoveryMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), rep)

	_, err := mw(context.Background(), "Boom", normalCmd{}, func(context.Context) (any, error) {
		panic("kaboom")
	})
	if err == nil {
		t.Fatal("panic must convert to an error")
	}
	if len(rep.errs) != 1 {
		t.Fatalf("reporter must capture the panic exactly once, got %d", len(rep.errs))
	}
}

// An ordinary returned error must NOT be reported — only panics reach the
// reporter. This guards the noise-control invariant: a future "also report
// failed commands" change (reporter.Capture after next()) would flood the
// tracker with every 4xx/validation/auth error, and this test would catch it.
func TestRecoveryMiddleware_DoesNotReportReturnedError(t *testing.T) {
	t.Parallel()
	rep := &captureReporter{}
	mw := RecoveryMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), rep)

	wantErr := errors.New("ordinary failure")
	_, err := mw(context.Background(), "Fails", normalCmd{}, func(context.Context) (any, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("returned error must propagate unchanged, got %v", err)
	}
	if len(rep.errs) != 0 {
		t.Fatalf("a returned error must NOT be reported, got %d captures", len(rep.errs))
	}
}
