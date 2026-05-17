package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestUpdateUserHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_success.db"))
	target := fx.SeedUser(t, "bob", "oldpass12", "user")

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdateUserCommand{
		ID:       target.ID,
		Nickname: "robert",
		Password: "newpass12",
		Email:    "robert@example.com",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, err := fx.Users.FindByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if updated.Nickname != "robert" {
		t.Fatalf("nickname: got %q want robert", updated.Nickname)
	}
	if updated.Email != "robert@example.com" {
		t.Fatalf("email: got %q want robert@example.com", updated.Email)
	}
	if updated.Role != "admin" {
		t.Fatalf("role: got %q want admin", updated.Role)
	}
	if err := fx.Hasher.Verify("newpass12", updated.PasswordHash); err != nil {
		t.Fatalf("password verify: %v", err)
	}
}

func TestUpdateUserHandler_EmptyPasswordPreservesHash(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_keep_pwd.db"))
	target := fx.SeedUser(t, "bob", "originalpw", "user")
	originalHash := target.PasswordHash

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdateUserCommand{
		ID:       target.ID,
		Nickname: "bob",
		Password: "",
		Email:    "bob@example.com",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, err := fx.Users.FindByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if updated.PasswordHash != originalHash {
		t.Fatal("password hash should be preserved when Password is empty")
	}
	if err := fx.Hasher.Verify("originalpw", updated.PasswordHash); err != nil {
		t.Fatalf("original password should still verify: %v", err)
	}
}

func TestUpdateUserHandler_DuplicateNickname(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_dup_nick.db"))
	fx.SeedUser(t, "alice", "secret12", "user")
	target := fx.SeedUser(t, "bob", "secret12", "user")

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdateUserCommand{
		ID:       target.ID,
		Nickname: "alice",
		Email:    "bob@example.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("expected field=nickname, got %s", ve.Field)
	}
}

func TestUpdateUserHandler_KeepingOwnNickname(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_keep_nick.db"))
	target := fx.SeedUser(t, "bob", "secret12", "user")

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdateUserCommand{
		ID:       target.ID,
		Nickname: "bob",
		Email:    "bob+changed@example.com",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("expected success when nickname unchanged, got %v", err)
	}
}

func TestUpdateUserHandler_NotFound(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_notfound.db"))

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdateUserCommand{
		ID:       "00000000-0000-0000-0000-000000000000",
		Nickname: "bob",
		Email:    "bob@example.com",
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "id" {
		t.Fatalf("expected field=id, got %s", ve.Field)
	}
}

func TestUpdateUserCommand_RequiredPermission(t *testing.T) {
	if got := (UpdateUserCommand{}).RequiredPermission(); got != "admin:users:update" {
		t.Fatalf("expected admin:users:update, got %q", got)
	}
}

// Mirror of DeleteUserHandler's self-lockout guard: an admin must not be
// able to demote themselves out of the admin role and lock the org out
// of admin operations. Self-update of nickname/password/email remains OK.
func TestUpdateUserHandler_BlocksSelfDemote(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_self_demote.db"))
	admin := fx.SeedUser(t, "boss", "secret12", "admin")

	authedCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: admin.ID,
		Role:   admin.Role,
	})

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(authedCtx, UpdateUserCommand{
		ID:       admin.ID,
		Nickname: admin.Nickname,
		Email:    admin.Email,
		Role:     "user",
	})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "role" {
		t.Fatalf("expected field=role, got %s", ve.Field)
	}
}

// Counterpart: same admin updating non-role fields must still succeed.
func TestUpdateUserHandler_SelfUpdateKeepingRoleIsAllowed(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_self_keep.db"))
	admin := fx.SeedUser(t, "boss", "secret12", "admin")

	authedCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: admin.ID,
		Role:   admin.Role,
	})

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(authedCtx, UpdateUserCommand{
		ID:       admin.ID,
		Nickname: "boss-renamed",
		Email:    "boss@example.com",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("self-update of non-role fields must be allowed, got %v", err)
	}
}
