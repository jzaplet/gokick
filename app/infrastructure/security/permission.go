package security

import (
	"context"

	"gokick/app/domain/shared"
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

	if !shared.IsPermissionAllowedForRole(permission, claims.Role) {
		return &shared.PermissionError{Message: "insufficient permissions"}
	}

	return nil
}
