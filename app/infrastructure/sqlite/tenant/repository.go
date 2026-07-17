package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gokick/app/domain/shared"
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

// emptyTenantCond is the "owns no users" test, correlated against the tenants
// row the outer statement is deleting. It rides INSIDE the DELETE on purpose:
// as a separate SELECT it would be a check-then-act, and the grid's user_count
// is already stale by the time the superadmin clicks. As one statement, a user
// inserted concurrently either loses the race or saves its tenant — never both.
// The subquery reads users across tenants, hence the marker. (The gate accepts
// any query containing "tenant_id", which this does as a JOIN key rather than a
// scope — the marker is the honest classification, not a formality.)
const emptyTenantCond = ` AND NOT EXISTS (
	SELECT 1 FROM users u /* tenant-scope-exempt: platform superadmin */
	 WHERE u.tenant_id = tenants.id
)`

// DeleteIfEmptyAcrossTenants deletes the tenant iff it owns no users, reporting
// whether it did. A false return means it still has users — the caller has
// already established the tenant exists, so that is the only other outcome.
func (r *Repository) DeleteIfEmptyAcrossTenants(ctx context.Context, id string) (bool, error) {
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM tenants WHERE id=?`+emptyTenantCond, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()

	return n > 0, err
}

// overviewFilterConds renders the tenants grid's name/plan filters as loose
// conditions, so each caller can join them into its own WHERE. Single-sourced
// deliberately: "delete all filtered" must hit EXACTLY the set the grid showed,
// and two hand-written filter builders would be free to drift apart — the
// superadmin would then delete a set they never saw.
//
// prefix is the column qualifier: the paged read aliases tenants to `t` (it
// joins users), while a DELETE cannot alias and passes "".
func overviewFilterConds(f tenant.ListFilters, prefix string) ([]string, []any) {
	conds := []string{}
	args := []any{}
	if f.Name != "" {
		conds = append(conds, prefix+"name LIKE ?")
		args = append(args, "%"+f.Name+"%")
	}
	if f.Plan != "" {
		conds = append(conds, prefix+"plan = ?")
		args = append(args, f.Plan)
	}

	return conds, args
}

// bulkWhere renders a tenant BulkSelection: either the filters the grid had
// applied (AllFiltered) or an explicit id list. Mirrors the user grids' twin.
func bulkWhere(sel tenant.BulkSelection) (string, []any) {
	if sel.AllFiltered {
		conds, args := overviewFilterConds(sel.Filters, "")
		where := ""
		for _, c := range conds {
			where += " AND " + c
		}

		return where, args
	}
	where := ` AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(sel.IDs)), ",") + `)`
	args := make([]any, 0, len(sel.IDs))
	for _, id := range sel.IDs {
		args = append(args, id)
	}

	return where, args
}

// BulkDeleteEmptyAcrossTenants deletes every selected tenant that owns no users
// and returns how many went. Non-empty tenants are skipped rather than refused —
// the same emptyTenantCond the single-row delete uses, so one row's failure
// cannot decide the fate of the rest.
func (r *Repository) BulkDeleteEmptyAcrossTenants(
	ctx context.Context,
	sel tenant.BulkSelection,
) (int64, error) {
	if sel.IsEmpty() {
		return 0, nil
	}
	where, args := bulkWhere(sel)
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM tenants WHERE id != ?`+where+emptyTenantCond,
		append([]any{shared.DefaultTenantID}, args...)...)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

var overviewSortSQL = map[tenant.SortColumn]string{
	tenant.SortByName:  "t.name",
	tenant.SortByUsers: "user_count",
}

// OverviewPageAcrossTenants is the platform tenants grid read — each tenant plus its user
// count (a GROUP BY tenant_id aggregate) with paging, filters and a whitelisted
// sort. The LEFT JOIN touches the tenant-owned users table cross-tenant, so the
// query carries the platform exempt marker (tenants itself is control-plane /
// exempt). The COUNT runs over tenants alone (the aggregate join would distort
// it); the page query keeps the LEFT JOIN for the user_count column.
func (r *Repository) OverviewPageAcrossTenants(
	ctx context.Context,
	c tenant.ListCriteria,
) (tenant.ListPage, error) {
	conds, args := overviewFilterConds(c.Filters, "t.")
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
