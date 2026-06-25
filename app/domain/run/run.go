// Package run is the durable long-running-work bounded context — the gokick
// primitive for "agents": background work that runs for minutes/hours, persists
// resumable state, and survives a worker crash by being reclaimed and resumed.
//
// It is the long-running sibling of the job context. The crucial difference is
// the execution model (see ADR-0001): a job handler runs INSIDE one transaction
// (atomic, but it holds SQLite's single write lock for its whole duration — fine
// only for short work); a run handler runs OUTSIDE any transaction and persists
// progress via short owner-checked Checkpoint writes, while a heartbeat renews a
// lease. gokick supplies these primitives; the agent's own loop lives in the app.
package run

import (
	"time"

	"github.com/google/uuid"
)

// Run is a persisted unit of durable long-running work. State is DERIVED from
// columns, mirroring job.Job:
//   - completed_at != nil → succeeded
//   - failed_at != nil    → permanently failed
//   - locked_until > now  → currently running under a lease held by locked_by
//   - otherwise           → pending / reclaimable
//
// Two distinct counters, deliberately NOT merged (unlike jobs, where a claim ==
// an attempt):
//   - Attempts: LOGIC retries — bumped only when Reschedule runs (the handler
//     returned a retryable error). max_retries governs this counter.
//   - Reclaims: CRASH reclaims — bumped when ClaimDue reclaims an EXPIRED lease
//     (a worker died mid-run). A long run that merely straddles a deploy/OOM must
//     not burn its logic-retry budget, so reclaims are counted and bounded apart.
type Run struct {
	ID   string `db:"id"`
	Kind string `db:"kind"`
	// TenantID is the tenant the run was enqueued for; the worker restores it into
	// the handler ctx from the claimed row (it bypasses the bus). ClaimDue is a
	// global drain (no tenant filter) — this only propagates.
	TenantID    string     `db:"tenant_id"`
	Payload     []byte     `db:"payload"`      // immutable initial input
	State       []byte     `db:"state"`        // latest checkpoint; nil (NULL) until the first Checkpoint
	RunAt       time.Time  `db:"run_at"`       // when eligible to claim
	Attempts    int        `db:"attempts"`     // logic retries (bumped on Reschedule)
	Reclaims    int        `db:"reclaims"`     // crash reclaims (bumped on ClaimDue of an expired lease)
	MaxRetries  int        `db:"max_retries"`  // caps Attempts only, never Reclaims
	LockedBy    *string    `db:"locked_by"`    // owner token of the worker holding the lease (a per-claim nonce)
	LockedUntil *time.Time `db:"locked_until"` // lease expiry; the heartbeat renews it
	LastError   *string    `db:"last_error"`
	FailedAt    *time.Time `db:"failed_at"`
	CompletedAt *time.Time `db:"completed_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"` // bumped by every mutating repo write (claim/renew/checkpoint/finalize)
}

// NewRun constructs a fresh pending Run with a uuid id, RunAt=now, and zero
// attempts/reclaims. maxRetries must be >= 0 (validated by the dispatcher); it
// caps logic retries only, never crash reclaims.
func NewRun(kind string, payload []byte, maxRetries int) *Run {
	now := time.Now()
	return &Run{
		ID:         uuid.NewString(),
		Kind:       kind,
		Payload:    payload,
		RunAt:      now,
		MaxRetries: maxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
