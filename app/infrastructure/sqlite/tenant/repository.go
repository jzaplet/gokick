package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

// FindByName returns the first tenant matching name (or nil). tenants is
// control-plane / exempt, so no tenant_id scoping applies.
func (r *Repository) FindByName(ctx context.Context, name string) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := r.Conn(ctx).GetContext(ctx, &t, `SELECT * FROM tenants WHERE name=? LIMIT 1`, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

// CountAcrossTenants returns the total number of tenants (platform dashboard card).
// Cross-tenant escape hatch — the *AcrossTenants name keeps the isolation gate on it.
func (r *Repository) CountAcrossTenants(ctx context.Context) (int, error) {
	var n int
	err := r.Conn(ctx).GetContext(ctx, &n, `SELECT COUNT(*) FROM tenants`)
	return n, err
}

// OverviewAcrossTenants is the platform-plane aggregate: each tenant plus its
// user count via a single GROUP BY tenant_id. The LEFT JOIN touches the
// tenant-owned users table cross-tenant, so the query carries the platform
// exempt marker (tenants itself is control-plane / exempt).
func (r *Repository) OverviewAcrossTenants(ctx context.Context) ([]tenant.Overview, error) {
	var rows []tenant.Overview
	err := r.Conn(ctx).SelectContext(ctx, &rows,
		`SELECT t.id, t.name, t.plan, COUNT(u.id) AS user_count
		   FROM tenants t
		   LEFT JOIN users u ON u.tenant_id = t.id /* tenant-scope-exempt: platform superadmin */
		  GROUP BY t.id, t.name, t.plan
		  ORDER BY t.name`)
	return rows, err
}

var overviewSortSQL = map[tenant.SortColumn]string{
	tenant.SortByName:  "t.name",
	tenant.SortByUsers: "user_count",
}

// OverviewPage is the platform tenants grid read — OverviewAcrossTenants'
// aggregate with paging, a name filter and a whitelisted sort. The COUNT runs
// over tenants alone (the aggregate join would distort it); the page query
// keeps the LEFT JOIN for the user_count column.
func (r *Repository) OverviewPage(
	ctx context.Context,
	c tenant.ListCriteria,
) (tenant.ListPage, error) {
	conds := []string{}
	args := []any{}
	if c.Filters.Name != "" {
		conds = append(conds, "t.name LIKE ?")
		args = append(args, "%"+c.Filters.Name+"%")
	}
	if c.Filters.Plan != "" {
		conds = append(conds, "t.plan = ?")
		args = append(args, c.Filters.Plan)
	}
	where := ""
	if len(conds) > 0 {
		where = ` WHERE ` + strings.Join(conds, " AND ")
	}

	page := tenant.ListPage{Items: []tenant.Overview{}}
	if err := r.Conn(ctx).GetContext(ctx, &page.Total,
		`SELECT COUNT(*) FROM tenants t`+where, args...); err != nil {
		return tenant.ListPage{}, err
	}

	col, ok := overviewSortSQL[c.Sort]
	if !ok {
		col = "t.name"
	}
	orderBy := fmt.Sprintf(` ORDER BY %s %s, t.name ASC`, col, c.SortDir)
	err := r.Conn(ctx).SelectContext(ctx, &page.Items,
		`SELECT t.id, t.name, t.plan, COUNT(u.id) AS user_count
		   FROM tenants t
		   LEFT JOIN users u ON u.tenant_id = t.id /* tenant-scope-exempt: platform superadmin */`+
			where+` GROUP BY t.id, t.name, t.plan`+orderBy+` LIMIT ? OFFSET ?`,
		append(args, c.PerPage, c.Offset())...)
	return page, err
}
