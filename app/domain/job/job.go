package job

import (
	"time"

	"github.com/google/uuid"
)

// Job is a persisted unit of background work. State is derived from columns:
//   - completed_at != nil → succeeded
//   - failed_at != nil    → permanently failed (retries exhausted)
//   - locked_until > now  → currently being processed
//   - otherwise           → pending / retryable
//
// Persistence sets created_at via the DB DEFAULT.
type Job struct {
	ID          string     `db:"id"`
	Kind        string     `db:"kind"`
	Payload     []byte     `db:"payload"`
	RunAt       time.Time  `db:"run_at"`
	Attempts    int        `db:"attempts"`    // 0-based count of how many times claim has run this job
	MaxRetries  int        `db:"max_retries"` // 0 = no retry, just the first attempt; N = up to N retries after the first failure
	LockedUntil *time.Time `db:"locked_until"`
	LastError   *string    `db:"last_error"`
	FailedAt    *time.Time `db:"failed_at"`
	CompletedAt *time.Time `db:"completed_at"`
	CreatedAt   time.Time  `db:"created_at"`
}

// NewJob constructs a fresh pending Job with a UUIDv7 id and RunAt=now.
// maxRetries must be >= 0 — callers are required to pick a value explicitly
// (0 = run once with no retry; pick higher for flaky external work).
func NewJob(kind string, payload []byte, maxRetries int) *Job {
	return &Job{
		ID:         uuid.NewString(),
		Kind:       kind,
		Payload:    payload,
		RunAt:      time.Now(),
		Attempts:   0,
		MaxRetries: maxRetries,
		CreatedAt:  time.Now(),
	}
}
