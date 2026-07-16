package user_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// FindPageAcrossTenants is the inverse of FindPage: it must return users from
// EVERY tenant, ignoring any tenant in context. The 4a isolation test proves
// FindPage hides other tenants; this proves the platform read deliberately
// doesn't, and joins the tenant NAME onto each row.
func TestUserRepository_FindPageAcrossTenants_SeesAllTenants(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_all_users.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "anna", "user", tenantA.ID)
	fx.SeedUserInTenant(t, "bob", "admin", tenantB.ID)

	// A tenant-A context must NOT scope this read — it spans all tenants.
	ctxA := shared.ContextWithTenantID(context.Background(), tenantA.ID)
	page, err := fx.PlatformUsers.FindPageAcrossTenants(ctxA, user.PlatformListCriteria{
		Page: 1, PerPage: 100, Sort: user.SortByTenant, SortDir: shared.SortAsc,
	}.Normalize())
	if err != nil {
		t.Fatalf("FindPageAcrossTenants: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("platform read must see all 3 users across tenants, got total %d / %d rows",
			page.Total, len(page.Items))
	}
	// Each row must carry its tenant NAME (the JOIN), not just the id.
	names := map[string]string{}
	for _, u := range page.Items {
		names[u.TenantID] = u.TenantName
	}
	if names[tenantA.ID] != "Acme" || names[tenantB.ID] != "Globex" {
		t.Fatalf("platform read must span both tenants with names, got %v", names)
	}
}

// RecordLogin stamps last_login_at (NULL until first login). Login.go
// relies on this for the platform overview, so prove the write lands.
func TestUserRepository_RecordLogin_StampsLastLoginAt(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "record_login.db"))

	u := fx.SeedUserInTenant(t, "carol", "user", shared.DefaultTenantID)

	before, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}
	if before.LastLoginAt != nil {
		t.Fatal("last_login_at must be NULL before any login")
	}

	if err := fx.Users.RecordLogin(ctx, u.ID); err != nil {
		t.Fatalf("RecordLogin: %v", err)
	}

	after, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if after.LastLoginAt == nil {
		t.Fatal("last_login_at must be set after RecordLogin")
	}
}
