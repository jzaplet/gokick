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
