package query

import (
	"context"

	"gokick/app/domain/user"
)

// ListAllUsersQuery lists every user across ALL tenants — the superadmin
// platform overview, the deliberate inverse of the tenant-scoped admin listing.
// platform:overview is superadmin-only (an admin is denied; see
// shared.IsPermissionAllowedForRole).
type ListAllUsersQuery struct{}

func (ListAllUsersQuery) RequiredPermission() string { return "platform:overview" }

type ListAllUsersHandler struct {
	users user.Repository
}

func NewListAllUsersHandler(users user.Repository) *ListAllUsersHandler {
	return &ListAllUsersHandler{users: users}
}

func (h *ListAllUsersHandler) Handle(
	ctx context.Context,
	_ ListAllUsersQuery,
) ([]user.PlatformRow, error) {
	return h.users.FindAllAcrossTenants(ctx)
}
