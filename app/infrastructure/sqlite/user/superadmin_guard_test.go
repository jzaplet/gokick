package user_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// Defense-in-depth: the seeded superadmin lives in the default tenant,
// where a regular admin also lives. Since admin Update/Delete scope by tenant_id
// alone, without an extra guard a same-tenant admin could reset the superadmin's
// password (then log in as it) or delete it — a back-door escalation. The admin
// repo queries therefore also exclude role='superadmin': a tenant admin can
// neither SEE, MODIFY, nor DELETE a superadmin, even one sharing its tenant.
func TestUserRepository_AdminCannotTouchSuperadminInSameTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "superadmin_guard.db"))

	// A superadmin and a regular admin, both in the default tenant.
	super := fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)
	fx.SeedUserInTenant(t, "alice", "admin", shared.DefaultTenantID)
	originalHash := super.PasswordHash

	dctx := shared.ContextWithTenantID(ctx, shared.DefaultTenantID)

	// (1) The admin listing must NOT include the superadmin (but must include alice).
	all, err := fx.Users.FindAll(dctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	sawAlice := false
	for _, u := range all {
		if u.Role == "superadmin" {
			t.Fatal("admin user listing leaked a superadmin")
		}
		if u.Nickname == "alice" {
			sawAlice = true
		}
	}
	if !sawAlice {
		t.Fatal("admin listing must still include regular tenant users (alice)")
	}

	// (2) Update must be a no-op against a superadmin (password reset attempt).
	super.PasswordHash = "hijacked"
	super.Nickname = "pwned"
	if err := fx.Users.Update(dctx, super); err != nil {
		t.Fatalf("Update (expected no-op): %v", err)
	}

	// (3) Delete must be a no-op against a superadmin.
	if err := fx.Users.Delete(dctx, super.ID); err != nil {
		t.Fatalf("Delete (expected no-op): %v", err)
	}

	// The superadmin must survive intact — original hash, original nickname.
	got, err := fx.Users.FindByID(ctx, super.ID)
	if err != nil {
		t.Fatalf("superadmin must still exist after the no-op admin writes: %v", err)
	}
	if got.PasswordHash != originalHash {
		t.Fatal("a tenant admin reset the superadmin password — escalation back-door")
	}
	if got.Nickname != "root" {
		t.Fatalf("a tenant admin renamed the superadmin, got %q", got.Nickname)
	}
}
