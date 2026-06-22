package tenant

import (
	"context"
	"database/sql"
	"errors"

	"gokick/app/domain/tenant"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/sqlite"
)

type Repository struct {
	sqlite.BaseRepository
}

func NewRepository(db *database.SqliteManager) *Repository {
	return &Repository{BaseRepository: sqlite.BaseRepository{DB: db}}
}

func (r *Repository) Save(ctx context.Context, t *tenant.Tenant) error {
	const q = `INSERT INTO tenants (id, name, plan, created_at, updated_at)
		VALUES (:id, :name, :plan, :created_at, :updated_at)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, t)
	return err
}

func (r *Repository) FindByID(ctx context.Context, id string) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := r.Conn(ctx).GetContext(ctx, &t, `SELECT * FROM tenants WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}
