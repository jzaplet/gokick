package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestChangePasswordHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pwd_success.db"))
	u := fx.SeedUser(t, "alice", "old-password", "user")

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	})

	handler := NewChangePasswordHandler(fx.Users, fx.Hasher)
	err := handler.Handle(authCtx, ChangePasswordCommand{
		OldPassword: "old-password",
		NewPassword: "brand-new-password",
	})
	if err != nil {
		t.Fatalf("change password: %v", err)
	}

	// Reload user and verify the new password works, the old one does not.
	reloaded, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.PasswordHash == u.PasswordHash {
		t.Fatal("expected password hash to change")
	}
	if err := fx.Hasher.Verify("brand-new-password", reloaded.PasswordHash); err != nil {
		t.Fatalf("new password should verify: %v", err)
	}
	if err := fx.Hasher.Verify("old-password", reloaded.PasswordHash); err == nil {
		t.Fatal("old password must no longer verify")
	}
}

func TestChangePasswordHandler_WrongOldPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pwd_wrong_old.db"))
	u := fx.SeedUser(t, "alice", "real-password", "user")
	originalHash := u.PasswordHash

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	})

	handler := NewChangePasswordHandler(fx.Users, fx.Hasher)
	err := handler.Handle(authCtx, ChangePasswordCommand{
		OldPassword: "wrong-old",
		NewPassword: "new-one",
	})

	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}

	// Password must remain unchanged.
	reloaded, _ := fx.Users.FindByID(ctx, u.ID)
	if reloaded.PasswordHash != originalHash {
		t.Fatal("password hash must not change on failed verification")
	}
}

func TestChangePasswordHandler_WithoutClaimsReturnsAuthError(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pwd_noauth.db"))

	handler := NewChangePasswordHandler(fx.Users, fx.Hasher)
	err := handler.Handle(ctx, ChangePasswordCommand{OldPassword: "x", NewPassword: "y"})

	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestChangePasswordHandler_UnknownUser(t *testing.T) {
	// Claims reference a user ID that doesn't exist in DB (e.g. user deleted after token issued).
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pwd_unknown.db"))

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: "00000000-0000-0000-0000-000000000000", Role: "user", Nickname: "ghost",
	})

	handler := NewChangePasswordHandler(fx.Users, fx.Hasher)
	err := handler.Handle(authCtx, ChangePasswordCommand{OldPassword: "a", NewPassword: "b"})

	var validationErr *shared.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
}

func TestChangePasswordHandler_InvalidNewPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pwd_invalid_new.db"))
	u := fx.SeedUser(t, "alice", "old-password", "user")
	originalHash := u.PasswordHash

	authCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: u.ID, Role: u.Role, Nickname: u.Nickname,
	})

	handler := NewChangePasswordHandler(fx.Users, fx.Hasher)
	err := handler.Handle(authCtx, ChangePasswordCommand{
		OldPassword: "old-password",
		NewPassword: "short",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "new_password" {
		t.Fatalf("expected field=new_password, got %s", ve.Field)
	}

	// Password must remain unchanged.
	reloaded, _ := fx.Users.FindByID(ctx, u.ID)
	if reloaded.PasswordHash != originalHash {
		t.Fatal("password hash must not change on invalid new password")
	}
}

func TestChangePasswordCommand_RequiredPermission(t *testing.T) {
	if got := (ChangePasswordCommand{}).RequiredPermission(); got != "profile:update" {
		t.Fatalf("expected permission profile:update, got %q", got)
	}
}
