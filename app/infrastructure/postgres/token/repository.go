// Package token implements token.Repository on Postgres — the twin of the SQLite
// repository. Login and refresh, which issue, look up and rotate tokens before
// any tenant is known, run on the system plane (shared.PreTenant); logout revokes
// the caller's own tokens inside its tenant transaction, where row-level security
// shows it exactly the tokens of its tenant's users.
package token

import (
	"context"
	"database/sql"
	"errors"

	"gokick/app/domain/token"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
)

type Repository struct {
	postgres.BaseRepository
}

func NewRepository(db *postgres.Manager) *Repository {
	return &Repository{BaseRepository: postgres.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, t *token.RefreshToken) error {
	const q = `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, used_at)
		VALUES (:id, :user_id, :token_hash, :expires_at, :created_at, :used_at)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, t)
	return err
}

func (r *Repository) FindByHash(ctx context.Context, hash string) (*token.RefreshToken, error) {
	var t token.RefreshToken
	err := r.Conn(ctx).
		GetContext(ctx, &t, `SELECT * FROM refresh_tokens WHERE token_hash = $1`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MarkUsed flips used_at atomically — the rotation CAS. Of two concurrent
// requests carrying the same token, the second waits for the first's row lock,
// re-reads the row, finds used_at set and affects no row: false, which the
// handler turns into theft detection.
func (r *Repository) MarkUsed(ctx context.Context, hash string) (bool, error) {
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE refresh_tokens SET used_at = `+postgres.NowExpr+`
		  WHERE token_hash = $1 AND used_at IS NULL`, hash)
	return database.RowsAffectedBool(res, err)
}

func (r *Repository) DeleteByUserID(ctx context.Context, userID string) error {
	id, ok := postgres.ParseID(userID)
	if !ok {
		return nil
	}
	_, err := r.Conn(ctx).ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, id)
	return err
}

// DeleteExpired is the scheduler's sweep over every tenant's tokens — system
// plumbing, so it runs on the system role whatever ctx says (the scheduler sets
// no plane).
func (r *Repository) DeleteExpired(ctx context.Context) error {
	_, err := r.SystemConn(ctx).ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < `+postgres.NowExpr)
	return err
}
