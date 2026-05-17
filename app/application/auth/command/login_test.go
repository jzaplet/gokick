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

// One bad password bumps the brute-force counter to 1. The handler still
// returns a neutral AuthError so the response shape gives nothing away.
func TestLoginHandler_FailedLoginIncrementsCounter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_inc.db"))
	u := fx.SeedUser(t, "dora", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "dora", Password: "wrong"})

	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 1 {
		t.Fatalf("counter: got %d want 1", got.FailedLoginAttempts)
	}
}

// Five failures inside the window should lock the account; the next
// login attempt — even with the correct password — must be rejected with
// the same neutral error.
func TestLoginHandler_LocksAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_lockout.db"))
	fx.SeedUser(t, "evan", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	// 5 bad attempts → counter resets to 0 on the 5th and lock kicks in.
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "evan", Password: "wrong"})
	}

	// Correct password but locked — still AuthError, response shape gives nothing away.
	_, err := handler.Handle(ctx, LoginCommand{Nickname: "evan", Password: "correct-pw"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("locked account must return *shared.AuthError, got %T: %v", err, err)
	}

	fx.AssertTokenCount(t, 0)
}

// A successful login clears the counter so the next failure cycle
// starts fresh.
func TestLoginHandler_SuccessResetsCounter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_reset.db"))
	u := fx.SeedUser(t, "frank", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	// Seed a couple of failures.
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "frank", Password: "wrong"})
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "frank", Password: "wrong"})

	pre, _ := fx.Users.FindByID(ctx, u.ID)
	if pre.FailedLoginAttempts != 2 {
		t.Fatalf("setup: expected counter=2, got %d", pre.FailedLoginAttempts)
	}

	if _, err := handler.Handle(ctx, LoginCommand{Nickname: "frank", Password: "correct-pw"}); err != nil {
		t.Fatalf("good login should succeed: %v", err)
	}

	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 0 {
		t.Fatalf("success must reset counter, got %d", got.FailedLoginAttempts)
	}
}
