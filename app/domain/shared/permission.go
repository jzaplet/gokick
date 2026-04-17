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

// IsPermissionAllowedForRole reports whether the given role may execute an
// operation requiring the specified permission. Rule: "admin" role has access
// to everything; any other role is denied permissions prefixed with "admin:".
func IsPermissionAllowedForRole(permission, role string) bool {
	if role == "admin" {
		return true
	}

	return !strings.HasPrefix(permission, "admin:")
}
