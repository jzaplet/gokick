package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/application/auth/command/internal/testfx"
	"gokick/app/domain/shared"
)

func TestRefreshTokenHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_success.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	result, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.RefreshToken == "" || result.RefreshToken == raw {
		t.Fatal("expected new (rotated) refresh token")
	}
	if result.User.ID != u.ID {
		t.Fatalf("user id mismatch: got %s want %s", result.User.ID, u.ID)
	}

	// Old token must be gone.
	old, _ := fx.Tokens.FindByHash(ctx, fx.HashToken(raw))
	if old != nil {
		t.Fatal("expected old refresh token to be removed")
	}
	// Exactly one new token should exist.
	fx.AssertTokenCount(t, 1)
	// The persisted token must match the returned raw.
	stored, _ := fx.Tokens.FindByHash(ctx, fx.HashToken(result.RefreshToken))
	if stored == nil || stored.UserID != u.ID {
		t.Fatal("expected new refresh token persisted for user")
	}
}

func TestRefreshTokenHandler_Expired(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_expired.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(-1*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
	// Expired token must be cleaned up.
	fx.AssertTokenCount(t, 0)
}

func TestRefreshTokenHandler_UnknownToken(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_unknown.db"))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: "not-a-real-token"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
	fx.AssertTokenCount(t, 0)
}

func TestRefreshTokenHandler_UserDeletedAfterIssue(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_user_gone.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	// Simulate the user being deleted after the token was issued (cascades to refresh_tokens).
	if err := fx.Users.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestRefreshTokenHandler_RotationPreventsReuse(t *testing.T) {
	// After a successful refresh, the original raw token must be invalid.
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_rotation.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	if _, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw}); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError on reuse, got %T: %v", err, err)
	}
}
