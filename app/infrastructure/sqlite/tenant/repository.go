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

// FindAllWithUserCount is the platform-plane aggregate: each tenant plus its
// user count via a single GROUP BY tenant_id. The LEFT JOIN touches the
// tenant-owned users table cross-tenant, so the query carries the platform
// exempt marker (tenants itself is control-plane / exempt).
func (r *Repository) FindAllWithUserCount(ctx context.Context) ([]tenant.Overview, error) {
	var rows []tenant.Overview
	err := r.Conn(ctx).SelectContext(ctx, &rows,
		`SELECT t.id, t.name, t.plan, COUNT(u.id) AS user_count
		   FROM tenants t
		   LEFT JOIN users u ON u.tenant_id = t.id /* tenant-scope-exempt: platform superadmin */
		  GROUP BY t.id, t.name, t.plan
		  ORDER BY t.name`)
	return rows, err
}
