package user

import "database/sql"

// PlatformRow is a read model for the cross-tenant platform user list: a user
// joined to the name of its tenant (so the superadmin sees "Acme", not a UUID).
// Primitives only, no behavior — it never leaves the read path.
type PlatformRow struct {
	ID          string       `db:"id"`
	Nickname    string       `db:"nickname"`
	Email       string       `db:"email"`
	Role        string       `db:"role"`
	Active      bool         `db:"active"`
	TenantID    string       `db:"tenant_id"`
	TenantName  string       `db:"tenant_name"`
	LastLoginAt sql.NullTime `db:"last_login_at"`
}
