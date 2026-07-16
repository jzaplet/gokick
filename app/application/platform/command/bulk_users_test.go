package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

func superadminCtx(id string) context.Context {
	return shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: id, Role: "superadmin", Nickname: "root",
	})
}

// allPlatformRows reads every user across tenants (paged read, big page) for
// the count-based assertions after a bulk op.
func allPlatformRows(t *testing.T, fx *testfx.Fixture) []user.PlatformRow {
	t.Helper()
	page, err := fx.PlatformUsers.FindPageAcrossTenants(
		context.Background(),
		user.PlatformListCriteria{
			Page: 1, PerPage: 1000, Sort: user.SortByTenant, SortDir: shared.SortAsc,
		}.Normalize(),
	)
	if err != nil {
		t.Fatalf("read all platform rows: %v", err)
	}
	return page.Items
}

// Cross-tenant bulk delete by the tenant-name filter: only the matching
// tenant's users go; superadmin rows survive even when the filters match.
func TestBulkDeletePlatformUsers_AllFilteredByTenant(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_bulk.db"))
	tenantA := fx.SeedTenant(t, "acme")
	tenantB := fx.SeedTenant(t, "beta")
	fx.SeedUserInTenant(t, "alice", "user", tenantA.ID)
	fx.SeedUserInTenant(t, "amy", "admin", tenantA.ID)
	bob := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)
	root := fx.SeedUser(t, "root", "pwd", "superadmin")

	h := NewBulkDeletePlatformUsersHandler(fx.PlatformUsers)
	_, err := h.Handle(superadminCtx(root.ID), BulkDeletePlatformUsersCommand{
		AllFiltered: true,
		Tenant:      "acm",
	})
	if err != nil {
		t.Fatalf("bulk delete: %v", err)
	}

	rows := allPlatformRows(t, fx)
	if len(rows) != 2 {
		t.Fatalf("want bob + root to survive, got %d rows", len(rows))
	}
	for _, row := range rows {
		if row.TenantName == "acme" && row.Role != "superadmin" {
			t.Fatalf("acme user %q must be deleted", row.Nickname)
		}
	}
	_ = bob
}

func TestBulkSetPlatformUsersActive_ByIDs(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_bulk_active.db"))
	tenantA := fx.SeedTenant(t, "acme")
	alice := fx.SeedUserInTenant(t, "alice", "user", tenantA.ID)
	root := fx.SeedUser(t, "root", "pwd", "superadmin")

	h := NewBulkSetPlatformUsersActiveHandler(fx.PlatformUsers)
	_, err := h.Handle(superadminCtx(root.ID), BulkSetPlatformUsersActiveCommand{
		IDs:       []string{alice.ID, root.ID},
		SetActive: false,
	})
	if err != nil {
		t.Fatalf("bulk deactivate: %v", err)
	}

	got, err := fx.PlatformUsers.FindByID(context.Background(), alice.ID)
	if err != nil || got == nil {
		t.Fatalf("find alice: %v", err)
	}
	if got.Active != false {
		t.Fatal("alice must be deactivated")
	}

	rootRow, err := fx.PlatformUsers.FindByID(context.Background(), root.ID)
	if err != nil || rootRow == nil {
		t.Fatalf("find root: %v", err)
	}
	if rootRow.Active != true {
		t.Fatal("a superadmin row must never be touched by bulk operations")
	}
}

func TestBulkDeletePlatformUsers_EmptySelectionIsValidationError(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "platform_bulk_empty.db"))
	root := fx.SeedUser(t, "root", "pwd", "superadmin")

	h := NewBulkDeletePlatformUsersHandler(fx.PlatformUsers)
	_, err := h.Handle(superadminCtx(root.ID), BulkDeletePlatformUsersCommand{})

	var verr *shared.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError for an empty selection, got %v", err)
	}
}
