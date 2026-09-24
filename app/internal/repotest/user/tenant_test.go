package user_test

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// The migration creates the bootstrap "Default" tenant so every user has a real
// tenant to reference (and the FK has a target).
func TestMigration_BootstrapDefaultTenantExists(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t)

	tn, err := fx.Tenants.FindByID(ctx, shared.DefaultTenantID)
	if err != nil || tn == nil {
		t.Fatalf("bootstrap tenant must exist: %v / %v", tn, err)
	}
	if tn.Name != "Default" {
		t.Fatalf("bootstrap tenant name = %q, want %q", tn.Name, "Default")
	}
}

// A saved user is stamped with the default tenant — the NOT NULL
// DEFAULT column supplies it on insert and SELECT * reads it back. Proves
// single-tenant users all belong to the bootstrap tenant.
func TestUserSave_StampsDefaultTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t)

	u := fx.SeedUser(t, "alice", "secret12", "user")

	got, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.TenantID != shared.DefaultTenantID {
		t.Fatalf("user tenant_id = %q, want default %q", got.TenantID, shared.DefaultTenantID)
	}
}
