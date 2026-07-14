package user_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// F-083: the admin read-one (GET /admin/users/{id}) is backed by FindScopedByID,
// which MUST scope by tenant like FindAll — NOT reuse the tenant-exempt FindByID.
// This is the regression guard the advisor called out: it fails the instant
// FindByID is wired in place of FindScopedByID, because a tenant-A admin would
// then be able to read a tenant-B user by id — the cross-tenant leak the finding
// exists to close.
func TestUserRepository_FindScopedByID_IsTenantScoped(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "read_one_scoped.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	alice := fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	bob := fx.SeedUserInTenant(t, "bob", "admin", tenantB.ID)

	ctxA := shared.ContextWithTenantID(context.Background(), tenantA.ID)

	// Happy path: a tenant-A admin reads its own tenant's user.
	got, err := fx.Users.FindScopedByID(ctxA, alice.ID)
	if err != nil {
		t.Fatalf("FindScopedByID(alice): %v", err)
	}
	if got == nil || got.Nickname != "alice" {
		t.Fatalf("tenant-A admin must read alice, got %+v", got)
	}

	// The guard: bob lives in tenant B, so a tenant-A read must find nothing.
	// FindByID (tenant-exempt) would return bob here — that's the leak.
	cross, err := fx.Users.FindScopedByID(ctxA, bob.ID)
	if err != nil {
		t.Fatalf("FindScopedByID(bob) cross-tenant: %v", err)
	}
	if cross != nil {
		t.Fatalf("cross-tenant leak: tenant-A read returned tenant-B user %q", cross.Nickname)
	}
}

// A superadmin row is excluded from the admin read-one even in the caller's own
// tenant — the same escalation guard FindAll/Update/Delete carry (role !=
// 'superadmin'). A tenant admin must never load a platform account by id.
func TestUserRepository_FindScopedByID_ExcludesSuperadmin(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "read_one_superadmin.db"))

	super := fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)
	alice := fx.SeedUserInTenant(t, "alice", "admin", shared.DefaultTenantID)
	ctx := shared.ContextWithTenantID(context.Background(), shared.DefaultTenantID)

	got, err := fx.Users.FindScopedByID(ctx, super.ID)
	if err != nil {
		t.Fatalf("FindScopedByID(superadmin): %v", err)
	}
	if got != nil {
		t.Fatalf("admin read-one leaked a superadmin: %q", got.Nickname)
	}

	// Sanity: a regular same-tenant user IS readable, so the nil above is the
	// superadmin exclusion, not an empty database.
	ok, err := fx.Users.FindScopedByID(ctx, alice.ID)
	if err != nil || ok == nil || ok.Nickname != "alice" {
		t.Fatalf("regular same-tenant user must be readable, got %+v (err %v)", ok, err)
	}
}

// Not-found is (nil, nil) — the repository idiom the application handler turns
// into a 400. An unknown id must not error.
func TestUserRepository_FindScopedByID_NotFoundIsNilNil(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "read_one_missing.db"))
	ctx := shared.ContextWithTenantID(context.Background(), shared.DefaultTenantID)

	got, err := fx.Users.FindScopedByID(ctx, "01931234-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unknown id must not error: %v", err)
	}
	if got != nil {
		t.Fatalf("unknown id must return nil, got %+v", got)
	}
}

// F-083: the platform read-one (GET /platform/users/{id}) is the cross-tenant
// INVERSE — FindByIDAcrossTenants reads a user in ANY tenant, joined to its
// tenant name, regardless of the tenant in context.
func TestUserRepository_FindByIDAcrossTenants_ReadsAnyTenant(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "read_one_platform.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	bob := fx.SeedUserInTenant(t, "bob", "admin", tenantB.ID)

	// A tenant-A context must NOT scope this read — it spans all tenants.
	ctxA := shared.ContextWithTenantID(context.Background(), tenantA.ID)
	got, err := fx.PlatformUsers.FindByIDAcrossTenants(ctxA, bob.ID)
	if err != nil {
		t.Fatalf("FindByIDAcrossTenants(bob): %v", err)
	}
	if got == nil || got.Nickname != "bob" {
		t.Fatalf("platform read must find bob across tenants, got %+v", got)
	}
	if got.TenantName != "Globex" {
		t.Fatalf("platform read must carry the tenant name (JOIN), got %q", got.TenantName)
	}

	missing, err := fx.PlatformUsers.FindByIDAcrossTenants(
		ctxA,
		"01931234-0000-0000-0000-000000000000",
	)
	if err != nil {
		t.Fatalf("unknown id must not error: %v", err)
	}
	if missing != nil {
		t.Fatalf("unknown id must return nil, got %+v", missing)
	}
}
