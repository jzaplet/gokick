package query

import (
	"context"

	"gokick/app/domain/user"
)

type GetAdminDashboardQuery struct{}

func (GetAdminDashboardQuery) RequiredPermission() string { return "admin:dashboard:read" }

// AdminDashboard carries the tenant's user stats for the stat cards (active
// count big, total small — aibobr parity). It replaced the placeholder
// welcome message.
type AdminDashboard struct {
	UsersActive int
	UsersTotal  int
}

type GetAdminDashboardHandler struct {
	users user.Repository
}

func NewGetAdminDashboardHandler(users user.Repository) *GetAdminDashboardHandler {
	return &GetAdminDashboardHandler{users: users}
}

// Handle reads both counts in a single tenant-scoped aggregate (CountByActive)
// — the platform dashboard already took this dedicated-count altitude
// (CountAcrossTenants); the admin dashboard now mirrors it instead of running
// two grid page reads that hydrate and discard a full user row each.
func (h *GetAdminDashboardHandler) Handle(
	ctx context.Context,
	_ GetAdminDashboardQuery,
) (AdminDashboard, error) {
	total, active, err := h.users.CountByActive(ctx)
	if err != nil {
		return AdminDashboard{}, err
	}

	return AdminDashboard{UsersActive: active, UsersTotal: total}, nil
}
