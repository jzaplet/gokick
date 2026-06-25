package run

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// newRegistry builds a registry whose listed kinds map to no-op handlers, with a
// 1s default lease. Mirrors the job dispatcher test helper.
func newRegistry(t *testing.T, kinds ...string) *HandlerRegistry {
	t.Helper()
	regs := map[string]Registration{}
	for _, k := range kinds {
		regs[k] = Registration{
			Handler: func(context.Context, *run.Run, Checkpointer) error { return nil },
		}
	}
	r, err := NewHandlerRegistry(regs, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

func TestDispatcher_EnqueueRegisteredKind(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_dispatch_ok.db"))
	d := NewDispatcher(fx.Runs, newRegistry(t, "agent:summarize"))

	payload := map[string]any{"doc_id": "d1", "lang": "cs"}
	if err := d.Enqueue(context.Background(), "agent:summarize", 0, payload); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := fx.Runs.ClaimDue(context.Background(), "owner-1", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("expected one due run, got %v err=%v", got, err)
	}
	if got.Kind != "agent:summarize" {
		t.Fatalf("kind: got %q want agent:summarize", got.Kind)
	}
	if !strings.Contains(string(got.Payload), `"doc_id":"d1"`) {
		t.Fatalf("payload not JSON-marshalled correctly: %s", got.Payload)
	}
}

func TestDispatcher_EnqueueUnknownKindFails(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_dispatch_unknown.db"))
	d := NewDispatcher(fx.Runs, newRegistry(t, "known:kind"))

	err := d.Enqueue(context.Background(), "unknown:kind", 0, nil)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("error: %v", err)
	}

	// An unknown kind must NOT have persisted a row (validation precedes Enqueue).
	got, cerr := fx.Runs.ClaimDue(context.Background(), "owner-1", time.Minute)
	if cerr != nil {
		t.Fatalf("claim: %v", cerr)
	}
	if got != nil {
		t.Fatalf("unknown kind must not enqueue a run, got %+v", got)
	}
}

// maxRetries must be >= 0 — callers cannot rely on a default, and negative values
// would mean "skip even the first attempt", which is nonsensical.
func TestDispatcher_EnqueueRejectsNegativeMaxRetries(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_dispatch_negative.db"))
	d := NewDispatcher(fx.Runs, newRegistry(t, "any:kind"))

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

// WithDelay sets RunAt in the future — claim must return nil immediately and only
// pick the run up after the delay elapses.
func TestDispatcher_WithDelay_NotClaimableBeforeRunAt(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_dispatch_delay.db"))
	d := NewDispatcher(fx.Runs, newRegistry(t, "delayed:kind"))

	if err := d.Enqueue(context.Background(), "delayed:kind", 0, nil, shared.WithDelay(800*time.Millisecond)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Immediately after enqueue → not yet due.
	got, err := fx.Runs.ClaimDue(context.Background(), "owner-1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil before delay elapsed, got %+v", got)
	}

	// Wait past the delay (with margin) → now claimable.
	time.Sleep(1200 * time.Millisecond)
	got, err = fx.Runs.ClaimDue(context.Background(), "owner-1", time.Minute)
	if err != nil {
		t.Fatalf("claim after delay: %v", err)
	}
	if got == nil {
		t.Fatal("expected delayed run to become claimable")
	}
	if got.Kind != "delayed:kind" {
		t.Fatalf("kind: got %q want delayed:kind", got.Kind)
	}
}

// The run dispatcher stamps the tenant from the enqueuing context so the worker
// (which bypasses the bus) can restore it for the handler. A context carrying an
// explicit tenant must land on the row; a context with none falls back to the
// default tenant (never an empty string, which would override the column DEFAULT).
func TestDispatcher_StampsTenantFromContext(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_dispatch_tenant.db"))
	d := NewDispatcher(fx.Runs, newRegistry(t, "scoped:kind", "unscoped:kind"))

	// Explicit tenant in ctx → stamped onto the run.
	tenantCtx := shared.ContextWithTenantID(context.Background(), "tenant-x")
	if err := d.Enqueue(tenantCtx, "scoped:kind", 0, nil); err != nil {
		t.Fatalf("enqueue scoped: %v", err)
	}
	got, err := fx.Runs.ClaimDue(context.Background(), "owner-1", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("expected scoped run, got %v err=%v", got, err)
	}
	if got.TenantID != "tenant-x" {
		t.Fatalf("tenant: got %q want tenant-x", got.TenantID)
	}

	// No tenant in ctx → falls back to the default tenant, not "".
	if err := d.Enqueue(context.Background(), "unscoped:kind", 0, nil); err != nil {
		t.Fatalf("enqueue unscoped: %v", err)
	}
	got, err = fx.Runs.ClaimDue(context.Background(), "owner-2", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("expected unscoped run, got %v err=%v", got, err)
	}
	if got.TenantID != shared.DefaultTenantID {
		t.Fatalf("tenant fallback: got %q want default %q", got.TenantID, shared.DefaultTenantID)
	}
}

func TestRegistry_RejectsEmptyKind(t *testing.T) {
	t.Parallel()
	_, err := NewHandlerRegistry(map[string]Registration{
		"": {Handler: func(context.Context, *run.Run, Checkpointer) error { return nil }},
	}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "empty kind") {
		t.Fatalf("expected empty-kind error, got %v", err)
	}
}

func TestRegistry_RejectsNilHandler(t *testing.T) {
	t.Parallel()
	_, err := NewHandlerRegistry(map[string]Registration{
		"some:kind": {Handler: nil},
	}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "nil handler") {
		t.Fatalf("expected nil-handler error, got %v", err)
	}
}

func TestRegistry_RejectsNonPositiveDefaultLease(t *testing.T) {
	t.Parallel()
	for _, lease := range []time.Duration{0, -time.Second} {
		_, err := NewHandlerRegistry(map[string]Registration{}, lease)
		if err == nil || !strings.Contains(err.Error(), "default lease must be > 0") {
			t.Fatalf("lease=%s: expected default-lease error, got %v", lease, err)
		}
	}
}

// A payload that cannot be JSON-marshalled (a chan) must fail the enqueue with a
// wrapped "marshal payload" error AND persist no row — the validation between a
// caller and a corrupt/empty run.
func TestDispatcher_EnqueueMarshalErrorFails(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "run_dispatch_marshal.db"))
	d := NewDispatcher(fx.Runs, newRegistry(t, "agent"))

	err := d.Enqueue(context.Background(), "agent", 0, make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal payload") {
		t.Fatalf("expected a marshal-payload error, got %v", err)
	}

	got, cerr := fx.Runs.ClaimDue(context.Background(), "owner-1", time.Minute)
	if cerr != nil {
		t.Fatalf("claim: %v", cerr)
	}
	if got != nil {
		t.Fatalf("a marshal failure must not enqueue a row, got %+v", got)
	}
}
