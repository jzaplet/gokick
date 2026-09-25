package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// roleFacts is what the startup check needs to know about the role a pool
// connects as.
type roleFacts struct {
	Name      string `db:"rolname"`
	Super     bool   `db:"rolsuper"`
	BypassRLS bool   `db:"rolbypassrls"`
	// OwnedTables counts the tables (and views) of the current database whose
	// owner the role is, or is a member of. An owner can switch row security off
	// on its own tables, and row security never applies to the owner anyway.
	OwnedTables int `db:"owned_tables"`
}

const roleFactsQuery = `
SELECT r.rolname, r.rolsuper, r.rolbypassrls,
       (SELECT count(*)
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind IN ('r', 'p', 'v', 'm')
           AND n.nspname NOT IN ('pg_catalog', 'information_schema')
           AND n.nspname NOT LIKE 'pg\_%'
           AND pg_has_role(current_user, c.relowner, 'MEMBER')) AS owned_tables
  FROM pg_roles r
 WHERE r.rolname = current_user`

// VerifyRoles fails when either pool connects as a role that would quietly defeat
// the tenant wall — the check that row-level security is really on, since a
// misconfigured DSN would otherwise pass every test and still leak across
// tenants:
//
//   - the tenant plane (APP_DB_URL) must not be a superuser (superusers bypass
//     row security even under FORCE), must not have BYPASSRLS, and must not own —
//     or be a member of the owner of — any table;
//   - the system plane (APP_DB_SYSTEM_URL) must have BYPASSRLS (without it every
//     cross-tenant read would see a filtered, silently wrong result), and must be
//     neither a superuser nor an owner — it needs to cross tenants, not to rewrite
//     the schema.
//
// It runs after the migrations (Migrator.RunUp calls it), when the tables whose
// ownership it checks exist.
func (m *Manager) VerifyRoles(ctx context.Context) error {
	app, err := readRoleFacts(ctx, m.app, "APP_DB_URL")
	if err != nil {
		return err
	}
	system, err := readRoleFacts(ctx, m.system, "APP_DB_SYSTEM_URL")
	if err != nil {
		return err
	}
	return checkRoles(app, system)
}

func readRoleFacts(ctx context.Context, db *sqlx.DB, key string) (roleFacts, error) {
	var f roleFacts
	if err := db.GetContext(ctx, &f, roleFactsQuery); err != nil {
		return f, fmt.Errorf("postgres: read the role of %s: %w", key, err)
	}
	return f, nil
}

// checkRoles applies the VerifyRoles rules to the facts of both roles.
func checkRoles(app, system roleFacts) error {
	if err := checkRole("APP_DB_URL", "gokick_app", app, false); err != nil {
		return err
	}
	return checkRole("APP_DB_SYSTEM_URL", "gokick_system", system, true)
}

// checkRole refuses a superuser or a table owner on either plane, and a role whose
// BYPASSRLS is not the plane's (the tenant plane must not have it, the system
// plane must). role is the gokick role the plane should connect as.
func checkRole(key, role string, f roleFacts, bypassRLS bool) error {
	switch {
	case f.Super:
		return fmt.Errorf("postgres: %s connects as %q, a superuser — superusers bypass "+
			"row-level security and can rewrite the schema; connect as %s", key, f.Name, role)
	case f.BypassRLS && !bypassRLS:
		return fmt.Errorf("postgres: %s connects as %q, which has BYPASSRLS — the tenant "+
			"plane must be bound by row-level security; connect as %s", key, f.Name, role)
	case !f.BypassRLS && bypassRLS:
		return fmt.Errorf("postgres: %s connects as %q, which lacks BYPASSRLS — cross-tenant "+
			"work would see a silently filtered result; connect as %s", key, f.Name, role)
	case f.OwnedTables > 0:
		return fmt.Errorf("postgres: %s connects as %q, which owns (or is a member of the "+
			"owner of) %d tables — row-level security does not bind a table's owner; connect "+
			"as %s, never as the schema owner", key, f.Name, f.OwnedTables, role)
	}
	return nil
}
