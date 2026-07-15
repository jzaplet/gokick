// Package run is the durable long-running-work bounded context — the gokick
// primitive for "agents": background work that runs for minutes/hours, persists
// resumable state, and survives a worker crash by being reclaimed and resumed.
//
// The crucial property is the execution model: a run handler runs OUTSIDE any
// transaction. A transaction would hold SQLite's single write lock for the
// handler's whole duration — fine only for the short, own-DB-only work a command
// does. A run instead persists progress via short owner-checked Checkpoint
// writes, while a heartbeat renews a lease. gokick supplies these primitives;
// the agent's own loop lives in the app.
package run

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Construction invariants, enforced by NewRun so no caller can persist a Run
// that violates them (the dispatcher is not the only enqueue path — debug_run
// and any future caller go through NewRun too).
var (
	ErrEmptyKind       = errors.New("run: kind must not be empty")
	ErrNegativeRetries = errors.New("run: maxRetries must be >= 0")
)

// Run is a persisted unit of durable long-running work. State is DERIVED from
// columns:
//   - completed_at != nil → succeeded
//   - failed_at != nil    → permanently failed
//   - locked_until > now  → currently running under a lease held by locked_by
//   - otherwise           → pending / reclaimable
//
// Three distinct counters, deliberately NOT merged (a claim is NOT an attempt):
//   - Attempts: LOGIC retries — bumped only when Reschedule runs (the handler
//     returned a retryable error). max_retries governs this counter.
//   - Reclaims: CRASH reclaims — bumped when ClaimDue reclaims an EXPIRED lease
//     (a worker died mid-run). A long run that merely straddles a deploy/OOM must
//     not burn its logic-retry budget, so reclaims are counted and bounded apart.
//   - Parks: REGISTRY-SKEW parks — bumped when Park reschedules an unknown-kind run
//     during a rolling deploy (a binary without the handler claimed it). Bounded by
//     the worker INDEPENDENTLY of max_retries, so deploy timing never consumes the
//     handler's logic-retry budget.
//
// INCREMENT TIMING dictates each budget check's operator — do NOT "unify" them:
// Reclaims is bumped by ClaimDue (in SQL) BEFORE failIfPoison sees it, so that
// check is `>` (post-increment). Attempts and Parks are bumped by Reschedule /
// Park AFTER their checks run, so those checks are `>=` (pre-increment). Moving
// any increment across its check requires flipping that check's operator; the
// poison-boundary test (run_worker_test.go, MaxReclaims+1) pins the reclaims side.
type Run struct {
	ID   string `db:"id"`
	Kind string `db:"kind"`
	// TenantID is the tenant the run was enqueued for; the worker restores it into
	// the handler ctx from the claimed row (it bypasses the bus). ClaimDue is a
	// global drain (no tenant filter) — this only propagates.
	TenantID    string     `db:"tenant_id"`
	Payload     []byte     `db:"payload"`      // immutable initial input
	State       []byte     `db:"state"`        // latest checkpoint; empty (len 0) until the first Checkpoint
	RunAt       time.Time  `db:"run_at"`       // when eligible to claim
	Attempts    int        `db:"attempts"`     // logic retries (bumped on Reschedule)
	Reclaims    int        `db:"reclaims"`     // crash reclaims (bumped on ClaimDue of an expired lease)
	Parks       int        `db:"parks"`        // registry-skew parks (bumped on Park); bounded independently of MaxRetries
	MaxRetries  int        `db:"max_retries"`  // caps Attempts only, never Reclaims/Parks
	LockedBy    *string    `db:"locked_by"`    // owner token of the worker holding the lease (a per-claim nonce)
	LockedUntil *time.Time `db:"locked_until"` // lease expiry; the heartbeat renews it
	LastError   *string    `db:"last_error"`
	FailedAt    *time.Time `db:"failed_at"`
	CompletedAt *time.Time `db:"completed_at"`
	// CancelRequested is the operator signal; the worker observes it and winds the
	// handler down, then records CancelledAt. It survives a reclaim (rides on the row).
	CancelRequested bool       `db:"cancel_requested"`
	CancelledAt     *time.Time `db:"cancelled_at"` // terminal: cancellation recorded by the worker
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"` // bumped by every mutating repo write (claim/renew/checkpoint/finalize)
}

// NewRun constructs a fresh pending Run with a uuid id, RunAt=now, and zero
// attempts/reclaims. It enforces the construction invariants (non-empty kind,
// maxRetries >= 0) so an invalid Run can never be persisted, whatever the
// enqueue path. maxRetries caps logic retries only, never crash reclaims.
func NewRun(kind string, payload []byte, maxRetries int) (*Run, error) {
	if kind == "" {
		return nil, ErrEmptyKind
	}
	if maxRetries < 0 {
		return nil, ErrNegativeRetries
	}
	now := time.Now()
	return &Run{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Kind:       kind,
		Payload:    payload,
		RunAt:      now,
		MaxRetries: maxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// IsTerminal reports whether the run has reached a terminal state — one of the
// three terminal timestamps is set. This is the domain mirror of the SQL
// sqlite.NotTerminalClause (which encodes NOT IsTerminal); both express the one
// terminal-state rule, from the entity's side and the query's side.
func (r *Run) IsTerminal() bool {
	return r.CompletedAt != nil || r.FailedAt != nil || r.CancelledAt != nil
}

// Status derives the observable lifecycle label from the row. The three terminal
// labels come from the terminal timestamps (the IsTerminal rule); a lease still in
// the future reads as "running", otherwise "pending".
//
// This is the ONE place the read path derives status — a presentation handler must
// call it, not re-express the switch. NOTE: the running/pending split uses the
// passed wall clock for an OBSERVATIONAL label only; lease/fencing DECISIONS in the
// repo use the DB clock and the owner-checked fencing bool, never this.
func (r *Run) Status(now time.Time) string {
	switch {
	case r.CompletedAt != nil:
		return "completed"
	case r.FailedAt != nil:
		return "failed"
	case r.CancelledAt != nil:
		return "cancelled"
	case r.LockedUntil != nil && r.LockedUntil.After(now):
		return "running"
	default:
		return "pending"
	}
}
