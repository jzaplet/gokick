package user_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// The isolation proof. With two tenants each holding users, a query
// scoped to tenant A returns ONLY tenant A's users, never B's. Uses arbitrary
// tenants (not the default) so it proves real isolation rather than a tautology.
func TestUserRepository_FindAll_IsolatesByTenant(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_isolation.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "anna", "user", tenantA.ID)
	fx.SeedUserInTenant(t, "bob", "admin", tenantB.ID)

	ctxA := shared.ContextWithTenantID(context.Background(), tenantA.ID)
	usersA, err := fx.Users.FindAll(ctxA)
	if err != nil {
		t.Fatalf("FindAll(A): %v", err)
	}
	if len(usersA) != 2 {
		t.Fatalf("tenant A must see exactly its 2 users, got %d", len(usersA))
	}
	for _, u := range usersA {
		if u.TenantID != tenantA.ID {
			t.Fatalf("cross-tenant leak: %q belongs to %q, not tenant A", u.Nickname, u.TenantID)
		}
	}

	ctxB := shared.ContextWithTenantID(context.Background(), tenantB.ID)
	usersB, err := fx.Users.FindAll(ctxB)
	if err != nil {
		t.Fatalf("FindAll(B): %v", err)
	}
	if len(usersB) != 1 || usersB[0].Nickname != "bob" {
		t.Fatalf("tenant B must see exactly its 1 user (bob), got %d", len(usersB))
	}
}

// Scoped writes. An admin acting in tenant A cannot update or delete a
// user belonging to tenant B: the WHERE … AND tenant_id scope matches no row.
// Without this, a regressed Update/Delete (WHERE id only) would leak silently.
func TestUserRepository_UpdateDelete_IsolateByTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "write_isolation.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	bob := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)

	ctxA := shared.ContextWithTenantID(ctx, tenantA.ID)

	// A tenant-A admin tries to rename, then delete, bob (tenant B). Both scope to
	// tenant A, so they match no row — which now surfaces as a not-found error
	// (F-039), never a silent success — and must leave bob untouched.
	bob.Nickname = "hacked"
	var ve *shared.ValidationError
	if err := fx.Users.Update(ctxA, bob); !errors.As(err, &ve) {
		t.Fatalf("cross-tenant Update must error on 0 rows, got %T: %v", err, err)
	}
	if err := fx.Users.Delete(ctxA, bob.ID); !errors.As(err, &ve) {
		t.Fatalf("cross-tenant Delete must error on 0 rows, got %T: %v", err, err)
	}

	got, err := fx.Users.FindByID(ctx, bob.ID)
	if err != nil {
		t.Fatalf("bob must still exist — a tenant-A write must not affect him: %v", err)
	}
	if got.Nickname != "bob" {
		t.Fatalf("cross-tenant Update leaked: bob renamed to %q by a tenant-A admin", got.Nickname)
	}
}

// Fail-closed: with multitenancy ON, a query whose context carries no tenant
// panics rather than silently scoping to the default tenant (a cross-tenant
// leak). This is the guard the APP_MULTITENANCY flag exists for.
func TestUserRepository_FailsClosedOnMissingTenant(t *testing.T) {
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "fail_closed.db"))

	defer func() {
		if recover() == nil {
			t.Fatal("FindAll with no tenant in ctx must panic in multitenant mode")
		}
	}()
	_, _ = fx.Users.FindAll(context.Background())
}

// Fail-open: with multitenancy OFF (default), a query with no tenant in ctx
// falls back to the default tenant — no panic — so a single-tenant deployment
// runs unchanged.
func TestUserRepository_FailsOpenToDefaultTenant(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "fail_open.db"))
	fx.SeedUserInTenant(t, "solo", "admin", shared.DefaultTenantID)

	users, err := fx.Users.FindAll(context.Background())
	if err != nil {
		t.Fatalf("fail-open must not error: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("fail-open scopes to the default tenant, got %d users", len(users))
	}
}
