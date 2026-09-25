//go:build !nosqlite

package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gokick/app/domain/user"
	"gokick/app/infrastructure/database"
)

// The grid reads and bulk writes live in their own file: repository.go holds
// the single-row CRUD + identity lookups, this file everything the DataGrid
// stack drives (paged/filtered/sorted reads, dual-mode bulk operations).

// listSortSQL maps the whitelisted sort columns onto SQL. The map (not string
// interpolation of the wire value) IS the injection guard — an unknown column
// cannot reach the query because SortColumnFrom already collapsed it.
//
// Text columns sort through the Czech collation (database.CollateSort) — the same
// order the Postgres adapter produces.
var listSortSQL = map[user.SortColumn]string{
	user.SortByNickname: "nickname" + database.CollateSort,
	user.SortByEmail:    "email" + database.CollateSort,
	user.SortByRole:     "role" + database.CollateSort,
}

// listFilterWhere renders the optional filter conditions appended to the
// tenant-scoped base query. Text filters are case-insensitive substring matches
// (Unicode-aware, so Č finds č); what the user typed is matched literally — a %
// or _ is searched for, not treated as a wildcard (database.LikeContains).
func listFilterWhere(f user.ListFilters) (string, []any) {
	where := ""
	args := []any{}
	if f.Nickname != "" {
		where += ` AND nickname LIKE ?` + database.LikeEscape
		args = append(args, database.LikeContains(f.Nickname))
	}
	if f.Email != "" {
		where += ` AND email LIKE ?` + database.LikeEscape
		args = append(args, database.LikeContains(f.Email))
	}
	if f.Role != "" {
		where += ` AND role = ?`
		args = append(args, f.Role)
	}
	if f.Active == "1" || f.Active == "0" {
		where += ` AND active = ?`
		args = append(args, f.Active == "1")
	}
	return where, args
}

// FindPage is the admin users grid read: FindAll's scoping plus filters,
// whitelisted sort and paging, returned together with the filtered total (one
// consistent snapshot for the pager). Criteria arrive pre-normalized from the
// query handler.
func (r *Repository) FindPage(ctx context.Context, c user.ListCriteria) (user.ListPage, error) {
	where, filterArgs := listFilterWhere(c.Filters)
	base := `FROM users WHERE tenant_id=? AND role != 'superadmin'`
	args := append([]any{r.Tenant(ctx)}, filterArgs...)

	page := user.ListPage{Items: []user.User{}}
	if err := r.Conn(ctx).GetContext(ctx, &page.Total,
		`SELECT COUNT(*) `+base+where, args...); err != nil {
		return user.ListPage{}, err
	}

	col, ok := listSortSQL[c.Sort]
	if !ok {
		// Unreachable via SortColumnFrom; belt against a future raw criteria.
		col = "nickname" + database.CollateSort
	}
	// Secondary id sort is the tie-break anchor: without it, rows equal on the
	// primary column (role, email…) have an unspecified order across the
	// LIMIT/OFFSET page boundary, so a row could appear on two pages or none.
	orderBy := fmt.Sprintf(` ORDER BY %s %s, id ASC`, col, c.SortDir)
	err := r.Conn(ctx).SelectContext(ctx, &page.Items,
		`SELECT * `+base+where+orderBy+` LIMIT ? OFFSET ?`,
		append(args, c.PerPage, c.Offset())...)
	return page, err
}

// platformSortSQL is the platform grid's whitelist→SQL map. Text columns sort
// through the Czech collation (database.CollateSort); last_login sorts through
// julianday() — the repo-wide datetime comparison discipline (TEXT timestamps
// compare wrong lexically; the sqltime gate enforces this).
var platformSortSQL = map[user.SortColumn]string{
	user.SortByTenant:    "t.name" + database.CollateSort,
	user.SortByNickname:  "u.nickname" + database.CollateSort,
	user.SortByEmail:     "u.email" + database.CollateSort,
	user.SortByRole:      "u.role" + database.CollateSort,
	user.SortByLastLogin: "julianday(u.last_login_at)",
}

func platformFilterWhere(f user.PlatformListFilters) (string, []any) {
	where := ""
	args := []any{}
	if f.Nickname != "" {
		where += ` AND u.nickname LIKE ?` + database.LikeEscape
		args = append(args, database.LikeContains(f.Nickname))
	}
	if f.Email != "" {
		where += ` AND u.email LIKE ?` + database.LikeEscape
		args = append(args, database.LikeContains(f.Email))
	}
	if f.Role != "" {
		where += ` AND u.role = ?`
		args = append(args, f.Role)
	}
	if f.Active == "1" || f.Active == "0" {
		where += ` AND u.active = ?`
		args = append(args, f.Active == "1")
	}
	if f.Tenant != "" {
		where += ` AND t.name LIKE ?` + database.LikeEscape
		args = append(args, database.LikeContains(f.Tenant))
	}
	return where, args
}

// FindPageAcrossTenants is the platform users grid read — cross-tenant on
// purpose (marker below), joined to the tenant name. The secondary nickname
// sort keeps pages stable when the primary column has ties.
func (r *Repository) FindPageAcrossTenants(
	ctx context.Context,
	c user.PlatformListCriteria,
) (user.PlatformListPage, error) {
	where, filterArgs := platformFilterWhere(c.Filters)
	base := `FROM users u
		 INNER JOIN tenants t ON t.id = u.tenant_id /* tenant-scope-exempt: platform superadmin */
		 WHERE 1=1`

	page := user.PlatformListPage{Items: []user.PlatformRow{}}
	if err := r.Conn(ctx).GetContext(ctx, &page.Total,
		`SELECT COUNT(*) `+base+where, filterArgs...); err != nil {
		return user.PlatformListPage{}, err
	}

	col, ok := platformSortSQL[c.Sort]
	if !ok {
		// Unreachable via PlatformSortColumnFrom; belt against a future raw
		// criteria. It buckles to the column the DOMAIN declares as its default,
		// not to a second opinion — PlatformListCriteria.Normalize clamps
		// Page/PerPage/SortDir but NOT Sort, so a caller who reasonably expects
		// Normalize to make their criteria safe lands here, and a belt that picks
		// a different column than the whitelist's default would quietly hand them
		// tenant-ordered rows under a "Nickname" header.
		col = "u.nickname" + database.CollateSort
	}
	orderBy := fmt.Sprintf(
		` ORDER BY %s %s, u.nickname%s ASC`,
		col,
		c.SortDir,
		database.CollateSort,
	)
	err := r.Conn(ctx).SelectContext(ctx, &page.Items,
		`SELECT u.id, u.nickname, u.email, u.role, u.active, u.tenant_id,
		        t.name AS tenant_name, u.last_login_at `+base+where+orderBy+` LIMIT ? OFFSET ?`,
		append(filterArgs, c.PerPage, c.Offset())...)
	return page, err
}

// bulkWhere renders the dual-mode selection: an optional actor exclusion
// (self-protection for delete/deactivate — activate leaves it empty since
// activating yourself is harmless), then either the filter set (AllFiltered)
// or an explicit id list narrows the statement.
func bulkWhere(sel user.BulkSelection) (string, []any) {
	where := ""
	args := []any{}
	if sel.ExcludeID != "" {
		where += ` AND id != ?`
		args = append(args, sel.ExcludeID)
	}
	if sel.AllFiltered {
		fw, fargs := listFilterWhere(sel.Filters)
		return where + fw, append(args, fargs...)
	}
	where += ` AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(sel.IDs)), ",") + `)`
	for _, id := range sel.IDs {
		args = append(args, id)
	}
	return where, args
}

func (r *Repository) BulkDelete(ctx context.Context, sel user.BulkSelection) (int64, error) {
	if sel.IsEmpty() {
		return 0, nil
	}
	where, args := bulkWhere(sel)
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM users WHERE tenant_id=? AND role != 'superadmin'`+where,
		append([]any{r.Tenant(ctx)}, args...)...)
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
	where, args := bulkWhere(sel)
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET active = ?, updated_at = ? WHERE tenant_id=? AND role != 'superadmin'`+where,
		append([]any{active, time.Now().UTC(), r.Tenant(ctx)}, args...)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// platformBulkFilterWhere renders the platform grid filters against a bare
// users statement (DELETE/UPDATE cannot join) — the tenant-name filter goes
// through a subquery.
func platformBulkFilterWhere(f user.PlatformListFilters) (string, []any) {
	where, args := listFilterWhere(f.ListFilters)
	if f.Tenant != "" {
		where += ` AND tenant_id IN (SELECT t.id FROM tenants t WHERE t.name LIKE ?` + database.LikeEscape + `)`
		args = append(args, database.LikeContains(f.Tenant))
	}
	return where, args
}

func platformBulkWhere(sel user.PlatformBulkSelection) (string, []any) {
	where := ""
	args := []any{}
	if sel.ExcludeID != "" {
		where += ` AND id != ?`
		args = append(args, sel.ExcludeID)
	}
	if sel.AllFiltered {
		fw, fargs := platformBulkFilterWhere(sel.Filters)
		return where + fw, append(args, fargs...)
	}
	where += ` AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(sel.IDs)), ",") + `)`
	for _, id := range sel.IDs {
		args = append(args, id)
	}
	return where, args
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
	where, args := platformBulkWhere(sel)
	res, err := r.Conn(ctx).ExecContext(ctx,
		`DELETE FROM users /* tenant-scope-exempt: platform superadmin */
		  WHERE role != 'superadmin'`+where, args...)
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
	where, args := platformBulkWhere(sel)
	res, err := r.Conn(ctx).ExecContext(ctx,
		`UPDATE users /* tenant-scope-exempt: platform superadmin */
		    SET active = ?, updated_at = ? WHERE role != 'superadmin'`+where,
		append([]any{active, time.Now().UTC()}, args...)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
