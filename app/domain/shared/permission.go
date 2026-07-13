package shared

import (
	"context"
	"strings"
)

type Permissioned interface {
	RequiredPermission() string
}

type SkipPermission interface {
	SkipPermissionCheck()
}

type PermissionChecker interface {
	Check(ctx context.Context, permission string) error
}

// Canonical role identifiers — the single source of the role names. The
// user.Role value-object consts are defined as conversions of these
// (Role(shared.RoleAdmin), …) and IsPermissionAllowedForRole compares against
// them, so the authorization ladder and the domain VO can never drift apart on a
// bare string literal.
const (
	RoleSuperAdmin = "superadmin"
	RoleAdmin      = "admin"
	RoleUser       = "user"
)

// IsPermissionAllowedForRole reports whether the given role may execute an
// operation requiring the specified permission. Roles form a strict ladder:
//
//   - "superadmin" — the platform plane (app authors). Access to everything,
//     including platform:* (the cross-tenant overview). The only role above admin.
//   - "admin"      — tenant administrator. Access to admin:* and below, but NOT
//     platform:* — an admin manages their own tenant, never the platform.
//   - anything else (user, …) — denied both admin:* and platform:*.
//
// Order matters: the platform:* gate sits between superadmin and admin so that
// admin's "everything below" does not silently swallow the platform plane.
func IsPermissionAllowedForRole(permission, role string) bool {
	if role == RoleSuperAdmin {
		return true
	}

	if strings.HasPrefix(permission, "platform:") {
		return false
	}

	if role == RoleAdmin {
		return true
	}

	return !strings.HasPrefix(permission, "admin:")
}
