package token

import (
	"context"
	"database/sql"
	"errors"

	"gokick/app/domain/token"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, t *token.RefreshToken) error {
	const q = `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, used_at)
		VALUES (:id, :user_id, :token_hash, :expires_at, :created_at, :used_at)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, t)
	return err
}

func (r *Repository) FindByHash(ctx context.Context, hash string) (*token.RefreshToken, error) {
	var t token.RefreshToken
	err := r.Conn(ctx).GetContext(ctx, &t, `SELECT * FROM refresh_tokens WHERE token_hash=?`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

// MarkUsed flips used_at atomically. The AND used_at IS NULL guard plus the
// RowsAffected check make double-rotation observable even under concurrent
// requests carrying the same raw token: the second request sees 0 rows
// affected and returns false, letting the handler trigger theft detection.
func (r *Repository) MarkUsed(ctx context.Context, hash string) (bool, error) {
	// Reuse the shared exactly-one-row-affected → bool contract (same helper the
	// run finalizers use) so this CAS can't drift into a silent zero-row success.
	// Bind res/err first (not RowsAffectedBool(Exec(...)) inline) — matching the run
	// repo's call style AND dodging a go-arch-lint deepScan panic on the f(g()) form.
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE refresh_tokens SET used_at=datetime('now')
		 WHERE token_hash=? AND used_at IS NULL`, hash)
	return sqlite.RowsAffectedBool(res, err)
}

func (r *Repository) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.Conn(ctx).ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id=?`, userID)
	return err
}

func (r *Repository) DeleteExpired(ctx context.Context) error {
	// datetime() wraps expires_at because Go time.Time serializes with a
	// timezone offset that doesn't lex-compare to SQLite's UTC datetime('now').
	_, err := r.Conn(ctx).
		ExecContext(ctx, `DELETE FROM refresh_tokens WHERE datetime(expires_at) < datetime('now')`)
	return err
}
