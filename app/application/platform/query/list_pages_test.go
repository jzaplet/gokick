package query

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func superCtx() context.Context {
	return shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "s1", Role: "superadmin", Nickname: "root",
	})
}

// The platform users grid: tenant-name filter narrows page AND total, an
// unrecognised sort falls back to NICKNAME (what the grid header renders as its
// default), and the last_login sort (the julianday case) executes without error
// even when nobody ever logged in.
//
// The fixture is deliberately CROSSED — the tenant sorting first holds the user
// sorting last — so nickname order and tenant order disagree. An aligned fixture
// (alice@acme, bob@beta) makes both fallbacks produce the same rows, which is how
// this test previously kept asserting a tenant-name fallback that no longer
// existed: it passed either way and pinned nothing.
func TestListAllUsers_GridCriteria(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_users_grid.db"))
	tenantA := fx.SeedTenant(t, "acme")
	tenantB := fx.SeedTenant(t, "beta")
	fx.SeedUserInTenant(t, "zoe", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "alice", "user", tenantB.ID)

	h := NewListAllUsersHandler(fx.PlatformUsers)

	page, err := h.Handle(superCtx(), ListAllUsersQuery{Tenant: "acm"})
	if err != nil {
		t.Fatalf("tenant filter: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Nickname != "zoe" {
		t.Fatalf("tenant filter: got total %d, items %d", page.Total, len(page.Items))
	}

	page, err = h.Handle(superCtx(), ListAllUsersQuery{SortBy: "last_login", SortDir: "DESC"})
	if err != nil {
		t.Fatalf("last_login sort: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("last_login sort: got total %d want 2", page.Total)
	}

	// alice is in "beta" and zoe in "acme", so this distinguishes the two
	// fallbacks: nickname ASC leads with alice, tenant-name ASC would lead with zoe.
	page, err = h.Handle(superCtx(), ListAllUsersQuery{SortBy: "drop table users"})
	if err != nil {
		t.Fatalf("hostile sort: %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].Nickname != "alice" {
		t.Fatalf("hostile sort must fall back to nickname order, got %+v", page.Items)
	}
}

// The platform tenants grid: name filter + user-count sort + paging.
func TestListTenants_GridCriteria(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_tenants_grid.db"))
	tenantA := fx.SeedTenant(t, "acme")
	fx.SeedTenant(t, "beta")
	fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "amy", "user", tenantA.ID)

	h := NewListTenantsHandler(fx.PlatformTenants)

	page, err := h.Handle(superCtx(), ListTenantsQuery{SortBy: "users", SortDir: "DESC"})
	if err != nil {
		t.Fatalf("users sort: %v", err)
	}
	// testfx databases carry the migration-seeded Default tenant too.
	if page.Total != 3 || page.Items[0].Name != "acme" || page.Items[0].UserCount != 2 {
		t.Fatalf("users sort: got %+v", page.Items)
	}

	page, err = h.Handle(superCtx(), ListTenantsQuery{Name: "bet"})
	if err != nil {
		t.Fatalf("name filter: %v", err)
	}
	if page.Total != 1 || page.Items[0].Name != "beta" {
		t.Fatalf("name filter: got %+v", page.Items)
	}

	// Plan filter is an exact match — flip acme to a paid tier and expect only
	// acme back (page AND total).
	if _, err = fx.DB.DB().Exec(
		`UPDATE tenants SET plan = 'pro' WHERE id = ?`, tenantA.ID,
	); err != nil {
		t.Fatalf("flip plan: %v", err)
	}
	page, err = h.Handle(superCtx(), ListTenantsQuery{Plan: "pro"})
	if err != nil {
		t.Fatalf("plan filter: %v", err)
	}
	if page.Total != 1 || page.Items[0].Name != "acme" || page.Items[0].Plan != "pro" {
		t.Fatalf("plan filter: got %+v", page.Items)
	}

	// Name ASC orders Default (capital D) before acme and beta — page 2 of
	// size 1 is therefore acme.
	page, err = h.Handle(superCtx(), ListTenantsQuery{Page: 2, PerPage: 1})
	if err != nil {
		t.Fatalf("paging: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].Name != "acme" {
		t.Fatalf("paging: got %+v", page.Items)
	}
}
