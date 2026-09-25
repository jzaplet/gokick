// Package run implements run.Repository on Postgres — the twin of the SQLite
// repository, with the same owner-token fencing: every worker write past the
// claim is owner-checked and reports whether it affected its one row.
//
// Two things differ, because Postgres runs writers in parallel where SQLite
// serializes them:
//   - ClaimDue locks its candidate with FOR UPDATE SKIP LOCKED, so concurrent
//     workers each take a different run and never wait on one another;
//   - the worker's bookkeeping (claim, heartbeat, checkpoint, finalizers) is
//     cross-tenant plumbing and runs on the system role (SystemConn) whatever ctx
//     says — the handler itself runs in its run's tenant.
//
// The database clock is statement_timestamp() (postgres.NowExpr): a lease must
// move with the statement, not stand still at the start of a transaction.
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
	"gokick/app/infrastructure/postgres"
)

type Repository struct {
	postgres.BaseRepository
}

func NewRepository(db *postgres.Manager) *Repository {
	return &Repository{BaseRepository: postgres.BaseRepository{DB: db}}
}

// Enqueue inserts the run in its tenant — resolved fail-closed as on SQLite (an
// empty tenant is the default one in single-tenant mode, an error under
// multitenancy). It runs where ctx says (Conn): inside the enqueuing command's
// transaction, so the run is born exactly when the command commits.
func (r *Repository) Enqueue(ctx context.Context, rn *run.Run) error {
	const q = `INSERT INTO runs (id, kind, tenant_id, lang, payload, state, run_at, attempts, reclaims, parks, max_retries, locked_by, locked_until, last_error, failed_at, completed_at, cancel_requested, cancelled_at, created_at, updated_at)
		VALUES (:id, :kind, :tenant_id, :lang, :payload, :state, :run_at, :attempts, :reclaims, :parks, :max_retries, :locked_by, :locked_until, :last_error, :failed_at, :completed_at, :cancel_requested, :cancelled_at, :created_at, :updated_at)`
	row := *rn
	tenantID, err := shared.RequireTenant(row.TenantID, r.Multitenancy())
	if err != nil {
		return err
	}
	row.TenantID = tenantID
	row.RunAt = database.MsPrecisionUTC(row.RunAt)
	row.CreatedAt = database.MsPrecisionUTC(row.CreatedAt)
	row.UpdatedAt = database.MsPrecisionUTC(row.UpdatedAt)
	_, err = r.Conn(ctx).NamedExecContext(ctx, q, &row)
	return err
}

// ClaimDue atomically claims the oldest due run for owner. The candidate is
// locked FOR UPDATE SKIP LOCKED: a run another worker is claiming right now is
// skipped, not waited for, so N workers take N different runs in parallel. The
// outer WHERE repeats the claimability guard against the row as it is once locked
// — a claim that raced ours in between cannot be claimed twice. It stamps
// locked_by every claim (the fencing precondition) and bumps reclaims ONLY when
// reclaiming a previously-leased row — never attempts.
func (r *Repository) ClaimDue(
	ctx context.Context,
	owner string,
	lease time.Duration,
) (*run.Run, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("run: ClaimDue requires lease > 0 (got %s)", lease)
	}
	const claimable = database.NotTerminalClause + `
		  AND run_at <= ` + postgres.NowExpr + `
		  AND (locked_until IS NULL OR locked_until < ` + postgres.NowExpr + `)`
	q := `
		WITH next AS (
		    SELECT id FROM runs
		    WHERE ` + claimable + `
		    ORDER BY run_at
		    LIMIT 1
		    FOR UPDATE SKIP LOCKED
		)
		UPDATE runs
		SET locked_by = $1,
		    locked_until = ` + postgres.NowPlus("$2") + `,
		    reclaims = reclaims + (locked_until IS NOT NULL)::int,
		    updated_at = ` + postgres.NowExpr + `
		FROM next
		WHERE runs.id = next.id AND ` + claimable + `
		RETURNING runs.*`
	var rn run.Run
	err := r.SystemConn(ctx).GetContext(ctx, &rn, q, owner, lease.Seconds())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rn, nil
}

// RenewLease extends the lease iff still owned and not terminal — the heartbeat
// — and returns cancel_requested from the very row it renewed. Owner-only (it
// does not check locked_until): the fence is the token, not the clock. See the
// SQLite twin.
func (r *Repository) RenewLease(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
) (alive, cancelRequested bool, err error) {
	if lease <= 0 {
		return false, false, fmt.Errorf("run: RenewLease requires lease > 0 (got %s)", lease)
	}
	rid, ok := postgres.ParseID(id)
	if !ok {
		return false, false, nil
	}
	q := `
		UPDATE runs
		SET locked_until = ` + postgres.NowPlus("$1") + `,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $2 AND locked_by = $3
		  AND ` + database.NotTerminalClause + `
		RETURNING cancel_requested`
	err = r.SystemConn(ctx).GetContext(ctx, &cancelRequested, q, lease.Seconds(), rid, owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, cancelRequested, nil
}

// Checkpoint persists state AND renews the lease iff still owned and not
// terminal. state is bound verbatim as bytea (nil → NULL). A lost lease writes
// nothing.
func (r *Repository) Checkpoint(
	ctx context.Context,
	id, owner string,
	state []byte,
	lease time.Duration,
) (bool, error) {
	if lease <= 0 {
		return false, fmt.Errorf("run: Checkpoint requires lease > 0 (got %s)", lease)
	}
	q := `
		UPDATE runs
		SET state = $3,
		    locked_until = ` + postgres.NowPlus("$4") + `,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1 AND locked_by = $2
		  AND ` + database.NotTerminalClause
	return r.fenced(ctx, id, owner, q, state, lease.Seconds())
}

// MarkComplete records terminal success and clears the lock, iff still owned and
// not already terminal.
func (r *Repository) MarkComplete(ctx context.Context, id, owner string) (bool, error) {
	const q = `
		UPDATE runs
		SET completed_at = ` + postgres.NowExpr + `,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1 AND locked_by = $2
		  AND ` + database.NotTerminalClause
	return r.fenced(ctx, id, owner, q)
}

// Reschedule requeues a retryable failure: sets run_at + last_error, bumps
// attempts, and clears the lock — iff still owned and not terminal.
func (r *Repository) Reschedule(
	ctx context.Context,
	id, owner string,
	runAt time.Time,
	lastErr string,
) (bool, error) {
	const q = `
		UPDATE runs
		SET run_at = $3,
		    last_error = $4,
		    attempts = attempts + 1,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1 AND locked_by = $2
		  AND ` + database.NotTerminalClause
	return r.fenced(ctx, id, owner, q, database.MsPrecisionUTC(runAt), lastErr)
}

// Park requeues an unknown-kind run exactly like Reschedule but bumps parks, NOT
// attempts.
func (r *Repository) Park(
	ctx context.Context,
	id, owner string,
	runAt time.Time,
	reason string,
) (bool, error) {
	const q = `
		UPDATE runs
		SET run_at = $3,
		    last_error = $4,
		    parks = parks + 1,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1 AND locked_by = $2
		  AND ` + database.NotTerminalClause
	return r.fenced(ctx, id, owner, q, database.MsPrecisionUTC(runAt), reason)
}

// MarkFailed records terminal failure and clears the lock, iff still owned and
// not already terminal. The last checkpoint state is preserved for postmortem.
func (r *Repository) MarkFailed(
	ctx context.Context,
	id, owner string,
	lastErr string,
) (bool, error) {
	const q = `
		UPDATE runs
		SET failed_at = ` + postgres.NowExpr + `,
		    last_error = $3,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1 AND locked_by = $2
		  AND ` + database.NotTerminalClause
	return r.fenced(ctx, id, owner, q, lastErr)
}

// MarkCancelled records terminal cancellation and clears the lock, iff still
// owned and not already terminal.
func (r *Repository) MarkCancelled(ctx context.Context, id, owner string) (bool, error) {
	const q = `
		UPDATE runs
		SET cancelled_at = ` + postgres.NowExpr + `,
		    locked_until = NULL,
		    locked_by = NULL,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1 AND locked_by = $2
		  AND ` + database.NotTerminalClause
	return r.fenced(ctx, id, owner, q)
}

// fenced runs one owner-checked worker write on the system role: every such
// statement matches WHERE id = $1 AND locked_by = $2, and binds its own values
// from $3 on. A malformed id is a run that is not there — no row.
func (r *Repository) fenced(ctx context.Context, id, owner, q string, args ...any) (bool, error) {
	rid, ok := postgres.ParseID(id)
	if !ok {
		return false, nil
	}
	res, err := r.SystemConn(ctx).ExecContext(ctx, q, append([]any{rid, owner}, args...)...)
	return database.RowsAffectedBool(res, err)
}

// RequestCancel sets the operator cancel signal on a non-terminal run. NOT
// owner-checked (the operator is not the worker) and idempotent — a no-op on a
// terminal or missing run. It runs where ctx says: an operator command, in its
// tenant.
func (r *Repository) RequestCancel(ctx context.Context, id string) error {
	rid, ok := postgres.ParseID(id)
	if !ok {
		return nil
	}
	const q = `
		UPDATE runs
		SET cancel_requested = true,
		    updated_at = ` + postgres.NowExpr + `
		WHERE id = $1
		  AND ` + database.NotTerminalClause
	_, err := r.Conn(ctx).ExecContext(ctx, q, rid)
	return err
}

// FindByID returns the run, or (nil, nil) when absent (or the id is malformed).
func (r *Repository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	rid, ok := postgres.ParseID(id)
	if !ok {
		return nil, nil
	}
	var rn run.Run
	err := r.Conn(ctx).GetContext(ctx, &rn, `SELECT * FROM runs WHERE id = $1`, rid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rn, nil
}
