package user

import "time"

// PlatformRow is a read model for the cross-tenant platform user list: a user
// joined to the name of its tenant (so the superadmin sees "Acme", not a UUID).
// Primitives only, no behavior — it never leaves the read path.
type PlatformRow struct {
	ID         string `db:"id"`
	Nickname   string `db:"nickname"`
	Email      string `db:"email"`
	Role       string `db:"role"`
	Active     bool   `db:"active"`
	TenantID   string `db:"tenant_id"`
	TenantName string `db:"tenant_name"`
	// nil until the user has logged in at least once — *time.Time, same nullable-
	// time shape as the User entity and run.Run (nil = unset).
	LastLoginAt *time.Time `db:"last_login_at"`
}
