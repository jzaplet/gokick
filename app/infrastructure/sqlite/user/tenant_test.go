package user_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// The migration creates the bootstrap "Default" tenant so every user has a real
// tenant to reference (and the FK has a target).
func TestMigration_BootstrapDefaultTenantExists(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "bootstrap_tenant.db"))

	var name string
	err := fx.DB.DB().GetContext(ctx, &name,
		`SELECT name FROM tenants WHERE id = ?`, shared.DefaultTenantID)
	if err != nil {
		t.Fatalf("bootstrap tenant must exist: %v", err)
	}
	if name != "Default" {
		t.Fatalf("bootstrap tenant name = %q, want %q", name, "Default")
	}
}

// A saved user is stamped with the default tenant — the NOT NULL
// DEFAULT column supplies it on insert and SELECT * reads it back. Proves
// single-tenant users all belong to the bootstrap tenant.
func TestUserSave_StampsDefaultTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "user_tenant_stamp.db"))

	u := fx.SeedUser(t, "alice", "secret12", "user")

	got, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.TenantID != shared.DefaultTenantID {
		t.Fatalf("user tenant_id = %q, want default %q", got.TenantID, shared.DefaultTenantID)
	}
}
