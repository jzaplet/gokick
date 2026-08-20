package user

import (
	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

// Role is the wire contract for a user's role, not just a Go VO: tsgen emits it
// as the FE's Role const + type + guard (which is why the consts below must stay
// resolvable to string literals — see tools/gk/tsgen/union.go).
//
//gkts:assets/app/Auth/enums/roles.ts Role union
type Role string

const (
	// RoleSuperAdmin is the platform-level role (above admin/user): it sees
	// across all tenants and is the only role granted platform:* permissions.
	// Seeded out-of-band (APP_SEED_SUPERADMIN_PASSWORD), not via normal signup.
	// The VO consts are conversions of the canonical shared.Role* strings so the
	// authorization ladder (shared.IsPermissionAllowedForRole) and this VO share
	// one definition of each role name.
	RoleSuperAdmin = Role(shared.RoleSuperAdmin)
	RoleAdmin      = Role(shared.RoleAdmin)
	RoleUser       = Role(shared.RoleUser)
)

func NewRole(s string) (Role, error) {
	switch Role(s) {
	case RoleSuperAdmin, RoleAdmin, RoleUser:
		return Role(s), nil
	default:
		return "", &shared.ValidationError{Field: "role", Key: msgkey.UserRoleInvalid}
	}
}

// IsSuperAdmin reports whether this is the platform-level superadmin role.
// Tenant-facing user management (CreateUser/UpdateUser, gated by admin:users:*)
// must REFUSE to assign it: superadmin is seeded out-of-band, never minted or
// promoted through the admin API — otherwise a tenant admin could self-escalate
// to the cross-tenant platform plane. NewRole stays permissive because the
// seeder legitimately constructs the role.
func (r Role) IsSuperAdmin() bool {
	return r == RoleSuperAdmin
}
