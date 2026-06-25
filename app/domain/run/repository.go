package run

import (
	"context"
	"time"
)

// Repository is the domain port for the durable run queue — the long-running
// sibling of job.Repository, with the additions the outside-transaction model
// requires (see ADR-0001):
//
//   - Owner token (locked_by): every claim stamps a fresh per-claim nonce. The
//     mutating methods are owner-checked (WHERE locked_by = ?) so a worker that
//     lost its lease (stalled past expiry, then another worker reclaimed) cannot
//     stomp the run — its UPDATE matches zero rows. This is fencing.
//   - Heartbeat (RenewLease) and Checkpoint renew the lease, so a live long run
//     is not reclaimed; if the worker dies, the lease lapses and another worker
//     reclaims and resumes from the last checkpoint.
//   - The bool return: true = the owner-checked UPDATE affected its one row;
//     (false, nil) = ownership lost → the caller must ABANDON (fencing); (_, err)
//     = the write itself failed (transient contention) — distinct from ownership
//     loss; retry a bounded number of times.
//
// Execution context: Enqueue is called from a command handler and honors
// Conn(ctx) — its INSERT joins the business transaction (atomic enqueue). Every
// OTHER method is called by the worker, which runs with NO transaction in ctx,
// so they execute as standalone autocommit writes on the pool — committed
// independently and immediately visible to other workers (a heartbeat must be
// visible at once). They must never run inside a long-lived transaction.
//
// Time is DB-clock-sourced: lease expiry is computed and compared by SQLite
// (never by a worker's wall clock), so multi-worker lease decisions cannot skew.
// Application code must judge liveness from the bool these methods return, never
// by comparing a stored LockedUntil against its own clock.
//
// owner MUST be a per-claim nonce (a fresh uuid per claim), never a stable worker
// id — otherwise a worker cannot be fenced against its own earlier, abandoned
// in-flight goroutine.
type Repository interface {
	// Enqueue persists a new pending run. Honors Conn(ctx): from a CommandBus
	// handler the INSERT joins the same transaction as the business write.
	Enqueue(ctx context.Context, r *Run) error

	// ClaimDue atomically claims the oldest due run (run_at <= now, not locked,
	// not completed/failed) for owner: it stamps locked_by = owner, sets
	// locked_until = now + lease, and — ONLY when reclaiming an expired lease —
	// bumps reclaims. It does NOT bump attempts (a claim is not a logic retry).
	// Returns (nil, nil) when nothing is due. lease must be > 0.
	//
	// Atomic single-row claim is the CONTRACT; the mechanism is adapter-specific
	// (SQLite: writer serialization + UPDATE…RETURNING; Postgres: SELECT … FOR
	// UPDATE SKIP LOCKED). Two workers can never claim the same run.
	ClaimDue(ctx context.Context, owner string, lease time.Duration) (*Run, error)

	// RenewLease extends the lease (locked_until = now + lease) iff the run is
	// still owned by owner and not terminal — the heartbeat. (false, nil) = lease
	// lost. lease must be > 0.
	RenewLease(ctx context.Context, id, owner string, lease time.Duration) (bool, error)

	// Checkpoint persists resumable state AND renews the lease (a successful
	// checkpoint proves liveness) iff still owned and not terminal. state is stored
	// verbatim as a BLOB; binary preserved. The driver maps both a NULL column and
	// an empty blob to a zero-length slice, so a handler treats len(State)==0 as
	// resume-from-scratch. On (false, nil) = lease lost, the stale state is NOT
	// written. lease must be > 0.
	Checkpoint(
		ctx context.Context,
		id, owner string,
		state []byte,
		lease time.Duration,
	) (bool, error)

	// MarkComplete records terminal success (completed_at = now) and clears the
	// lock, iff still owned and not already terminal. (false, nil) = lease lost or
	// already terminal.
	MarkComplete(ctx context.Context, id, owner string) (bool, error)

	// Reschedule requeues the run for a retryable failure: sets run_at + last_error,
	// bumps attempts (the logic-retry counter), and clears the lock — iff still
	// owned and not terminal. (false, nil) = lease lost.
	Reschedule(ctx context.Context, id, owner string, runAt time.Time, lastErr string) (bool, error)

	// MarkFailed records terminal failure (failed_at = now, last_error) and clears
	// the lock, iff still owned and not already terminal. The last checkpoint state
	// is preserved for postmortem. (false, nil) = lease lost or already terminal.
	MarkFailed(ctx context.Context, id, owner string, lastErr string) (bool, error)

	// RequestCancel sets the operator cancel signal on a non-terminal run. It is
	// NOT owner-checked (the operator is not the worker) and is idempotent — a
	// no-op on a terminal or missing run. cancel_requested rides on the row, so a
	// worker that reclaims a crashed run honors a pending cancel. The worker
	// observes the signal, winds the handler down, and records the terminal state
	// via MarkCancelled.
	RequestCancel(ctx context.Context, id string) error

	// MarkCancelled records terminal cancellation (cancelled_at = now) and clears
	// the lock, iff still owned and not already terminal — the worker's response to
	// the cancel signal. The last checkpoint state is preserved. (false, nil) =
	// lease lost or already terminal.
	MarkCancelled(ctx context.Context, id, owner string) (bool, error)

	// IsCancelRequested reports whether the operator cancel signal is set on the run
	// — the worker's heartbeat polls it to observe a mid-run cancel. It reads only
	// the cancel_requested flag, NOT the whole row, so it never transfers the payload
	// or the (potentially large, growing) state BLOB on every heartbeat tick. Returns
	// (false, nil) when the run is absent.
	IsCancelRequested(ctx context.Context, id string) (bool, error)

	// FindByID returns the run, or (nil, nil) when absent.
	FindByID(ctx context.Context, id string) (*Run, error)
}
