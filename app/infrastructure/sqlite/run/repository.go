// Package run implements run.Repository on SQLite. It mirrors the job repo's
// time discipline (julianday comparisons, ms-precision writes) and adds the
// owner-token fencing the durable model needs: every mutating method is
// owner-checked and returns whether it affected its one row, so a worker that
// lost its lease cannot stomp a run another worker reclaimed.
package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

// nowExpr writes the DB clock at ms precision (for completed_at/failed_at/updated_at).
const nowExpr = `strftime('%Y-%m-%d %H:%M:%f', 'now')`

// leaseExpr computes locked_until = now + lease entirely in SQLite, sub-second
// precise: julianday('now') is a double (days), + lease_seconds/86400 adds the
// lease as a fraction of a day, strftime formats it back at ms precision. Unlike
// the jobs '+%d seconds' form it does NOT truncate a sub-second lease to +0s. The
// bound parameter is lease.Seconds() (a float64).
const leaseExpr = `strftime('%Y-%m-%d %H:%M:%f', julianday('now') + ? / 86400.0)`

func (r *Repository) Enqueue(ctx context.Context, rn *run.Run) error {
	const q = `INSERT INTO runs (id, kind, tenant_id, payload, state, run_at, attempts, reclaims, max_retries, locked_by, locked_until, last_error, failed_at, completed_at, cancel_requested, cancelled_at, created_at, updated_at)
		VALUES (:id, :kind, :tenant_id, :payload, :state, :run_at, :attempts, :reclaims, :max_retries, :locked_by, :locked_until, :last_error, :failed_at, :completed_at, :cancel_requested, :cancelled_at, :created_at, :updated_at)`
	row := *rn
	// Never write an empty tenant: an explicit "" would override the column's
	// NOT NULL DEFAULT and break scoping. The dispatcher stamps the real tenant
	// from ctx; this is the fallback for a direct Enqueue.
	if row.TenantID == "" {
		row.TenantID = shared.DefaultTenantID
	}
	row.RunAt = sqlite.MsPrecisionUTC(row.RunAt)
	row.CreatedAt = sqlite.MsPrecisionUTC(row.CreatedAt)
	row.UpdatedAt = sqlite.MsPrecisionUTC(row.UpdatedAt)
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, &row)
	return err
}

// ClaimDue atomically claims the oldest due run for owner via UPDATE … RETURNING.
// SQLite serializes writers (DSN _txlock=immediate), so concurrent claims queue
// rather than race; the locked_until guard sends each row to at most one worker.
// It stamps locked_by every claim (the fencing precondition) and bumps reclaims
// ONLY when reclaiming a previously-leased row (old locked_until non-NULL) —
// never attempts. All time comparisons use julianday() (double precision) to
// dodge strftime('%f') round-half-up skew.
func (r *Repository) ClaimDue(
	ctx context.Context,
	owner string,
	lease time.Duration,
) (*run.Run, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("run: ClaimDue requires lease > 0 (got %s)", lease)
	}
	const q = `
		UPDATE runs
		SET locked_by = ?,
		    locked_until = ` + leaseExpr + `,
		    reclaims = reclaims + (locked_until IS NOT NULL),
		    updated_at = ` + nowExpr + `
		WHERE id = (
		    SELECT id FROM runs
		    WHERE completed_at IS NULL
		      AND failed_at IS NULL
		      AND cancelled_at IS NULL
		      AND julianday(run_at) <= julianday('now')
		      AND (locked_until IS NULL OR julianday(locked_until) < julianday('now'))
		    ORDER BY julianday(run_at)
		    LIMIT 1
		)
		RETURNING *`
	var rn run.Run
	err := r.Conn(ctx).GetContext(ctx, &rn, q, owner, lease.Seconds())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rn, nil
}

// RenewLease extends the lease iff still owned and not terminal — the heartbeat.
// Owner-only (it does NOT check locked_until), so the original owner can rescue
// an expired-but-not-yet-reclaimed lease; once another worker reclaims (locked_by
// flips), this matches zero rows and returns false. Fence is the token, not the clock.
func (r *Repository) RenewLease(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
) (bool, error) {
	if lease <= 0 {
		return false, fmt.Errorf("run: RenewLease requires lease > 0 (got %s)", lease)
	}
	const q = `
		UPDATE runs
		SET locked_until = ` + leaseExpr + `,
		    updated_at = ` + nowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	res, err := r.Conn(ctx).ExecContext(ctx, q, lease.Seconds(), id, owner)
	return rowsAffectedBool(res, err)
}

// Checkpoint persists state AND renews the lease iff still owned and not terminal.
// state is bound verbatim as a BLOB (nil → NULL). A lost lease writes nothing.
func (r *Repository) Checkpoint(
	ctx context.Context,
	id, owner string,
	state []byte,
	lease time.Duration,
) (bool, error) {
	if lease <= 0 {
		return false, fmt.Errorf("run: Checkpoint requires lease > 0 (got %s)", lease)
	}
	const q = `
		UPDATE runs
		SET state = ?,
		    locked_until = ` + leaseExpr + `,
		    updated_at = ` + nowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	res, err := r.Conn(ctx).ExecContext(ctx, q, state, lease.Seconds(), id, owner)
	return rowsAffectedBool(res, err)
}

// MarkComplete records terminal success and clears the lock, iff still owned and
// not already terminal (the completed/failed guard makes a double-finalize a no-op).
func (r *Repository) MarkComplete(ctx context.Context, id, owner string) (bool, error) {
	const q = `
		UPDATE runs
		SET completed_at = ` + nowExpr + `,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + nowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	res, err := r.Conn(ctx).ExecContext(ctx, q, id, owner)
	return rowsAffectedBool(res, err)
}

// Reschedule requeues a retryable failure: sets run_at + last_error, bumps
// attempts (the logic-retry counter), and clears the lock — iff still owned and
// not terminal. runAt is normalized to ms precision.
func (r *Repository) Reschedule(
	ctx context.Context,
	id, owner string,
	runAt time.Time,
	lastErr string,
) (bool, error) {
	const q = `
		UPDATE runs
		SET run_at = ?,
		    last_error = ?,
		    attempts = attempts + 1,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + nowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	res, err := r.Conn(ctx).ExecContext(ctx, q, sqlite.MsPrecisionUTC(runAt), lastErr, id, owner)
	return rowsAffectedBool(res, err)
}

// MarkFailed records terminal failure and clears the lock, iff still owned and
// not already terminal. The last checkpoint state is preserved (not cleared) for
// postmortem.
func (r *Repository) MarkFailed(
	ctx context.Context,
	id, owner string,
	lastErr string,
) (bool, error) {
	const q = `
		UPDATE runs
		SET failed_at = ` + nowExpr + `,
		    last_error = ?,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + nowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	res, err := r.Conn(ctx).ExecContext(ctx, q, lastErr, id, owner)
	return rowsAffectedBool(res, err)
}

// RequestCancel sets the operator cancel signal on a non-terminal run. NOT
// owner-checked (the operator is not the worker) and idempotent — a no-op on a
// terminal/missing run. cancel_requested rides on the row, so it survives a reclaim.
func (r *Repository) RequestCancel(ctx context.Context, id string) error {
	const q = `
		UPDATE runs
		SET cancel_requested = 1,
		    updated_at = ` + nowExpr + `
		WHERE id = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	_, err := r.Conn(ctx).ExecContext(ctx, q, id)
	return err
}

// MarkCancelled records terminal cancellation and clears the lock, iff still owned
// and not already terminal — the worker's response to the cancel signal. The last
// checkpoint state is preserved.
func (r *Repository) MarkCancelled(ctx context.Context, id, owner string) (bool, error) {
	const q = `
		UPDATE runs
		SET cancelled_at = ` + nowExpr + `,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + nowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND completed_at IS NULL AND failed_at IS NULL AND cancelled_at IS NULL`
	res, err := r.Conn(ctx).ExecContext(ctx, q, id, owner)
	return rowsAffectedBool(res, err)
}

func (r *Repository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	var rn run.Run
	err := r.Conn(ctx).GetContext(ctx, &rn, `SELECT * FROM runs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &rn, err
}

// IsCancelRequested reads only the cancel_requested flag — the heartbeat's cheap
// cancel poll, avoiding a full-row FindByID (and its payload/state BLOBs) on every
// tick. Absent run → (false, nil).
func (r *Repository) IsCancelRequested(ctx context.Context, id string) (bool, error) {
	var cancel bool
	err := r.Conn(ctx).
		GetContext(ctx, &cancel, `SELECT cancel_requested FROM runs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return cancel, err
}

// rowsAffectedBool turns an owner-checked UPDATE result into the fencing bool:
// (true, nil) iff exactly one row was affected, (false, nil) when zero rows
// matched (ownership lost / terminal — NOT an error), (false, err) on a real
// write failure. This is the contract the whole fence rests on — the job repo
// finalizers ignore sql.Result, which would silently return success on a zero-row
// stale write; runs must not.
func rowsAffectedBool(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
