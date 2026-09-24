package query

import (
	"context"
	"testing"

	"gokick/app/internal/testfx"
)

// The admin dashboard stats come off the tenant-scoped grid read: total counts
// every non-superadmin user in the tenant, active only the non-deactivated
// ones. Flipping one user inactive must move exactly the active count.
func TestGetAdminDashboard_UserStats(t *testing.T) {
	fx := testfx.New(t)
	fx.SeedUser(t, "root", "pwd", "admin")
	alice := fx.SeedUser(t, "alice", "pwd", "user")
	fx.SeedUser(t, "bob", "pwd", "user")

	fx.SetUserActive(t, alice.ID, false)

	h := NewGetAdminDashboardHandler(fx.Users)

	stats, err := h.Handle(context.Background(), GetAdminDashboardQuery{})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if stats.UsersTotal != 3 || stats.UsersActive != 2 {
		t.Fatalf("want total 3 / active 2, got %+v", stats)
	}
}
