package job

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func newRegistry(t *testing.T, kinds ...string) *HandlerRegistry {
	t.Helper()
	handlers := map[string]HandlerFunc{}
	for _, k := range kinds {
		handlers[k] = func(_ context.Context, _ []byte) error { return nil }
	}
	r, err := NewHandlerRegistry(handlers)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

func TestDispatcher_EnqueueRegisteredKind(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "dispatch_ok.db"))
	d := NewDispatcher(fx.Jobs, newRegistry(t, "welcome:send"))

	payload := map[string]any{"user_id": "u1", "email": "u1@example.com"}
	if err := d.Enqueue(context.Background(), "welcome:send", 0, payload); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := fx.Jobs.ClaimDue(context.Background(), 0)
	if err != nil || got == nil {
		t.Fatalf("expected one due job, got %v err=%v", got, err)
	}
	if got.Kind != "welcome:send" {
		t.Fatalf("kind: got %q want welcome:send", got.Kind)
	}
	if !strings.Contains(string(got.Payload), `"user_id":"u1"`) {
		t.Fatalf("payload not JSON-marshalled correctly: %s", got.Payload)
	}
}

func TestDispatcher_EnqueueUnknownKindFails(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "dispatch_unknown.db"))
	d := NewDispatcher(fx.Jobs, newRegistry(t, "known:kind"))

	err := d.Enqueue(context.Background(), "unknown:kind", 0, nil)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("error: %v", err)
	}
}

// maxRetries must be >= 0 — callers cannot rely on a default, and negative
// values would mean "skip even the first attempt" which is nonsensical.
func TestDispatcher_EnqueueRejectsNegativeMaxRetries(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "dispatch_negative_retries.db"))
	d := NewDispatcher(fx.Jobs, newRegistry(t, "any:kind"))

	for _, n := range []int{-1, -100} {
		err := d.Enqueue(context.Background(), "any:kind", n, nil)
		if err == nil {
			t.Fatalf("expected error for maxRetries=%d", n)
		}
		if !strings.Contains(err.Error(), "maxRetries >= 0") {
			t.Fatalf("maxRetries=%d error: %v", n, err)
		}
	}

	// 0 is valid — one attempt, no retry.
	if err := d.Enqueue(context.Background(), "any:kind", 0, nil); err != nil {
		t.Fatalf("maxRetries=0 should be accepted, got %v", err)
	}
}

// WithDelay sets RunAt in the future — claim must return nil immediately and
// only pick the job up after the delay elapses.
func TestDispatcher_WithDelay_NotClaimableBeforeRunAt(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "dispatch_delay.db"))
	d := NewDispatcher(fx.Jobs, newRegistry(t, "delayed:kind"))

	if err := d.Enqueue(context.Background(), "delayed:kind", 0, nil, shared.WithDelay(800*time.Millisecond)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Immediately after enqueue → not yet due.
	got, err := fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil before delay elapsed, got %+v", got)
	}

	// Wait past the delay (with margin) → now claimable.
	time.Sleep(1200 * time.Millisecond)
	got, err = fx.Jobs.ClaimDue(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim after delay: %v", err)
	}
	if got == nil {
		t.Fatal("expected delayed job to become claimable")
	}
	if got.Kind != "delayed:kind" {
		t.Fatalf("kind: got %q want delayed:kind", got.Kind)
	}
}

func TestRegistry_RejectsEmptyKind(t *testing.T) {
	t.Parallel()
	_, err := NewHandlerRegistry(map[string]HandlerFunc{
		"": func(_ context.Context, _ []byte) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "empty kind") {
		t.Fatalf("expected empty-kind error, got %v", err)
	}
}
