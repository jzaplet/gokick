package user

import (
	"context"
	"time"

	"gokick/app/domain/user"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
)

// The grid reads and bulk writes live in their own file, as on SQLite:
// repository.go holds the single-row CRUD + identity lookups, this file everything
// the DataGrid stack drives.

// listSortSQL maps the whitelisted sort columns onto SQL. The map (not string
// interpolation of the wire value) IS the injection guard. Text columns sort in
// the Czech collation — the same order the SQLite adapter produces.
var listSortSQL = map[user.SortColumn]string{
	user.SortByNickname: "nickname" + database.CollateSort,
	user.SortByEmail:    "email" + database.CollateSort,
	user.SortByRole:     "role" + database.CollateSort,
}

// listFilterWhere renders the optional filter conditions appended to the
// tenant-scoped base query. Text filters are case-insensitive literal substring
// matches (postgres.ILikeContains).
func listFilterWhere(f user.ListFilters, a *postgres.Args) string {
	where := ""
	if f.Nickname != "" {
		where += ` AND ` + postgres.ILikeContains("nickname", a, f.Nickname)
	}
	if f.Email != "" {
		where += ` AND ` + postgres.ILikeContains("email", a, f.Email)
	}
	if f.Role != "" {
		where += ` AND role = ` + a.Add(f.Role)
	}
	if f.Active == "1" || f.Active == "0" {
		where += ` AND active = ` + a.Add(f.Active == "1")
	}
	return where
}

// FindPage is the admin users grid read: FindAll's scoping plus filters,
// whitelisted sort and paging, with the filtered total. Both statements run in
// the query's read transaction — one snapshot for the pager.
func (r *Repository) FindPage(ctx context.Context, c user.ListCriteria) (user.ListPage, error) {
	a := postgres.Args{r.Tenant(ctx)}
	base := `FROM users WHERE tenant_id = $1 AND role <> 'superadmin'` + listFilterWhere(
		c.Filters,
		&a,
	)

	page := user.ListPage{Items: []user.User{}}
	if err := r.Conn(ctx).GetContext(ctx, &page.Total, `SELECT COUNT(*) `+base, a...); err != nil {
		return user.ListPage{}, err
	}

	col, ok := listSortSQL[c.Sort]
	if !ok {
		// Unreachable via SortColumnFrom; belt against a future raw criteria.
		col = "nickname" + database.CollateSort
	}
	// The id tie-break keeps rows equal on the sort column from straddling a page
	// boundary.
	orderBy := ` ORDER BY ` + col + ` ` + string(c.SortDir) + `, id ASC`
	err := r.Conn(ctx).SelectContext(ctx, &page.Items,
		`SELECT * `+base+orderBy+` LIMIT `+a.Add(c.PerPage)+` OFFSET `+a.Add(c.Offset()), a...)
	return page, err
}

// platformSort is a platform grid sort column; nullable ones order NULL as SQLite
// does (postgres.NullsSmallest).
type platformSort struct {
	expr     string
	nullable bool
}

var platformSortSQL = map[user.SortColumn]platformSort{
	user.SortByTenant:    {expr: "t.name" + database.CollateSort},
	user.SortByNickname:  {expr: "u.nickname" + database.CollateSort},
	user.SortByEmail:     {expr: "u.email" + database.CollateSort},
	user.SortByRole:      {expr: "u.role" + database.CollateSort},
	user.SortByLastLogin: {expr: "u.last_login_at", nullable: true},
}

func platformFilterWhere(f user.PlatformListFilters, a *postgres.Args) string {
	where := ""
	if f.Nickname != "" {
		where += ` AND ` + postgres.ILikeContains("u.nickname", a, f.Nickname)
	}
	if f.Email != "" {
		where += ` AND ` + postgres.ILikeContains("u.email", a, f.Email)
	}
	if f.Role != "" {
		where += ` AND u.role = ` + a.Add(f.Role)
	}
	if f.Active == "1" || f.Active == "0" {
		where += ` AND u.active = ` + a.Add(f.Active == "1")
	}
	if f.Tenant != "" {
		where += ` AND ` + postgres.ILikeContains("t.name", a, f.Tenant)
	}
	return where
}

// FindPageAcrossTenants is the platform users grid read — cross-tenant on
// purpose (marker below), joined to the tenant name. The secondary nickname sort
// keeps pages stable when the primary column has ties.
func (r *Repository) FindPageAcrossTenants(
	ctx context.Context,
	c user.PlatformListCriteria,
) (user.PlatformListPage, error) {
	var a postgres.Args
	base := `FROM users u
		 INNER JOIN tenants t ON t.id = u.tenant_id /* tenant-scope-exempt: platform superadmin */
		 WHERE true` + platformFilterWhere(c.Filters, &a)

	page := user.PlatformListPage{Items: []user.PlatformRow{}}
	if err := r.SystemConn(ctx).GetContext(ctx, &page.Total, `SELECT COUNT(*) `+base, a...); err != nil {
		return user.PlatformListPage{}, err
	}

	col, ok := platformSortSQL[c.Sort]
	if !ok {
		// Unreachable via PlatformSortColumnFrom; buckles to the domain's default
		// column — see the SQLite twin for why it must not pick another.
		col = platformSortSQL[user.SortByNickname]
	}
	term := col.expr + ` ` + string(c.SortDir)
	if col.nullable {
		term += postgres.NullsSmallest(c.SortDir)
	}
	orderBy := ` ORDER BY ` + term + `, u.nickname` + database.CollateSort + ` ASC`
	err := r.SystemConn(ctx).SelectContext(ctx, &page.Items,
		`SELECT u.id, u.nickname, u.email, u.role, u.active, u.tenant_id,
		        t.name AS tenant_name, u.last_login_at `+base+orderBy+
			` LIMIT `+a.Add(c.PerPage)+` OFFSET `+a.Add(c.Offset()), a...)
	return page, err
}

// bulkWhere renders the dual-mode selection: an optional actor exclusion, then
// either the filter set (AllFiltered) or an explicit id list. A malformed id
// matches no row, so it is dropped from the list — and a malformed exclusion
// excludes nothing that could match.
func bulkWhere(sel user.BulkSelection, a *postgres.Args) string {
	where := ""
	if id, ok := postgres.ParseID(sel.ExcludeID); ok {
		where += ` AND id <> ` + a.Add(id)
	}
	if sel.AllFiltered {
		return where + listFilterWhere(sel.Filters, a)
	}
	return where + ` AND id = ANY(` + a.Add(postgres.ParseIDs(sel.IDs)) + `)`
}

func (r *Repository) BulkDelete(ctx context.Context, sel user.BulkSelection) (int64, error) {
	if sel.IsEmpty() {
		return 0, nil
	}
	a := postgres.Args{r.Tenant(ctx)}
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM users WHERE id IN (
		   SELECT id FROM users WHERE tenant_id = $1 AND role <> 'superadmin'`+
			bulkWhere(sel, &a)+postgres.LockInIDOrder+`)`, a...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) BulkSetActive(
	ctx context.Context,
	sel user.BulkSelection,
	active bool,
) (int64, error) {
	if sel.IsEmpty() {
		return 0, nil
	}
	a := postgres.Args{active, time.Now().UTC(), r.Tenant(ctx)}
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET active = $1, updated_at = $2 WHERE id IN (
		   SELECT id FROM users WHERE tenant_id = $3 AND role <> 'superadmin'`+
			bulkWhere(sel, &a)+postgres.LockInIDOrder+`)`, a...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// platformBulkFilterWhere renders the platform grid filters against a bare users
// statement (DELETE/UPDATE cannot join) — the tenant-name filter goes through a
// subquery.
func platformBulkFilterWhere(f user.PlatformListFilters, a *postgres.Args) string {
	where := listFilterWhere(f.ListFilters, a)
	if f.Tenant != "" {
		where += ` AND tenant_id IN (SELECT t.id FROM tenants t WHERE ` +
			postgres.ILikeContains("t.name", a, f.Tenant) + `)`
	}
	return where
}

func platformBulkWhere(sel user.PlatformBulkSelection, a *postgres.Args) string {
	where := ""
	if id, ok := postgres.ParseID(sel.ExcludeID); ok {
		where += ` AND id <> ` + a.Add(id)
	}
	if sel.AllFiltered {
		return where + platformBulkFilterWhere(sel.Filters, a)
	}
	return where + ` AND id = ANY(` + a.Add(postgres.ParseIDs(sel.IDs)) + `)`
}

// BulkDeleteAcrossTenants is BulkDelete's platform twin: any tenant (marker
// below), superadmin rows and the actor always spared.
func (r *Repository) BulkDeleteAcrossTenants(
	ctx context.Context,
	sel user.PlatformBulkSelection,
) (int64, error) {
	if sel.IsEmpty() {
		return 0, nil
	}
	var a postgres.Args
	res, err := r.SystemConn(ctx).ExecContext(ctx,
		`DELETE FROM users /* tenant-scope-exempt: platform superadmin */ WHERE id IN (
		   SELECT id FROM users WHERE role <> 'superadmin'`+
			platformBulkWhere(sel, &a)+postgres.LockInIDOrder+`)`, a...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) BulkSetActiveAcrossTenants(
	ctx context.Context,
	sel user.PlatformBulkSelection,
	active bool,
) (int64, error) {
	if sel.IsEmpty() {
		return 0, nil
	}
	a := postgres.Args{active, time.Now().UTC()}
	res, err := r.SystemConn(ctx).ExecContext(ctx,
		`UPDATE users /* tenant-scope-exempt: platform superadmin */
		    SET active = $1, updated_at = $2 WHERE id IN (
		   SELECT id FROM users WHERE role <> 'superadmin'`+
			platformBulkWhere(sel, &a)+postgres.LockInIDOrder+`)`, a...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
