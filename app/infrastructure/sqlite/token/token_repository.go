package token

import (
	"context"
	"database/sql"
	"errors"

	"myapp/app/domain/token"
	"myapp/app/infrastructure/database"
	"myapp/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, t *token.RefreshToken) error {
	const q = `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES (:id, :user_id, :token_hash, :expires_at, :created_at)`
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

func (r *Repository) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.Conn(ctx).ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id=?`, userID)
	return err
}

func (r *Repository) DeleteExpired(ctx context.Context) error {
	_, err := r.Conn(ctx).
		ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < datetime('now')`)
	return err
}
