package worker

import (
	"context"
	"path/filepath"
	"testing"

	jobapp "gokick/app/application/job"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// Default worker concurrency is 1: NewWorker clamps any non-positive request
// (0 or negative) up to a single goroutine so the pool always runs at least
// one loop. A positive value is preserved verbatim. The constructor never
// touches repo/tx/dispatcher, so this is a pure unit test of the clamp — no DB
// fixture required.
//
// Closes:
//   - roadmap-32 (default worker concurrency is 1; NewWorker(0) and NewWorker(-1) -> 1)
func TestNewWorker_DefaultConcurrency(t *testing.T) {
	registry, err := jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{
		"noop": func(_ context.Context, _ []byte) error { return nil },
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	cases := []struct {
		name    string
		request int
		want    int
	}{
		{"zero clamps to one", 0, 1},
		{"negative clamps to one", -1, 1},
		{"large negative clamps to one", -100, 1},
		{"positive is preserved", 4, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := NewWorker(
				silentLogger(),
				shared.NopReporter{},
				nil,
				registry,
				nil,
				noopDispatcher(),
				c.request,
			)
			if w.concurrency != c.want {
				t.Fatalf("NewWorker(..., %d).concurrency: got %d want %d",
					c.request, w.concurrency, c.want)
			}
		})
	}
}

// Crash-resume safety: a live lock prevents double-claim. When a job is claimed
// with the real 5m lockFor, an *immediate* second ClaimDue must return nil —
// the locked_until guard in the ClaimDue WHERE clause hides a job that is
// currently leased to another worker. Only after the lease expires does the job
// become claimable again (the at-least-once resume).
//
// This pins the negative half that TestWorker_ExpiredLockIsReclaimed_AtLeastOnce
// does NOT assert: that test forces locked_until into the past and proves
// reclaim works, but it never proves a *held* lock blocks a concurrent claim.
// A mutant dropping the `locked_until IS NULL OR julianday(locked_until) <
// julianday('now')` guard would still pass the at-least-once test (the row is
// unlocked-in-past + due) yet fail here, because the still-leased job would be
// handed out a second time.
//
// We use defaultLockFor (5m) deliberately, not a sub-second lockFor: ClaimDue
// formats the lease as fmt.Sprintf("+%d seconds", int(lockFor.Seconds())), so
// any sub-second duration truncates to "+0 seconds" (locked_until = now) and
// would NOT actually hold the lock — the immediate re-claim would spuriously
// succeed. A multi-second lease is required to observe the held-lock branch.
//
// Closes:
//   - infra-sched-job-40 (on crash/restart the worker resumes from where it
//     left off: a held lease blocks reclaim, an expired lease is reclaimable)
func TestWorker_HeldLockBlocksReclaim_ResumesAfterExpiry(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "worker_held_lock.db"))
	ctx := context.Background()
	j := enqueue(t, fx, "lease:kind", 4)

	// Claim with the real 5m lease — the job is now leased to "this worker".
	first, err := fx.Jobs.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if first == nil || first.ID != j.ID {
		t.Fatalf("first claim must return the enqueued job, got %v", first)
	}
	if first.LockedUntil == nil {
		t.Fatal("first claim must set locked_until (job is now leased)")
	}

	// Crash-resume guard: while the lease is live, a second worker's ClaimDue
	// must find nothing — the held lock prevents a duplicate, concurrent claim.
	blocked, err := fx.Jobs.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		t.Fatalf("second claim (while locked): %v", err)
	}
	if blocked != nil {
		t.Fatalf("a held lease must block reclaim; got duplicate claim of job %s", blocked.ID)
	}

	// Simulate the original worker crashing mid-flight: the lease elapses
	// without MarkComplete. The job must now resume (become claimable again).
	if _, err := fx.DB.DB().Exec(
		`UPDATE jobs SET locked_until = strftime('%Y-%m-%d %H:%M:%f','now','-1 seconds') WHERE id = ?`,
		j.ID,
	); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	resumed, err := fx.Jobs.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		t.Fatalf("resume claim: %v", err)
	}
	if resumed == nil || resumed.ID != j.ID {
		t.Fatalf("after lease expiry the job must resume (be reclaimable); got %v", resumed)
	}
	if resumed.Attempts != 2 {
		t.Fatalf("resumed attempts: got %d want 2 (first claim + resume)", resumed.Attempts)
	}
}
