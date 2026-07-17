package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// The rule the whole tenant-delete feature stands on: a tenant that still owns
// users survives. The UI disables the button, but the button is a hint over a
// count that was stale the moment it rendered — this is the gate that actually
// holds.
func TestDeletePlatformTenant_RefusesTenantWithUsers(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_busy.db"))

	busy := fx.SeedTenant(t, "Beta")
	fx.SeedUserInTenant(t, "bob", "user", busy.ID)

	h := NewDeletePlatformTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeletePlatformTenantCommand{ID: busy.ID})
	if err == nil {
		t.Fatal("deleting a tenant that still has users must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError (400), got %T: %v", err, err)
	}

	// The refusal must be a refusal, not a message over a completed delete.
	got, err := fx.Tenants.FindByID(ctx, busy.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the tenant must still exist after a refused delete")
	}
}

func TestDeletePlatformTenant_DeletesEmptyTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_empty.db"))

	empty := fx.SeedTenant(t, "Ghost")

	h := NewDeletePlatformTenantHandler(fx.PlatformTenants)
	if err := h.Handle(ctx, DeletePlatformTenantCommand{ID: empty.ID}); err != nil {
		t.Fatalf("an empty tenant must be deletable: %v", err)
	}

	got, err := fx.Tenants.FindByID(ctx, empty.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatal("the tenant must be gone")
	}
}

// The default tenant is refused by IDENTITY, not merely because it usually holds
// a superadmin. Single-tenant mode puts every user in it and runs.tenant_id
// DEFAULTs to its id, so deleting it would strand rows — the guard must hold even
// on the empty tenant this test constructs, where the user-count rule would not.
func TestDeletePlatformTenant_RefusesDefaultTenantEvenWhenEmpty(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_default.db"))

	// No users seeded: the default tenant is empty here, so only the identity
	// guard can save it.
	h := NewDeletePlatformTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeletePlatformTenantCommand{ID: shared.DefaultTenantID})
	if err == nil {
		t.Fatal("the default tenant must never be deletable")
	}

	got, err := fx.Tenants.FindByID(ctx, shared.DefaultTenantID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the default tenant must survive")
	}
}

func TestDeletePlatformTenant_UnknownTenantIsNotFound(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_delete_404.db"))

	h := NewDeletePlatformTenantHandler(fx.PlatformTenants)
	err := h.Handle(ctx, DeletePlatformTenantCommand{ID: "01920000-0000-7000-8000-000000000000"})
	if err == nil {
		t.Fatal("an unknown tenant must be reported, not silently succeed")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "id" {
		t.Fatalf("expected *shared.ValidationError{Field:\"id\"}, got %T: %v", err, err)
	}
}

// Bulk is partial by design: the empty tenants in the selection go, the ones that
// still have users stay, and `affected` counts only what actually happened. A
// selection mixing both must not be all-or-nothing in either direction.
func TestBulkDeletePlatformTenants_DeletesOnlyTheEmptyOnes(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_mixed.db"))

	empty1 := fx.SeedTenant(t, "Ghost One")
	empty2 := fx.SeedTenant(t, "Ghost Two")
	busy := fx.SeedTenant(t, "Busy")
	fx.SeedUserInTenant(t, "bob", "user", busy.ID)

	h := NewBulkDeletePlatformTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeletePlatformTenantsCommand{
		IDs: []string{empty1.ID, empty2.ID, busy.ID},
	})
	if err != nil {
		t.Fatalf("a mixed selection must not error: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected must count only the deleted tenants, got %d want 2", affected)
	}

	for _, id := range []string{empty1.ID, empty2.ID} {
		got, _ := fx.Tenants.FindByID(ctx, id)
		if got != nil {
			t.Fatalf("empty tenant %q must be gone", id)
		}
	}
	if got, _ := fx.Tenants.FindByID(ctx, busy.ID); got == nil {
		t.Fatal("the tenant with users must survive a bulk delete")
	}
}

// The default tenant is spared in bulk too — including all-filtered mode, where
// nobody enumerated an id and the statement is the only thing standing between a
// broad selection and the tenant the whole single-tenant mode rests on.
func TestBulkDeletePlatformTenants_SparesDefaultTenantWhenAllFiltered(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_default.db"))

	victim := fx.SeedTenant(t, "Ghost")

	h := NewBulkDeletePlatformTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeletePlatformTenantsCommand{AllFiltered: true})
	if err != nil {
		t.Fatalf("all-filtered bulk delete: %v", err)
	}

	if got, _ := fx.Tenants.FindByID(ctx, shared.DefaultTenantID); got == nil {
		t.Fatal("the default tenant must survive an unfiltered bulk delete")
	}
	if got, _ := fx.Tenants.FindByID(ctx, victim.ID); got != nil {
		t.Fatal("a selected empty tenant should still have been deleted")
	}
	if affected != 1 {
		t.Fatalf("only the one empty non-default tenant should go, got affected=%d", affected)
	}
}

// All-filtered mode must delete exactly the set the grid showed — the filters are
// what the superadmin actually saw, so a filter that fails to reach the statement
// silently widens the blast radius.
func TestBulkDeletePlatformTenants_AllFilteredHonoursTheNameFilter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_filter.db"))

	matching := fx.SeedTenant(t, "Ghost One")
	other := fx.SeedTenant(t, "Keep Me")

	h := NewBulkDeletePlatformTenantsHandler(fx.PlatformTenants)
	affected, err := h.Handle(ctx, BulkDeletePlatformTenantsCommand{
		AllFiltered: true,
		Name:        "Ghost",
	})
	if err != nil {
		t.Fatalf("filtered bulk delete: %v", err)
	}
	if affected != 1 {
		t.Fatalf("only the filtered tenant should go, got affected=%d", affected)
	}

	if got, _ := fx.Tenants.FindByID(ctx, matching.ID); got != nil {
		t.Fatal("the tenant matching the filter must be deleted")
	}
	if got, _ := fx.Tenants.FindByID(ctx, other.ID); got == nil {
		t.Fatal("a tenant OUTSIDE the filter must never be touched")
	}
}

func TestBulkDeletePlatformTenants_EmptySelectionIsRefused(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bulk_none.db"))

	h := NewBulkDeletePlatformTenantsHandler(fx.PlatformTenants)
	_, err := h.Handle(ctx, BulkDeletePlatformTenantsCommand{})
	if err == nil {
		t.Fatal("an empty selection must be refused, not treated as all-filtered")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
}
