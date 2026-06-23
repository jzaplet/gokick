package user

import "gokick/app/domain/shared"

type Role string

const (
	// RoleSuperAdmin is the platform-level role (above admin/user): it sees
	// across all tenants and is the only role granted platform:* permissions.
	// Seeded out-of-band (APP_SEED_SUPERADMIN_PASSWORD), not via normal signup.
	RoleSuperAdmin Role = "superadmin"
	RoleAdmin      Role = "admin"
	RoleUser       Role = "user"
)

func NewRole(s string) (Role, error) {
	switch Role(s) {
	case RoleSuperAdmin, RoleAdmin, RoleUser:
		return Role(s), nil
	default:
		return "", &shared.ValidationError{Field: "role", Message: "invalid role"}
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
