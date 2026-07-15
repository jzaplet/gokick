package query

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
)

// ListTenantsQuery lists tenants with their user counts — the superadmin
// platform grid. Same wire discipline as the user grids: raw values in,
// whitelisted sort (name/users) + clamped paging out.
// platform:overview is superadmin-only.
type ListTenantsQuery struct {
	Page    int
	PerPage int
	SortBy  string
	SortDir string
	Name    string
}

func (ListTenantsQuery) RequiredPermission() string { return "platform:overview" }

type ListTenantsHandler struct {
	tenants tenant.PlatformRepository
}

func NewListTenantsHandler(tenants tenant.PlatformRepository) *ListTenantsHandler {
	return &ListTenantsHandler{tenants: tenants}
}

func (h *ListTenantsHandler) Handle(
	ctx context.Context,
	q ListTenantsQuery,
) (tenant.ListPage, error) {
	criteria := tenant.ListCriteria{
		Page:    q.Page,
		PerPage: q.PerPage,
		Sort:    tenant.SortColumnFrom(q.SortBy),
		SortDir: shared.SortDirectionFrom(q.SortDir),
		Filters: tenant.ListFilters{Name: q.Name},
	}.Normalize()

	return h.tenants.OverviewPage(ctx, criteria)
}
