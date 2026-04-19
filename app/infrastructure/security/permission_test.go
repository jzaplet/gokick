package security

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
)

func TestCheck_NoClaims(t *testing.T) {
	c := NewPermissionChecker()
	err := c.Check(context.Background(), "any:action")
	if err == nil {
		t.Fatal("expected error when no claims in context")
	}
}

func TestCheck_AdminHasFullAccess(t *testing.T) {
	c := NewPermissionChecker()
	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "u1", Role: "admin", Nickname: "root",
	})
	if err := c.Check(ctx, "admin:users:delete"); err != nil {
		t.Fatalf("admin should have full access: %v", err)
	}
	if err := c.Check(ctx, "profile:read"); err != nil {
		t.Fatalf("admin should access user-level resources: %v", err)
	}
}

func TestCheck_UserDeniedAdminPermission(t *testing.T) {
	c := NewPermissionChecker()
	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "u2", Role: "user", Nickname: "jane",
	})
	err := c.Check(ctx, "admin:users:list")
	if err == nil {
		t.Fatal("user role should be denied admin permissions")
	}
}

func TestCheck_UserAllowedNonAdminPermission(t *testing.T) {
	c := NewPermissionChecker()
	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "u2", Role: "user", Nickname: "jane",
	})
	if err := c.Check(ctx, "profile:read"); err != nil {
		t.Fatalf("user should access non-admin resources: %v", err)
	}
}

func TestCheck_ErrorIsAuthError(t *testing.T) {
	c := NewPermissionChecker()
	err := c.Check(context.Background(), "anything")
	if _, ok := err.(*shared.AuthError); !ok {
		t.Fatalf("expected *shared.AuthError, got %T", err)
	}
}
