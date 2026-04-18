package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestLogoutHandler_DeletesAllUserTokens(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "logout_all.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")

	// Simulate several active sessions (e.g. phone, laptop, tablet).
	fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))
	fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))
	fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))
	fx.AssertTokenCount(t, 3)

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: u.ID, Role: "user", Nickname: u.Nickname,
	})

	handler := NewLogoutHandler(fx.Tokens)
	if err := handler.Handle(authCtx, LogoutCommand{}); err != nil {
		t.Fatalf("logout: %v", err)
	}

	fx.AssertTokenCount(t, 0)
}

func TestLogoutHandler_DoesNotTouchOtherUsers(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "logout_scope.db"))
	alice := fx.SeedUser(t, "alice", "pwd", "user")
	bob := fx.SeedUser(t, "bob", "pwd", "user")

	fx.SeedRefreshToken(t, alice.ID, time.Now().Add(24*time.Hour))
	fx.SeedRefreshToken(t, bob.ID, time.Now().Add(24*time.Hour))
	fx.AssertTokenCount(t, 2)

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: alice.ID, Role: "user", Nickname: alice.Nickname,
	})

	handler := NewLogoutHandler(fx.Tokens)
	if err := handler.Handle(authCtx, LogoutCommand{}); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// Bob's token must survive.
	fx.AssertTokenCount(t, 1)
}

func TestLogoutHandler_WithoutClaimsReturnsAuthError(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "logout_noauth.db"))

	handler := NewLogoutHandler(fx.Tokens)
	err := handler.Handle(ctx, LogoutCommand{})

	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestLogoutCommand_RequiredPermission(t *testing.T) {
	if got := (LogoutCommand{}).RequiredPermission(); got != "auth:logout" {
		t.Fatalf("expected permission auth:logout, got %q", got)
	}
}
