package job

import (
	"time"

	"github.com/google/uuid"
)

// Job is a persisted unit of background work. State is derived from columns:
//   - completed_at != nil → succeeded
//   - failed_at != nil    → permanently failed (max_attempts exhausted)
//   - locked_until > now  → currently being processed
//   - otherwise           → pending / retryable
//
// Persistence sets created_at via the DB DEFAULT.
type Job struct {
	ID           string     `db:"id"`
	Kind         string     `db:"kind"`
	Payload      []byte     `db:"payload"`
	RunAt        time.Time  `db:"run_at"`
	Attempts     int        `db:"attempts"`
	MaxAttempts  int        `db:"max_attempts"`
	LockedUntil  *time.Time `db:"locked_until"`
	LastError    *string    `db:"last_error"`
	FailedAt     *time.Time `db:"failed_at"`
	CompletedAt  *time.Time `db:"completed_at"`
	CreatedAt    time.Time  `db:"created_at"`
}

// NewJob constructs a fresh pending Job with a UUIDv7 id and RunAt=now.
// MaxAttempts defaults to DefaultMaxAttempts when caller passes 0.
func NewJob(kind string, payload []byte, maxAttempts int) *Job {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	return &Job{
		ID:          uuid.NewString(),
		Kind:        kind,
		Payload:     payload,
		RunAt:       time.Now(),
		Attempts:    0,
		MaxAttempts: maxAttempts,
		CreatedAt:   time.Now(),
	}
}

// DefaultMaxAttempts is applied when JobDispatcher.Enqueue doesn't specify.
// One attempt = no retries. Caller opts into retries with shared.WithMaxAttempts(n)
// — picking a retry count is a deliberate decision per job kind, not a default.
const DefaultMaxAttempts = 1
