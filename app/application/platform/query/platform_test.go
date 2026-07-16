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

// The platform plane proven end-to-end through the query bus, both
// directions of the boundary:
//   - a SUPERADMIN identity dispatching the platform user listing sees users from
//     EVERY tenant (the deliberate inverse of the tenant isolation test);
//   - an ADMIN identity is DENIED at the bus (PermissionError) — the handler
//     never runs, so a tenant admin can never reach the cross-tenant view.
func TestListAllUsers_SuperadminSeesAllTenants_AdminDenied(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_users.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)

	_, queryBus, _ := fx.NewBuses()
	h := NewListAllUsersHandler(fx.PlatformUsers)
	q := ListAllUsersQuery{}
	dispatch := func(ctx context.Context) ([]user.PlatformRow, error) {
		page, err := testfx.ExecQuery(ctx, queryBus, "PlatformListUsers", q,
			func(ctx context.Context) (user.PlatformListPage, error) { return h.Handle(ctx, q) })
		return page.Items, err
	}

	// Superadmin → sees both tenants' users, each carrying its tenant NAME.
	superCtx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "s1", Role: "superadmin", Nickname: "root",
	})
	users, err := dispatch(superCtx)
	if err != nil {
		t.Fatalf("superadmin platform query must succeed: %v", err)
	}
	names := map[string]string{}
	for _, u := range users {
		names[u.TenantID] = u.TenantName
	}
	if len(users) != 2 || names[tenantA.ID] != "Acme" || names[tenantB.ID] != "Globex" {
		t.Fatalf("superadmin view must span both tenants with names, got %d users: %v",
			len(users), names)
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

// The dashboard stats count tenants and users; the user count includes all
// tenants (and matches what the platform user list shows).
func TestGetStats_CountsTenantsAndUsers(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_stats.db"))

	// Bootstrap "Default" tenant exists from migration; add two more → 3 tenants.
	fx.SeedTenant(t, "Acme")
	fx.SeedTenant(t, "Globex")
	fx.SeedUserInTenant(t, "alice", "admin", shared.DefaultTenantID)
	fx.SeedUserInTenant(t, "bob", "user", shared.DefaultTenantID)
	fx.SeedUserInTenant(t, "root", "superadmin", shared.DefaultTenantID)

	stats, err := NewGetStatsHandler(
		fx.PlatformTenants,
		fx.PlatformUsers,
	).Handle(ctx, GetStatsQuery{})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TenantCount != 3 {
		t.Fatalf("tenant count: got %d want 3", stats.TenantCount)
	}
	// The user count includes the superadmin (3 = alice + bob + root) — it must
	// match what the platform user list shows, or the dashboard and table disagree.
	if stats.UserCount != 3 {
		t.Fatalf("user count: got %d want 3 (must include the superadmin)", stats.UserCount)
	}

	page, err := fx.PlatformUsers.FindPageAcrossTenants(ctx, user.PlatformListCriteria{
		Page: 1, PerPage: 1000, Sort: user.SortByTenant, SortDir: shared.SortAsc,
	}.Normalize())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	rows := page.Items
	if len(rows) != stats.UserCount {
		t.Fatalf("user_count (%d) must match the platform list length (%d)",
			stats.UserCount, len(rows))
	}
	hasSuper := false
	for _, r := range rows {
		if r.Role == "superadmin" {
			hasSuper = true
		}
	}
	if !hasSuper {
		t.Fatal("the platform list must include the superadmin (the count includes it)")
	}
}
