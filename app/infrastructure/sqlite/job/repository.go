package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gokick/app/domain/job"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Enqueue(ctx context.Context, j *job.Job) error {
	const q = `INSERT INTO jobs (id, kind, payload, run_at, attempts, max_retries, locked_until, last_error, failed_at, completed_at, created_at)
		VALUES (:id, :kind, :payload, :run_at, :attempts, :max_retries, :locked_until, :last_error, :failed_at, :completed_at, :created_at)`
	row := *j
	row.RunAt = msPrecisionUTC(row.RunAt)
	row.CreatedAt = msPrecisionUTC(row.CreatedAt)
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, &row)
	return err
}

// msPrecisionUTC normalizes a Go time.Time to UTC + millisecond precision
// before it crosses into SQLite. Required because ncruces' WASM SQLite
// returns 'now' from a clock that ticks at ~1 ms granularity and trails
// Go's time.Now() by up to ~1 ms on the same wall clock. A job written
// with µs precision (e.g. .467806) can therefore beat 'now' in the
// julianday(run_at) <= julianday('now') check despite being "in the
// past" on the real clock — ClaimDue then misses a freshly-enqueued row
// at random. Truncating writes to the lowest common precision removes
// the race; downstream reads round-trip the exact same ms value.
func msPrecisionUTC(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

// ClaimDue atomically locks the next due row in a single UPDATE … RETURNING.
// SQLite serializes writers, so concurrent claim attempts queue rather than
// race; the LIMIT 1 + locked_until guard guarantees each row goes to at most
// one worker.
//
// All time comparisons go through julianday() (double-precision Julian Day):
//
//  1. Timezone-aware: julianday() parses Go's RFC3339 with offset (e.g.
//     +02:00) and SQLite's UTC 'now' to the same scalar, so a written
//     run_at compares correctly against 'now' regardless of TZ.
//  2. Sub-millisecond precision: a Go time.Time has microsecond resolution.
//     strftime('%f', t) rounds to whole milliseconds (round-half-up), which
//     pushes any value with µs >= 500 a tick ahead of 'now' even though
//     real time has already passed it — a job enqueued at T.467806 would
//     have run_at_str=".468" and lose the race against now_str=".467".
//     julianday() keeps the full double precision and avoids the rounding
//     entirely; it's also what WithDelay(800ms) needs for sub-second tests.
func (r *Repository) ClaimDue(ctx context.Context, lockFor time.Duration) (*job.Job, error) {
	const q = `
		UPDATE jobs
		SET attempts = attempts + 1,
		    locked_until = strftime('%Y-%m-%d %H:%M:%f', 'now', ?)
		WHERE id = (
		    SELECT id FROM jobs
		    WHERE completed_at IS NULL
		      AND failed_at IS NULL
		      AND julianday(run_at) <= julianday('now')
		      AND (locked_until IS NULL
		           OR julianday(locked_until) < julianday('now'))
		    ORDER BY julianday(run_at)
		    LIMIT 1
		)
		RETURNING *`

	var j job.Job
	lockExpr := fmt.Sprintf("+%d seconds", int(lockFor.Seconds()))
	err := r.Conn(ctx).GetContext(ctx, &j, q, lockExpr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Repository) MarkComplete(ctx context.Context, id string) error {
	_, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE jobs SET completed_at = datetime('now'), locked_until = NULL WHERE id = ?`, id)
	return err
}

func (r *Repository) Reschedule(
	ctx context.Context,
	id string,
	runAt time.Time,
	lastErr string,
) error {
	_, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE jobs SET run_at = ?, last_error = ?, locked_until = NULL WHERE id = ?`,
		msPrecisionUTC(runAt), lastErr, id)
	return err
}

func (r *Repository) MarkFailed(ctx context.Context, id string, lastErr string) error {
	_, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE jobs SET failed_at = datetime('now'), last_error = ?, locked_until = NULL WHERE id = ?`,
		lastErr, id)
	return err
}

func (r *Repository) FindByID(ctx context.Context, id string) (*job.Job, error) {
	var j job.Job
	err := r.Conn(ctx).GetContext(ctx, &j, `SELECT * FROM jobs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}
