package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, u *user.User) error {
	const q = `INSERT INTO users (id, nickname, password_hash, email, role, active, created_at, updated_at)
		VALUES (:id, :nickname, :password_hash, :email, :role, :active, :created_at, :updated_at)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, u)
	return err
}

func (r *Repository) Update(ctx context.Context, u *user.User) error {
	const q = `UPDATE users SET nickname=:nickname, password_hash=:password_hash, email=:email,
		role=:role, active=:active, updated_at=:updated_at WHERE id=:id`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, u)
	return err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.Conn(ctx).ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}

func (r *Repository) FindByID(ctx context.Context, id string) (*user.User, error) {
	var u user.User
	err := r.Conn(ctx).GetContext(ctx, &u, `SELECT * FROM users WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &shared.ValidationError{Field: "id", Message: "user not found"}
	}
	return &u, err
}

func (r *Repository) FindByNickname(ctx context.Context, nickname string) (*user.User, error) {
	var u user.User
	err := r.Conn(ctx).GetContext(ctx, &u, `SELECT * FROM users WHERE nickname=?`, nickname)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) FindAllActive(ctx context.Context) ([]user.User, error) {
	var users []user.User
	err := r.Conn(ctx).
		SelectContext(ctx, &users, `SELECT * FROM users WHERE active=1 ORDER BY nickname`)
	return users, err
}

func (r *Repository) FindAll(ctx context.Context) ([]user.User, error) {
	var users []user.User
	err := r.Conn(ctx).SelectContext(ctx, &users, `SELECT * FROM users ORDER BY nickname`)
	return users, err
}

// RecordFailedLogin runs ENTIRELY in SQL so the counter decision (reset
// after the window, increment otherwise, lock when threshold reached) is
// atomic relative to other concurrent failed logins for the same row.
// Uses r.DB.DB() (raw pool) instead of r.Conn(ctx) (tx-aware) on purpose
// — the surrounding LoginHandler returns AuthError on bad credentials,
// which rolls back its bus transaction; the counter update must survive
// that rollback or brute-force protection becomes a no-op.
func (r *Repository) RecordFailedLogin(
	ctx context.Context,
	userID string,
	threshold int,
	window, lockDuration time.Duration,
) (*time.Time, error) {
	windowSec := int(window.Seconds())
	lockExpr := fmt.Sprintf("+%d seconds", int(lockDuration.Seconds()))

	// CASE branches read pre-update column values; `failed_login_attempts + 1`
	// is the post-increment count. Logic:
	//   - last attempt older than window  → reset to 1
	//   - else, increment by 1
	//   - if the resulting count hits the threshold → reset to 0 (so the
	//     post-unlock cycle starts fresh) AND set locked_until
	const q = `
		UPDATE users SET
		    failed_login_attempts = CASE
		        WHEN last_failed_login_at IS NULL
		             OR (julianday('now') - julianday(last_failed_login_at)) * 86400 > ?
		        THEN 1
		        WHEN failed_login_attempts + 1 >= ?
		        THEN 0
		        ELSE failed_login_attempts + 1
		    END,
		    last_failed_login_at = strftime('%Y-%m-%d %H:%M:%f', 'now'),
		    locked_until = CASE
		        WHEN last_failed_login_at IS NOT NULL
		             AND (julianday('now') - julianday(last_failed_login_at)) * 86400 <= ?
		             AND failed_login_attempts + 1 >= ?
		        THEN strftime('%Y-%m-%d %H:%M:%f', 'now', ?)
		        ELSE locked_until
		    END
		WHERE id = ?
		RETURNING locked_until`

	var locked sql.NullTime
	err := r.DB.DB().GetContext(ctx, &locked, q,
		windowSec, threshold, windowSec, threshold, lockExpr, userID)
	if err != nil {
		return nil, err
	}
	if !locked.Valid {
		return nil, nil
	}
	return &locked.Time, nil
}

// ResetFailedLogin runs outside the caller's tx for the same reason as
// RecordFailedLogin — a successful login should clear the counter even
// if a later step in the same handler hits an error and rolls back.
func (r *Repository) ResetFailedLogin(ctx context.Context, userID string) error {
	_, err := r.DB.DB().ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = ?`,
		userID)
	return err
}
