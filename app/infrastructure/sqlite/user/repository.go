package user

import (
	"context"
	"database/sql"
	"errors"

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
