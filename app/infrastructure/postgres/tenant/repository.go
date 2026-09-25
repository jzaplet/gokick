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
// See the SQLite twin for why both users and non-terminal runs count; a finished
// run does not, and goes with its tenant (runs.tenant_id cascades). On its own it
// loses a race to a concurrent insert — see BulkDeleteEmptyAcrossTenants.
const emptyTenantCond = ` AND NOT EXISTS (
	SELECT 1 FROM users u /* tenant-scope-exempt: platform superadmin */
	 WHERE u.tenant_id = tenants.id
) AND NOT EXISTS (
	SELECT 1 FROM runs /* tenant-scope-exempt: platform superadmin */
	 WHERE runs.tenant_id = tenants.id
	   AND ` + database.NotTerminalClause + `
)`

// DeleteIfEmptyAcrossTenants deletes the tenant iff it owns nothing live and is
// not the default tenant, reporting whether it did — see the SQLite twin. It is
// the bulk delete of one id (a malformed id matches no row).
func (r *Repository) DeleteIfEmptyAcrossTenants(ctx context.Context, id string) (bool, error) {
	ids, err := r.BulkDeleteEmptyAcrossTenants(ctx, tenant.BulkSelection{IDs: []string{id}})
	return len(ids) > 0, err
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
//
// emptyTenantCond alone would lose a race SQLite cannot have: a user or run
// inserted concurrently is invisible to the DELETE's snapshot, so the DELETE would
// fail the users foreign key (a 500) or cascade the new run away with its tenant.
// The delete therefore first locks the selected tenants (never the default one)
// in id order — two overlapping deletes queue up instead of deadlocking — and
// deletes in a second statement of the same transaction. An insert takes a
// key-share lock on its tenant's row for the foreign-key check, so the lock waits
// for an insert in flight to commit, and the DELETE — a new statement with a fresh
// snapshot — sees what it inserted; an insert that comes after the lock waits
// until the tenant is gone and fails its own foreign-key check, as it would
// against a tenant deleted long before.
func (r *Repository) BulkDeleteEmptyAcrossTenants(
	ctx context.Context,
	sel tenant.BulkSelection,
) ([]string, error) {
	if sel.IsEmpty() {
		return nil, nil
	}
	var ids []string
	err := r.SystemTx(ctx, func(conn postgres.Conn) error {
		a := postgres.Args{shared.DefaultTenantID}
		where := bulkWhere(sel, &a)
		var locked []string
		err := conn.SelectContext(ctx, &locked,
			`SELECT id FROM tenants WHERE id <> $1`+where+postgres.LockInIDOrder, a...)
		if err != nil || len(locked) == 0 {
			return err
		}
		return conn.SelectContext(ctx, &ids,
			`DELETE FROM tenants WHERE id = ANY($1)`+emptyTenantCond+` RETURNING id`, locked)
	})
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
