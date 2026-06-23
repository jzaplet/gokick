package tenant_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestTenantRepository_SaveAndFindByID(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_repo.db"))

	tn := fx.SeedTenant(t, "Acme")

	got, err := fx.Tenants.FindByID(ctx, tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Name != "Acme" {
		t.Fatalf("round-trip failed: got %+v", got)
	}
}

func TestTenantRepository_FindByID_NotFound_ReturnsNilNil(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_notfound.db"))

	got, err := fx.Tenants.FindByID(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing tenant must return nil, got %+v", got)
	}
}

// The bootstrap "Default" tenant created by migration is findable via the repo;
// its id is shared.DefaultTenantID.
func TestTenantRepository_BootstrapDefaultFound(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_bootstrap.db"))

	got, err := fx.Tenants.FindByID(context.Background(), shared.DefaultTenantID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Name != "Default" {
		t.Fatalf("bootstrap default tenant must be findable, got %+v", got)
	}
}
