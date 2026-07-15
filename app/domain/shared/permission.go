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

// SkipsTransaction is the command-level opt-out marker for commands that MUST run
// outside the bus-managed transaction. It lives here with the other command
// declaration markers (Permissioned / SkipPermission) rather than in the bus
// middleware, so the three share one home. The WHY (raw-pool self-deadlock,
// force-logout persistence) is documented at TransactionMiddleware, the enforcer.
type SkipsTransaction interface {
	SkipTransaction()
}

// CLIOnly marks a Permissioned that is dispatched ONLY through the operator-
// trusted SystemCommandBus (a CLI create/seed path) and has no HTTP route. The
// system bus skips AuthorizeMiddleware, so its RequiredPermission is never
// enforced against a caller — it names the operation but nothing checks it. Such
// a permission must NOT leak into the FE-facing PermissionsRegistry: the frontend
// can never invoke the operation, so surfacing the permission would advertise a
// phantom capability. NewPermissionsRegistry excludes any item implementing this.
type CLIOnly interface {
	CLIOnly()
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
