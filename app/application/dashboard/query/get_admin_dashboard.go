package query

import (
	"context"

	"gokick/app/domain/shared"
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

// Handle reads both counts off the grid read (FindPage with PerPage 1 — the
// Total is what we're after); a dedicated COUNT method would be a second
// code path to keep tenant-scoped for the same numbers.
func (h *GetAdminDashboardHandler) Handle(
	ctx context.Context,
	_ GetAdminDashboardQuery,
) (AdminDashboard, error) {
	base := user.ListCriteria{
		PerPage: 1,
		Sort:    user.SortColumnFrom(""),
		SortDir: shared.SortDirectionFrom(""),
	}

	all, err := h.users.FindPage(ctx, base.Normalize())
	if err != nil {
		return AdminDashboard{}, err
	}

	activeOnly := base
	activeOnly.Filters = user.ListFilters{Active: "1"}
	active, err := h.users.FindPage(ctx, activeOnly.Normalize())
	if err != nil {
		return AdminDashboard{}, err
	}

	return AdminDashboard{UsersActive: active.Total, UsersTotal: all.Total}, nil
}
