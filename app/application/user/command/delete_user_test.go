package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func authedCtx(userID, role string) context.Context {
	return shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: userID,
		Role:   role,
	})
}

func TestDeleteUserHandler_Success(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "delete_success.db"))
	admin := fx.SeedUser(t, "admin", "secret12", "admin")
	target := fx.SeedUser(t, "bob", "secret12", "user")

	ctx := authedCtx(admin.ID, "admin")
	h := NewDeleteUserHandler(fx.Users)
	if err := h.Handle(ctx, DeleteUserCommand{ID: target.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	gone, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if gone != nil {
		t.Fatal("expected user removed from DB")
	}
}

func TestDeleteUserHandler_CannotDeleteSelf(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "delete_self.db"))
	admin := fx.SeedUser(t, "admin", "secret12", "admin")

	ctx := authedCtx(admin.ID, "admin")
	h := NewDeleteUserHandler(fx.Users)
	err := h.Handle(ctx, DeleteUserCommand{ID: admin.ID})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}

	stillThere, err := fx.Users.FindByNickname(ctx, "admin")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if stillThere == nil {
		t.Fatal("admin must NOT have been deleted")
	}
}

func TestDeleteUserHandler_NotFound(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "delete_notfound.db"))
	admin := fx.SeedUser(t, "admin", "secret12", "admin")

	ctx := authedCtx(admin.ID, "admin")
	h := NewDeleteUserHandler(fx.Users)
	err := h.Handle(ctx, DeleteUserCommand{ID: "00000000-0000-0000-0000-000000000000"})

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T", err)
	}
	if ve.Field != "id" {
		t.Fatalf("expected field=id, got %s", ve.Field)
	}
}

func TestDeleteUserHandler_RequiresAuth(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "delete_noauth.db"))
	target := fx.SeedUser(t, "bob", "secret12", "user")

	h := NewDeleteUserHandler(fx.Users)
	err := h.Handle(context.Background(), DeleteUserCommand{ID: target.ID})

	var ae *shared.AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *shared.AuthError, got %T", err)
	}
}

func TestDeleteUserCommand_RequiredPermission(t *testing.T) {
	if got := (DeleteUserCommand{}).RequiredPermission(); got != "admin:users:delete" {
		t.Fatalf("expected admin:users:delete, got %q", got)
	}
}
