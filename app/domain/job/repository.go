package job

import (
	"context"
	"time"
)

// Repository is the domain port for the perzistent job queue.
//
// ClaimDue is the only non-trivial contract: it must atomically pick one
// due job and mark it locked, so concurrent workers cannot pick the same
// row. Implementations rely on UPDATE … RETURNING in a single SQL statement.
type Repository interface {
	// Enqueue persists a new job. The Job's RunAt determines when ClaimDue
	// will return it. Conn(ctx) is honored — when called from a CommandBus
	// handler, the insert lands in the same transaction as the business write.
	Enqueue(ctx context.Context, j *Job) error

	// ClaimDue atomically selects one pending job with run_at <= now() and
	// (locked_until IS NULL OR locked_until < now()), bumps its attempts,
	// sets locked_until = now() + lockFor, and returns it. Returns (nil, nil)
	// when no job is due.
	ClaimDue(ctx context.Context, lockFor time.Duration) (*Job, error)

	// MarkComplete sets completed_at = now(). Intended to be called from
	// within the worker's transaction so handler success and completion
	// commit atomically.
	MarkComplete(ctx context.Context, id string) error

	// Reschedule updates run_at, last_error, and clears locked_until so the
	// next ClaimDue call may pick it up. Used for retryable failures.
	Reschedule(ctx context.Context, id string, runAt time.Time, lastErr string) error

	// MarkFailed records terminal failure (attempts exhausted or kind unknown).
	// Sets failed_at = now() and clears locked_until.
	MarkFailed(ctx context.Context, id string, lastErr string) error

	// FindByID is used by tests to inspect job state.
	FindByID(ctx context.Context, id string) (*Job, error)
}
