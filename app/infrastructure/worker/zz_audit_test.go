package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	jobapp "gokick/app/application/job"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// backoff() is unexported; this file is package worker so it is callable
// directly. We assert literal durations (5s/10s/20s/1h), never the
// defaultBaseBackoff constant — comparing a value against the very constant
// it is built from is a tautology that would stay green if the constant
// changed. The literals pin the documented schedule.
//
// Closes:
//   - roadmap-30        (backoff = 2^(attempts-1)*5s, capped at 1h)
//   - infra-sched-job-25 (delay half: 5s/10s/20s; exhaustion half is covered elsewhere)
//   - infra-sched-job-37 (base backoff is 5s)
//   - infra-sched-job-38 (cap is 1h, plus the attempts<1 guard)
func TestBackoff_ExponentialScheduleAndCap(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 5 * time.Second},  // first failure → base 5s
		{2, 10 * time.Second}, // 2^1 * 5s
		{3, 20 * time.Second}, // 2^2 * 5s
		{4, 40 * time.Second}, // 2^3 * 5s
		{11, time.Hour},       // 2^10 * 5s = 5120s > 1h → capped
		{50, time.Hour},       // far past the cap stays capped
		{0, 5 * time.Second},  // attempts<1 clamp → base
		{-1, 5 * time.Second}, // negative clamp → base (not the overflow cap)
	}
	for _, c := range cases {
		if got := backoff(c.attempts); got != c.want {
			t.Fatalf("backoff(%d): got %v want %v", c.attempts, got, c.want)
		}
	}
}

// At-least-once delivery: a job whose lock has expired without MarkComplete is
// reclaimed and runs again. Built deterministically with SQL (no sleep): the
// ClaimDue lock duration is fmt.Sprintf("+%d seconds", int(lockFor.Seconds())),
// so a sub-second lockFor truncates to 0 and would never actually lock — we
// instead force locked_until into the past via UPDATE, exactly modelling a
// lease that elapsed mid-flight. run_at is untouched by ClaimDue so the row
// stays due. A second ClaimDue must return the SAME id with attempts bumped
// to 2, proving the (locked_until < now) reclaim branch double-delivers.
//
// Closes:
//   - infra-sched-job-17 (at-least-once delivery)
//   - roadmap-29         (at-least-once; handlers must be idempotent)
func TestWorker_ExpiredLockIsReclaimed_AtLeastOnce(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_reclaim.db"))
	ctx := context.Background()
	j := enqueue(t, fx, "reclaim:kind", 4)

	first, err := fx.Jobs.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if first == nil || first.ID != j.ID {
		t.Fatalf("first claim must return the enqueued job, got %v", first)
	}
	if first.Attempts != 1 {
		t.Fatalf("first claim attempts: got %d want 1", first.Attempts)
	}
	if first.LockedUntil == nil {
		t.Fatal("first claim must set locked_until (job is now leased)")
	}

	// Expire the lease without completing — models a crash/timeout after the
	// handler ran a side effect but before MarkComplete.
	if _, err := fx.DB.DB().Exec(
		`UPDATE jobs SET locked_until = strftime('%Y-%m-%d %H:%M:%f','now','-60 seconds') WHERE id = ?`,
		j.ID,
	); err != nil {
		t.Fatalf("expire lock: %v", err)
	}

	second, err := fx.Jobs.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second == nil {
		t.Fatal("expired-lock job must be reclaimable (at-least-once), got nil")
	}
	if second.ID != j.ID {
		t.Fatalf("reclaim must return the SAME job id: got %s want %s", second.ID, j.ID)
	}
	if second.Attempts != 2 {
		t.Fatalf("reclaim attempts: got %d want 2 (double delivery)", second.Attempts)
	}
}

// On a handler panic with retries remaining, the worker must reschedule, not
// permanently fail. The discriminator is FailedAt: MarkFailed sets it,
// Reschedule leaves it nil. We do NOT assert a subsequent ClaimDue re-returns
// the job — Reschedule pushes run_at to now+backoff(1)=now+5s, so an immediate
// claim returns nil (not yet due). FailedAt==nil uniquely identifies the
// Reschedule branch on the panic path.
//
// Closes:
//   - infra-sched-job-32 (panic → caught, logged, rescheduled — not MarkFailed)
func TestWorker_HandlerPanics_Reschedules_NotFailed(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_panic_resched.db"))
	j := enqueue(t, fx, "panic:resched", 2) // maxRetries>=1 so a retry remains

	w := newWorker(t, fx, "panic:resched", func(_ context.Context, _ []byte) error {
		panic("kaboom")
	})

	runOnce(t, w)

	got, _ := fx.Jobs.FindByID(context.Background(), j.ID)
	if got == nil {
		t.Fatal("job row vanished")
	}
	if got.FailedAt != nil {
		t.Fatalf("panic with retries left must reschedule, not fail; FailedAt=%v", got.FailedAt)
	}
	if got.CompletedAt != nil {
		t.Fatal("panicking handler must not mark complete")
	}
	if got.LockedUntil != nil {
		t.Fatal("Reschedule must clear locked_until")
	}
	if got.LastError == nil {
		t.Fatal("panic path must record a LastError")
	}
}

// The job lock lease (lockFor) is 5 minutes. We pin the defaultLockFor
// constant by calling ClaimDue(defaultLockFor) directly and measuring the
// stored locked_until against 'now' ENTIRELY in SQL. Doing the delta in
// SQLite (julianday math, all UTC) sidesteps any Go-side timezone parsing of
// the naked datetime string — a Go WithinDuration compare could be hours off
// on a non-UTC host. We do not run the worker because a completed/failed job
// clears locked_until back to NULL.
//
// Closes:
//   - infra-sched-job-39 (lock timeout is 5 minutes)
func TestWorker_DefaultLockFor_IsFiveMinutes(t *testing.T) {
	if defaultLockFor != 5*time.Minute {
		t.Fatalf("defaultLockFor source constant: got %v want 5m", defaultLockFor)
	}

	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_lockfor.db"))
	ctx := context.Background()
	j := enqueue(t, fx, "lock:kind", 2)

	claimed, err := fx.Jobs.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil || claimed.ID != j.ID {
		t.Fatalf("claim must return the enqueued job, got %v", claimed)
	}

	var deltaSeconds float64
	if err := fx.DB.DB().GetContext(ctx, &deltaSeconds,
		`SELECT (julianday(locked_until) - julianday('now')) * 86400.0 FROM jobs WHERE id = ?`,
		j.ID,
	); err != nil {
		t.Fatalf("measure lease: %v", err)
	}
	// 5 minutes = 300s. Allow a generous lower bound for test-run latency and
	// a tiny upper slack; a 1m or 10m constant would blow past this window.
	if deltaSeconds < 290 || deltaSeconds > 301 {
		t.Fatalf("lease delta: got %.2fs want ~300s (5m lock)", deltaSeconds)
	}
}

// Cascade jobs: a running handler can enqueue further jobs because the worker
// injects a real JobDispatcher into the handler's tx ctx. newWorker hardcodes
// a noop dispatcher + single-kind registry, so we build the worker inline with
// a registry that knows BOTH parent and child (the dispatcher's Enqueue calls
// registry.Has(kind) and errors on an unknown child) and a real NewDispatcher.
// We assert the child row was committed via raw SQL count — not its
// pending/complete state, since the worker may not pick the child up within
// the single runOnce poll window.
//
// Closes:
//   - infra-sched-job-30 (cascade jobs allowed via JobDispatcher in ctx)
func TestWorker_HandlerCanEnqueueChildJob(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_cascade_ok.db"))
	ctx := context.Background()

	var childEnqueueErr error
	registry, err := jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{
		"parent": func(hctx context.Context, _ []byte) error {
			childEnqueueErr = shared.JobDispatcherFromContext(hctx).
				Enqueue(hctx, "child", 0, map[string]string{"from": "parent"})
			return childEnqueueErr
		},
		"child": func(_ context.Context, _ []byte) error { return nil },
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	dispatcher := jobapp.NewDispatcher(fx.Jobs, registry)
	w := NewWorker(silentLogger(), shared.NopReporter{}, fx.Jobs, registry, fx.DB, dispatcher, 1)

	parent := enqueue(t, fx, "parent", 2)

	runOnce(t, w)

	if childEnqueueErr != nil {
		t.Fatalf("handler's child Enqueue failed: %v", childEnqueueErr)
	}

	// Parent committed successfully (its child enqueue was inside the same tx).
	gotParent, _ := fx.Jobs.FindByID(ctx, parent.ID)
	if gotParent == nil || gotParent.CompletedAt == nil {
		t.Fatalf("parent must complete; got %v", gotParent)
	}

	var childCount int
	if err := fx.DB.DB().GetContext(ctx, &childCount,
		`SELECT COUNT(*) FROM jobs WHERE kind = 'child'`); err != nil {
		t.Fatalf("count child jobs: %v", err)
	}
	if childCount != 1 {
		t.Fatalf("handler must enqueue exactly one child job, got %d", childCount)
	}
}

// NOTE: the startup log-line claim (infra-sched-job-41) is intentionally NOT
// tested here: empty smallestTestToClose, low importance, and a bare
// log-string assertion is brittle. It is reported as skipped.
