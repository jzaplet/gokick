package query

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/internal/testfx"
)

// The admin dashboard stats come off the tenant-scoped grid read: total counts
// every non-superadmin user in the tenant, active only the non-deactivated
// ones. Flipping one user inactive must move exactly the active count.
func TestGetAdminDashboard_UserStats(t *testing.T) {
	fx := testfx.New(t, filepath.Join(t.TempDir(), "admin_dashboard.db"))
	fx.SeedUser(t, "root", "pwd", "admin")
	alice := fx.SeedUser(t, "alice", "pwd", "user")
	fx.SeedUser(t, "bob", "pwd", "user")

	if _, err := fx.DB.DB().Exec(
		`UPDATE users SET active = 0 WHERE id = ?`, alice.ID,
	); err != nil {
		t.Fatalf("deactivate alice: %v", err)
	}

	h := NewGetAdminDashboardHandler(fx.Users)

	stats, err := h.Handle(context.Background(), GetAdminDashboardQuery{})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if stats.UsersTotal != 3 || stats.UsersActive != 2 {
		t.Fatalf("want total 3 / active 2, got %+v", stats)
	}
}
