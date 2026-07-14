// Package run implements run.Repository on SQLite. It uses the project's
// julianday/ms-precision time discipline (julianday comparisons, ms-precision writes) and adds the
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

// The DB-clock (sqlite.NowExpr) and lease (sqlite.LeaseExpr) SQL, and the fencing
// rows-affected helper (sqlite.RowsAffectedBool), live in the shared sqlite package
// so the durable-queue time discipline and fence contract cannot drift per-repo.

func (r *Repository) Enqueue(ctx context.Context, rn *run.Run) error {
	const q = `INSERT INTO runs (id, kind, tenant_id, payload, state, run_at, attempts, reclaims, parks, max_retries, locked_by, locked_until, last_error, failed_at, completed_at, cancel_requested, cancelled_at, created_at, updated_at)
		VALUES (:id, :kind, :tenant_id, :payload, :state, :run_at, :attempts, :reclaims, :parks, :max_retries, :locked_by, :locked_until, :last_error, :failed_at, :completed_at, :cancel_requested, :cancelled_at, :created_at, :updated_at)`
	row := *rn
	// Resolve the tenant fail-closed (RequireTenant): the dispatcher stamps it from
	// ctx; an empty tenant (a non-bus direct Enqueue) becomes the default in
	// single-tenant mode but an ERROR in multitenant mode — a run is never silently
	// born in the default tenant. An explicit "" would also override the column's
	// NOT NULL DEFAULT and break scoping.
	tenantID, err := shared.RequireTenant(row.TenantID, r.Multitenancy())
	if err != nil {
		return err
	}
	row.TenantID = tenantID
	row.RunAt = sqlite.MsPrecisionUTC(row.RunAt)
	row.CreatedAt = sqlite.MsPrecisionUTC(row.CreatedAt)
	row.UpdatedAt = sqlite.MsPrecisionUTC(row.UpdatedAt)
	_, err = r.Conn(ctx).NamedExecContext(ctx, q, &row)
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
		    locked_until = ` + sqlite.LeaseExpr + `,
		    reclaims = reclaims + (locked_until IS NOT NULL),
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = (
		    SELECT id FROM runs
		    WHERE ` + sqlite.NotTerminalClause + `
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
// flips), this matches zero rows and returns alive=false. Fence is the token, not
// the clock.
//
// It RETURNS cancel_requested from the very row it just renewed, so the heartbeat
// observes a mid-run operator cancel in this one owner-checked write instead of a
// second IsCancelRequested round-trip — and the RETURNING projects only the flag,
// never the payload/state BLOB. A matched row → (true, flag, nil); zero rows
// (lease lost / terminal) → (false, false, nil).
func (r *Repository) RenewLease(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
) (alive, cancelRequested bool, err error) {
	if lease <= 0 {
		return false, false, fmt.Errorf("run: RenewLease requires lease > 0 (got %s)", lease)
	}
	const q = `
		UPDATE runs
		SET locked_until = ` + sqlite.LeaseExpr + `,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause + `
		RETURNING cancel_requested`
	err = r.Conn(ctx).GetContext(ctx, &cancelRequested, q, lease.Seconds(), id, owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, cancelRequested, nil
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
		    locked_until = ` + sqlite.LeaseExpr + `,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause
	res, err := r.Conn(ctx).ExecContext(ctx, q, state, lease.Seconds(), id, owner)
	return sqlite.RowsAffectedBool(res, err)
}

// MarkComplete records terminal success and clears the lock, iff still owned and
// not already terminal (the completed/failed guard makes a double-finalize a no-op).
func (r *Repository) MarkComplete(ctx context.Context, id, owner string) (bool, error) {
	const q = `
		UPDATE runs
		SET completed_at = ` + sqlite.NowExpr + `,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause
	res, err := r.Conn(ctx).ExecContext(ctx, q, id, owner)
	return sqlite.RowsAffectedBool(res, err)
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
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause
	res, err := r.Conn(ctx).ExecContext(ctx, q, sqlite.MsPrecisionUTC(runAt), lastErr, id, owner)
	return sqlite.RowsAffectedBool(res, err)
}

// Park requeues an unknown-kind run (registry skew) exactly like Reschedule but
// bumps parks, NOT attempts — so deploy-window parking is bounded separately and
// never consumes the handler's logic-retry budget.
func (r *Repository) Park(
	ctx context.Context,
	id, owner string,
	runAt time.Time,
	reason string,
) (bool, error) {
	const q = `
		UPDATE runs
		SET run_at = ?,
		    last_error = ?,
		    parks = parks + 1,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause
	res, err := r.Conn(ctx).ExecContext(ctx, q, sqlite.MsPrecisionUTC(runAt), reason, id, owner)
	return sqlite.RowsAffectedBool(res, err)
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
		SET failed_at = ` + sqlite.NowExpr + `,
		    last_error = ?,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause
	res, err := r.Conn(ctx).ExecContext(ctx, q, lastErr, id, owner)
	return sqlite.RowsAffectedBool(res, err)
}

// RequestCancel sets the operator cancel signal on a non-terminal run. NOT
// owner-checked (the operator is not the worker) and idempotent — a no-op on a
// terminal/missing run. cancel_requested rides on the row, so it survives a reclaim.
func (r *Repository) RequestCancel(ctx context.Context, id string) error {
	const q = `
		UPDATE runs
		SET cancel_requested = 1,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ?
		  AND ` + sqlite.NotTerminalClause
	_, err := r.Conn(ctx).ExecContext(ctx, q, id)
	return err
}

// MarkCancelled records terminal cancellation and clears the lock, iff still owned
// and not already terminal — the worker's response to the cancel signal. The last
// checkpoint state is preserved.
func (r *Repository) MarkCancelled(ctx context.Context, id, owner string) (bool, error) {
	const q = `
		UPDATE runs
		SET cancelled_at = ` + sqlite.NowExpr + `,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + sqlite.NowExpr + `
		WHERE id = ? AND locked_by = ?
		  AND ` + sqlite.NotTerminalClause
	res, err := r.Conn(ctx).ExecContext(ctx, q, id, owner)
	return sqlite.RowsAffectedBool(res, err)
}

// FindByID returns the run, or (nil, nil) when absent. On a real read error it
// returns (nil, err) — never a half-scanned row alongside the error.
func (r *Repository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	var rn run.Run
	err := r.Conn(ctx).GetContext(ctx, &rn, `SELECT * FROM runs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rn, nil
}
