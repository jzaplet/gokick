package main

import (
	"testing"

	"gokick/app/domain/shared"
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
