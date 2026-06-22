package query

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// Krok 4c — the platform plane proven end-to-end through the query bus, both
// directions of the boundary:
//   - a SUPERADMIN identity dispatching the platform user listing sees users from
//     EVERY tenant (the deliberate inverse of the Krok 4a isolation test);
//   - an ADMIN identity is DENIED at the bus (PermissionError) — the handler
//     never runs, so a tenant admin can never reach the cross-tenant view.
func TestListAllUsers_SuperadminSeesAllTenants_AdminDenied(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_users.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)

	_, queryBus, _ := fx.NewBuses()
	h := NewListAllUsersHandler(fx.Users)
	q := ListAllUsersQuery{}
	dispatch := func(ctx context.Context) ([]user.User, error) {
		return testfx.ExecQuery(ctx, queryBus, "PlatformListUsers", q,
			func(ctx context.Context) ([]user.User, error) { return h.Handle(ctx, q) })
	}

	// Superadmin → sees both tenants' users.
	superCtx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "s1", Role: "superadmin", Nickname: "root",
	})
	users, err := dispatch(superCtx)
	if err != nil {
		t.Fatalf("superadmin platform query must succeed: %v", err)
	}
	seen := map[string]bool{}
	for _, u := range users {
		seen[u.TenantID] = true
	}
	if len(users) != 2 || !seen[tenantA.ID] || !seen[tenantB.ID] {
		t.Fatalf("superadmin view must span both tenants, got %d users across %v",
			len(users), seen)
	}

	// Admin → denied at the bus, handler never runs.
	adminCtx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "a1", Role: "admin", Nickname: "tenant-admin", TenantID: tenantA.ID,
	})
	if _, err := dispatch(adminCtx); err == nil {
		t.Fatal("admin must be denied the platform query")
	} else {
		var pe *shared.PermissionError
		if !errors.As(err, &pe) {
			t.Fatalf("expected *shared.PermissionError for admin, got %T: %v", err, err)
		}
	}
}
