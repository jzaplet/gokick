// Package tenant implements tenant.Repository and tenant.PlatformRepository on
// Postgres — the twin of the SQLite repository.
package tenant

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/tenant"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
)

type Repository struct {
	postgres.BaseRepository
}

func NewRepository(db *postgres.Manager) *Repository {
	return &Repository{BaseRepository: postgres.BaseRepository{DB: db}}
}

// nameKey is the unique index on tenants.name.
const nameKey = "idx_tenants_name"

// Save inserts the tenant. A name another tenant already holds comes back as the
// error CreateTenantHandler's own pre-check returns: the check is for the
// operator's message, the index for correctness — two concurrent creates both
// pass the check, and the loser must see the 400, not a 500.
func (r *Repository) Save(ctx context.Context, t *tenant.Tenant) error {
	const q = `INSERT INTO tenants (id, name, plan, created_at, updated_at)
		VALUES (:id, :name, :plan, :created_at, :updated_at)`
	_, err := r.Conn(ctx).NamedExecContext(ctx, q, t)
	if postgres.IsUniqueViolation(err, nameKey) {
		return &shared.ValidationError{Field: "name", Key: msgkey.TenantNameTaken}
	}
	return err
}

func (r *Repository) FindByID(ctx context.Context, id string) (*tenant.Tenant, error) {
	tid, ok := postgres.ParseID(id)
	if !ok {
		return nil, nil
	}
	return r.getOne(ctx, `SELECT * FROM tenants WHERE id = $1`, tid)
}

// FindByName returns the tenant with that name (or nil). tenants is control-plane
// / exempt, so no tenant_id scoping applies.
func (r *Repository) FindByName(ctx context.Context, name string) (*tenant.Tenant, error) {
	return r.getOne(ctx, `SELECT * FROM tenants WHERE name = $1`, name)
}

func (r *Repository) getOne(ctx context.Context, q string, args ...any) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := r.Conn(ctx).GetContext(ctx, &t, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CountAcrossTenants returns the total number of tenants (platform dashboard card).
func (r *Repository) CountAcrossTenants(ctx context.Context) (int, error) {
	var n int
	err := r.SystemConn(ctx).GetContext(ctx, &n, `SELECT COUNT(*) FROM tenants`)
	return n, err
}

// emptyTenantCond is the "owns nothing live" test, correlated against the tenants
// row the outer statement deletes — inside the DELETE, never a check-then-act.
// See the SQLite twin for why both users and non-terminal runs count. Here the
// foreign keys back it up: a user or run inserted concurrently, which this
// statement's snapshot cannot see, makes the DELETE fail its foreign-key check
// instead of stranding the row (users) — and a finished run, which does not count,
// goes with its tenant (runs.tenant_id cascades).
const emptyTenantCond = ` AND NOT EXISTS (
	SELECT 1 FROM users u /* tenant-scope-exempt: platform superadmin */
	 WHERE u.tenant_id = tenants.id
) AND NOT EXISTS (
	SELECT 1 FROM runs /* tenant-scope-exempt: platform superadmin */
	 WHERE runs.tenant_id = tenants.id
	   AND ` + database.NotTerminalClause + `
)`

// DeleteIfEmptyAcrossTenants deletes the tenant iff it owns nothing live and is
// not the default tenant, reporting whether it did — see the SQLite twin. A
// foreign-key violation is a user or run that arrived while the statement ran: the
// tenant is not empty after all, the same refusal as the condition's.
func (r *Repository) DeleteIfEmptyAcrossTenants(ctx context.Context, id string) (bool, error) {
	tid, ok := postgres.ParseID(id)
	if !ok {
		return false, nil
	}
	res, err := r.SystemConn(ctx).ExecContext(ctx,
		`DELETE FROM tenants WHERE id = $1 AND id <> $2`+emptyTenantCond,
		tid, shared.DefaultTenantID)
	if postgres.IsForeignKeyViolation(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// overviewFilterConds renders the tenants grid's name/plan filters as loose
// conditions — single-sourced so "delete all filtered" hits exactly the set the
// grid showed. prefix is the column qualifier ("t." in the paged read, "" in a
// DELETE, which cannot alias).
func overviewFilterConds(f tenant.ListFilters, prefix string, a *postgres.Args) []string {
	conds := []string{}
	if f.Name != "" {
		conds = append(conds, postgres.ILikeContains(prefix+"name", a, f.Name))
	}
	if f.Plan != "" {
		conds = append(conds, prefix+"plan = "+a.Add(f.Plan))
	}
	return conds
}

// bulkWhere renders a tenant BulkSelection: the grid's filters (AllFiltered) or an
// explicit id list, where a malformed id matches no row.
func bulkWhere(sel tenant.BulkSelection, a *postgres.Args) string {
	if sel.AllFiltered {
		where := ""
		for _, c := range overviewFilterConds(sel.Filters, "", a) {
			where += " AND " + c
		}
		return where
	}
	return ` AND id = ANY(` + a.Add(postgres.ParseIDs(sel.IDs)) + `)`
}

// BulkDeleteEmptyAcrossTenants deletes every selected tenant that owns nothing
// live and returns the ids that actually went (DELETE … RETURNING) — see the
// SQLite twin.
func (r *Repository) BulkDeleteEmptyAcrossTenants(
	ctx context.Context,
	sel tenant.BulkSelection,
) ([]string, error) {
	if sel.IsEmpty() {
		return nil, nil
	}
	a := postgres.Args{shared.DefaultTenantID}
	var ids []string
	err := r.SystemConn(ctx).SelectContext(ctx, &ids,
		`DELETE FROM tenants WHERE id <> $1`+bulkWhere(sel, &a)+emptyTenantCond+` RETURNING id`,
		a...)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// overviewSortSQL: the name sorts in the Czech collation, the user count
// numerically.
var overviewSortSQL = map[tenant.SortColumn]string{
	tenant.SortByName:  "t.name" + database.CollateSort,
	tenant.SortByUsers: "user_count",
}

// OverviewPageAcrossTenants is the platform tenants grid read — each tenant plus
// its user count, paged, filtered and sorted. The COUNT runs over tenants alone;
// the page query keeps the LEFT JOIN for the user_count column.
func (r *Repository) OverviewPageAcrossTenants(
	ctx context.Context,
	c tenant.ListCriteria,
) (tenant.ListPage, error) {
	var a postgres.Args
	where := ""
	if conds := overviewFilterConds(c.Filters, "t.", &a); len(conds) > 0 {
		where = ` WHERE ` + strings.Join(conds, " AND ")
	}

	page := tenant.ListPage{Items: []tenant.Overview{}}
	if err := r.SystemConn(ctx).GetContext(ctx, &page.Total,
		`SELECT COUNT(*) FROM tenants t`+where, a...); err != nil {
		return tenant.ListPage{}, err
	}

	col, ok := overviewSortSQL[c.Sort]
	if !ok {
		col = "t.name" + database.CollateSort
	}
	orderBy := ` ORDER BY ` + col + ` ` + string(
		c.SortDir,
	) + `, t.name` + database.CollateSort + ` ASC`
	err := r.SystemConn(ctx).SelectContext(ctx, &page.Items,
		`SELECT t.id, t.name, t.plan, COUNT(u.id) AS user_count
		   FROM tenants t
		   LEFT JOIN users u ON u.tenant_id = t.id /* tenant-scope-exempt: platform superadmin */`+
			where+` GROUP BY t.id, t.name, t.plan`+orderBy+
			` LIMIT `+a.Add(c.PerPage)+` OFFSET `+a.Add(c.Offset()),
		a...)
	return page, err
}
