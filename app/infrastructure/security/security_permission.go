package security

import (
	"context"
	"strings"

	"myapp/app/domain/shared"
	"myapp/app/domain/user"
)

type PermissionChecker struct{}

func NewPermissionChecker() *PermissionChecker {
	return &PermissionChecker{}
}

func (c *PermissionChecker) Check(ctx context.Context, permission string) error {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return &shared.AuthError{Message: "authentication required"}
	}

	// Permission format: "resource:action" or "resource:action:role"
	// Admin role has access to everything.
	if claims.Role == string(user.RoleAdmin) {
		return nil
	}

	// Non-admin: deny any permission requiring admin role.
	if strings.HasPrefix(permission, "admin:") {
		return &shared.AuthError{Message: "admin access required"}
	}

	return nil
}
