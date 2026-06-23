package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// Privilege-escalation guard. A tenant admin (admin:users:*) must never
// be able to MINT a superadmin via CreateUser nor PROMOTE anyone to superadmin
// via UpdateUser — otherwise the admin self-escalates to the cross-tenant
// platform plane. NewRole stays permissive (the seeder needs it); the command
// layer is where the refusal lives. This is the inverse of the permission-check
// tests: it guards the role-ASSIGNMENT surface, not the role-CHECK surface.

func TestCreateUserHandler_RejectsSuperadminRole(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_super.db"))
	ctx, _ := shared.ContextWithEventCollector(context.Background())

	h := NewCreateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, CreateUserCommand{
		Nickname: "sneaky",
		Password: "secret12",
		Email:    "s@example.com",
		Role:     "superadmin",
	})
	if err == nil {
		t.Fatal("admin must not be able to create a superadmin")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected role ValidationError, got %T: %v", err, err)
	}
	if u, _ := fx.Users.FindByNickname(ctx, "sneaky"); u != nil {
		t.Fatal("no user must be persisted when the superadmin role is rejected")
	}
}

func TestUpdateUserHandler_RejectsPromotionToSuperadmin(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "update_super.db"))

	victim := fx.SeedUser(t, "bob", "password12", "user")

	h := NewUpdateUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdateUserCommand{
		ID:       victim.ID,
		Nickname: "bob",
		Email:    "bob@example.com",
		Role:     "superadmin",
	})
	if err == nil {
		t.Fatal("admin must not be able to promote a user to superadmin")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected role ValidationError, got %T: %v", err, err)
	}

	after, err := fx.Users.FindByID(ctx, victim.ID)
	if err != nil {
		t.Fatalf("load victim: %v", err)
	}
	if after.Role != "user" {
		t.Fatalf("victim role must stay 'user' after a rejected promotion, got %q", after.Role)
	}
}
