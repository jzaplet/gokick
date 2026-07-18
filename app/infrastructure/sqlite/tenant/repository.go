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

// emptyTenantCond is the "owns nothing live" test, correlated against the tenants
// row the outer statement is deleting. It rides INSIDE the DELETE on purpose:
// as a separate SELECT it would be a check-then-act, and the grid's user_count
// is already stale by the time the superadmin clicks. As one statement, a user
// inserted concurrently either loses the race or saves its tenant — never both.
// The subqueries read across tenants, hence the markers. (The gate accepts any
// query containing "tenant_id", which these do as a JOIN key rather than a scope —
// the marker is the honest classification, not a formality.)
//
// TWO things can own a tenant, not one. users is the obvious half and has an FK to
// back it up. runs is the half that bites: runs.tenant_id carries NO foreign key
// (see the init migration), so nothing under this statement would refuse a widowed
// run — the DB would take the delete and the worker would later restore a dead
// tenant id into the handler ctx, where every scoped read matches zero rows WITHOUT
// erroring. A run that resumes under a deleted tenant can complete "successfully"
// against an empty world, so this is silent-wrong-answer territory, not a crash.
//
// The rule funnels operators straight at it: a tenant with users is refused, so
// emptying it is the required first step — and that is exactly what strands its
// runs. Only NON-TERMINAL runs count; a completed/failed/cancelled row is history,
// never claimed again, and must not pin a tenant forever. That "still live"
// definition is sqlite.NotTerminalClause rather than a tenth hand-rolled copy —
// the constant exists precisely so this rule cannot drift between its call sites,
// and a delete gate disagreeing with the claim query about what "finished" means
// is the drift it was built to stop. It needs the runs table unaliased (bare
// column names), which also keeps the correlation to tenants.id explicit.
const emptyTenantCond = ` AND NOT EXISTS (
	SELECT 1 FROM users u /* tenant-scope-exempt: platform superadmin */
	 WHERE u.tenant_id = tenants.id
) AND NOT EXISTS (
	SELECT 1 FROM runs /* tenant-scope-exempt: platform superadmin */
	 WHERE runs.tenant_id = tenants.id
	   AND ` + sqlite.NotTerminalClause + `
)`

// DeleteIfEmptyAcrossTenants deletes the tenant iff it owns nothing live (no
// users, no non-terminal runs) and is not the default tenant, reporting whether it
// did. A false return means one of those THREE refused it — the caller has already
// established the tenant exists, so there is no fourth outcome, but there IS more
// than one, and a caller that wants to name the reason has to ask (see
// DeleteTenantHandler).
//
// Every floor lives in the statement, matching the bulk twin below (and the user
// repo's `role != 'superadmin'` twins): the emptiness test rides inside the DELETE
// so it cannot be a check-then-act, and the default tenant is excluded by identity
// for the same reason its bulk sibling excludes it — a protected row is the
// statement's business, not the caller's to remember. DeleteTenantHandler refuses
// the default tenant first and owns the honest 400; this is the floor under it, so
// a second caller (a cleanup job, a CLI delete-tenant) cannot inherit the emptiness
// rule for free and silently lose the others.
func (r *Repository) DeleteIfEmptyAcrossTenants(ctx context.Context, id string) (bool, error) {
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM tenants WHERE id=? AND id != ?`+emptyTenantCond,
		id, shared.DefaultTenantID)
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

// BulkDeleteEmptyAcrossTenants deletes every selected tenant that owns nothing
// live and returns the ids that actually went. Tenants still owning something are
// skipped rather than refused — the same emptyTenantCond the single-row delete
// uses, so one row's failure cannot decide the fate of the rest.
//
// It returns the IDS rather than a count because the count cannot be turned back
// into them afterwards: the rows are gone, and in all-filtered mode nobody
// enumerated a selection to compare against. A "5 selected, 2 deleted" audit record
// that cannot say WHICH 2 is not a record of an irreversible cross-tenant delete.
// DELETE ... RETURNING makes the deleted set fall out of the same statement, so it
// stays exactly what the DELETE decided — no second query to disagree with it.
func (r *Repository) BulkDeleteEmptyAcrossTenants(
	ctx context.Context,
	sel tenant.BulkSelection,
) ([]string, error) {
	if sel.IsEmpty() {
		return nil, nil
	}
	where, args := bulkWhere(sel)
	var ids []string
	err := r.Conn(ctx).SelectContext(ctx, &ids,
		`DELETE FROM tenants WHERE id != ?`+where+emptyTenantCond+` RETURNING id`,
		append([]any{shared.DefaultTenantID}, args...)...)
	if err != nil {
		return nil, err
	}

	return ids, nil
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
