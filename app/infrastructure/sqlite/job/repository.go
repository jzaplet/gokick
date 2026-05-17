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
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, j)
	return err
}

// ClaimDue atomically locks the next due row in a single UPDATE … RETURNING.
// SQLite serializes writers, so concurrent claim attempts queue rather than
// race; the LIMIT 1 + locked_until guard guarantees each row goes to at most
// one worker. strftime(...%f) wraps every column comparison for two reasons:
// (1) Go time.Time serializes with timezone offset (e.g. +02:00) while
// sqlite's 'now' is UTC without timezone — direct string compare would
// lex-mismatch; (2) plain datetime() truncates fractional seconds, which
// breaks sub-second delays like WithDelay(800ms) where two rows can share
// the same whole-second value.
func (r *Repository) ClaimDue(ctx context.Context, lockFor time.Duration) (*job.Job, error) {
	const q = `
		UPDATE jobs
		SET attempts = attempts + 1,
		    locked_until = strftime('%Y-%m-%d %H:%M:%f', 'now', ?)
		WHERE id = (
		    SELECT id FROM jobs
		    WHERE completed_at IS NULL
		      AND failed_at IS NULL
		      AND strftime('%Y-%m-%d %H:%M:%f', run_at) <= strftime('%Y-%m-%d %H:%M:%f', 'now')
		      AND (locked_until IS NULL
		           OR strftime('%Y-%m-%d %H:%M:%f', locked_until)
		              < strftime('%Y-%m-%d %H:%M:%f', 'now'))
		    ORDER BY strftime('%Y-%m-%d %H:%M:%f', run_at)
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
		runAt, lastErr, id)
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
