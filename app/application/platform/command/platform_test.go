package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// A superadmin edits a user in ANOTHER tenant: the cross-tenant write lands, the
// tenant is NOT changed, and `active` is PRESERVED (the form carries no active
// flag, so the command must not deactivate the user — the advisor's regression).
func TestUpdatePlatformUser_CrossTenant_PreservesActiveAndTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update.db"))

	tenantB := fx.SeedTenant(t, "Beta")
	victim := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID) // active=true

	h := NewUpdatePlatformUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       victim.ID,
		Nickname: "bobby",
		Email:    "bobby@example.com",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("cross-tenant edit must land: %v", err)
	}

	got, err := fx.Users.FindByID(ctx, victim.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Nickname != "bobby" || got.Role != "admin" {
		t.Fatalf("edit must persist, got nick=%q role=%q", got.Nickname, got.Role)
	}
	if got.Active != true {
		t.Fatal("active must be preserved across a platform edit (form sends no active flag)")
	}
	if got.TenantID != tenantB.ID {
		t.Fatalf("edit must NOT move the user between tenants, got %q", got.TenantID)
	}
}

// Editing a SUPERADMIN target is rejected (it is managed out-of-band only).
func TestUpdatePlatformUser_RejectsSuperadminTarget(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update_super.db"))

	super := fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)

	h := NewUpdatePlatformUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       super.ID,
		Nickname: "hax",
		Email:    "h@x.com",
		Role:     "admin",
	})
	if err == nil {
		t.Fatal("editing a superadmin must be rejected")
	}
	var pe *shared.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *shared.PermissionError, got %T: %v", err, err)
	}

	got, _ := fx.Users.FindByID(ctx, super.ID)
	if got.Nickname != "root" || got.Role != "superadmin" {
		t.Fatal("superadmin must be untouched after a rejected edit")
	}
}

// Promoting anyone to superadmin via the platform edit is rejected.
func TestUpdatePlatformUser_RejectsPromotionToSuperadmin(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_update_promote.db"))

	victim := fx.SeedUserInTenant(t, "bob", "user", shared.DefaultTenantID)

	h := NewUpdatePlatformUserHandler(fx.Users, fx.Hasher)
	err := h.Handle(ctx, UpdatePlatformUserCommand{
		ID:       victim.ID,
		Nickname: "bob",
		Email:    "bob@example.com",
		Role:     "superadmin",
	})
	if err == nil {
		t.Fatal("promotion to superadmin via the platform edit must be rejected")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected role ValidationError, got %T: %v", err, err)
	}
}

// Cross-tenant delete lands for a regular user; a superadmin target is rejected.
func TestDeletePlatformUser_CrossTenantAndSuperadminGuard(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_delete.db"))

	tenantB := fx.SeedTenant(t, "Beta")
	victim := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)
	super := fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)

	// Caller is a (distinct) superadmin identity.
	callerCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{
		UserID: "caller-super", Role: "superadmin",
	})

	h := NewDeletePlatformUserHandler(fx.Users)

	// A superadmin target is rejected (managed out-of-band).
	if err := h.Handle(callerCtx, DeletePlatformUserCommand{ID: super.ID}); err == nil {
		t.Fatal("deleting a superadmin must be rejected")
	}
	if got, _ := fx.Users.FindByID(ctx, super.ID); got == nil {
		t.Fatal("superadmin must survive a rejected delete")
	}

	// A regular user in another tenant is deleted.
	if err := h.Handle(callerCtx, DeletePlatformUserCommand{ID: victim.ID}); err != nil {
		t.Fatalf("cross-tenant delete must land: %v", err)
	}
	if got, _ := fx.Users.FindByID(ctx, victim.ID); got != nil {
		t.Fatal("victim must be deleted cross-tenant")
	}
}
