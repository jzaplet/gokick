package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/application/auth/command/internal/testfx"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

func TestLoginHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_success.db"))

	// Seed user
	const rawPassword = "super-secret"
	hash, err := fx.Hasher.Hash(rawPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	nickname, _ := user.NewNickname("alice")
	role, _ := user.NewRole("user")
	u := user.NewUser(nickname, hash, "alice@example.com", role)
	if err := fx.Users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	result, err := handler.Handle(ctx, LoginCommand{Nickname: "alice", Password: rawPassword})
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

	// Refresh token must be persisted
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

	hash, _ := fx.Hasher.Hash("correct-password")
	nickname, _ := user.NewNickname("bob")
	role, _ := user.NewRole("user")
	u := user.NewUser(nickname, hash, "bob@example.com", role)
	_ = fx.Users.Save(ctx, u)

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, err := handler.Handle(ctx, LoginCommand{Nickname: "bob", Password: "wrong-password"})
	if err == nil {
		t.Fatal("expected auth error")
	}
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T", err)
	}
}

func TestLoginHandler_UnknownUser(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_unknown.db"))

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, err := handler.Handle(ctx, LoginCommand{Nickname: "ghost", Password: "anything"})
	if err == nil {
		t.Fatal("expected auth error")
	}
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T", err)
	}
}

func TestLoginHandler_NoRefreshTokenOnFailure(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_no_token_on_fail.db"))

	hash, _ := fx.Hasher.Hash("right")
	nickname, _ := user.NewNickname("charlie")
	role, _ := user.NewRole("user")
	u := user.NewUser(nickname, hash, "charlie@example.com", role)
	_ = fx.Users.Save(ctx, u)

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "charlie", Password: "wrong"})

	// No tokens should have been persisted
	fx.AssertTokenCount(t, 0)
}
