package security

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

type PermissionChecker struct{}

func NewPermissionChecker() *PermissionChecker {
	return &PermissionChecker{}
}

func (c *PermissionChecker) Check(ctx context.Context, permission string) error {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return &shared.AuthError{Key: msgkey.AuthRequired}
	}

	if !shared.IsPermissionAllowedForRole(permission, claims.Role) {
		return &shared.PermissionError{Key: msgkey.PermissionDenied}
	}

	return nil
}
