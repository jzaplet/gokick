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

func TestLoginHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_success.db"))
	u := fx.SeedUser(t, "alice", "super-secret", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	result, err := handler.Handle(ctx, LoginCommand{Nickname: "alice", Password: "super-secret"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}
	if result.User.ID != u.ID {
		t.Fatalf("user id mismatch: got %s want %s", result.User.ID, u.ID)
	}
	if result.AccessExpiresIn <= 0 {
		t.Fatalf("expected positive expiration, got %v", result.AccessExpiresIn)
	}
	if !result.RefreshExpiresAt.After(time.Now()) {
		t.Fatal("expected refresh token to expire in the future")
	}

	stored, err := fx.Tokens.FindByHash(ctx, fx.HashToken(result.RefreshToken))
	if err != nil {
		t.Fatalf("find token: %v", err)
	}
	if stored == nil {
		t.Fatal("expected refresh token persisted in DB")
	}
	if stored.UserID != u.ID {
		t.Fatalf("token user_id mismatch: got %s want %s", stored.UserID, u.ID)
	}
}

func TestLoginHandler_WrongPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_wrong_pwd.db"))
	fx.SeedUser(t, "bob", "correct-password", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, err := handler.Handle(ctx, LoginCommand{Nickname: "bob", Password: "wrong-password"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestLoginHandler_UnknownUser(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_unknown.db"))

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, err := handler.Handle(ctx, LoginCommand{Nickname: "ghost", Password: "anything"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestLoginHandler_NoRefreshTokenOnFailure(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_no_token_on_fail.db"))
	fx.SeedUser(t, "charlie", "right", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "charlie", Password: "wrong"})

	fx.AssertTokenCount(t, 0)
}
