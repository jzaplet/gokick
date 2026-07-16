package query

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// ListAllUsersQuery lists users across ALL tenants — the superadmin platform
// grid, the deliberate inverse of the tenant-scoped admin listing. Wire values
// arrive raw; Handle whitelists sort (tenant/nickname/email/role/last_login)
// and clamps paging, same fallback discipline as the admin grid.
// platform:overview is superadmin-only (an admin is denied; see
// shared.IsPermissionAllowedForRole).
type ListAllUsersQuery struct {
	Page     int
	PerPage  int
	SortBy   string
	SortDir  string
	Nickname string
	Email    string
	Role     string
	Active   string
	Tenant   string
}

func (ListAllUsersQuery) RequiredPermission() string { return "platform:overview" }

type ListAllUsersHandler struct {
	users user.PlatformRepository
}

func NewListAllUsersHandler(users user.PlatformRepository) *ListAllUsersHandler {
	return &ListAllUsersHandler{users: users}
}

func (h *ListAllUsersHandler) Handle(
	ctx context.Context,
	q ListAllUsersQuery,
) (user.PlatformListPage, error) {
	criteria := user.PlatformListCriteria{
		Page:    q.Page,
		PerPage: q.PerPage,
		Sort:    user.PlatformSortColumnFrom(q.SortBy),
		SortDir: shared.SortDirectionFrom(q.SortDir),
		Filters: user.PlatformListFilters{
			ListFilters: user.ListFilters{
				Nickname: q.Nickname,
				Email:    q.Email,
				Role:     q.Role,
				Active:   q.Active,
			},
			Tenant: q.Tenant,
		},
	}.Normalize()

	return h.users.FindPageAcrossTenants(ctx, criteria)
}
