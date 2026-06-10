package query

import "testing"

// TestGetAdminDashboardQuery_RequiredPermission pins the permission that gates
// the admin dashboard. The admin route (GET /api/v1/dashboard/admin) maps to
// GetAdminDashboardQuery, and AuthorizeMiddleware enforces this exact string.
// It MUST be the admin-scoped permission — weakening it to the plain
// "dashboard:read" (or any other value) would let non-admin users reach the
// admin dashboard, so this assertion guards a real authorization boundary.
func TestGetAdminDashboardQuery_RequiredPermission(t *testing.T) {
	if got := (GetAdminDashboardQuery{}).RequiredPermission(); got != "admin:dashboard:read" {
		t.Fatalf("expected admin:dashboard:read, got %q", got)
	}
}

// TestGetUserDashboardQuery_RequiredPermission pins the permission that gates
// the user dashboard (GET /api/v1/dashboard/user -> GetUserDashboardQuery).
// The string must stay "dashboard:read": widening it to an admin-scoped value
// would lock ordinary users out of their own dashboard, and any typo here
// silently changes which permission AuthorizeMiddleware enforces on the route.
func TestGetUserDashboardQuery_RequiredPermission(t *testing.T) {
	if got := (GetUserDashboardQuery{}).RequiredPermission(); got != "dashboard:read" {
		t.Fatalf("expected dashboard:read, got %q", got)
	}
}
