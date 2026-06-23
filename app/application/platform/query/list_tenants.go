package query

import (
	"context"

	"gokick/app/domain/tenant"
)

// ListTenantsQuery lists every tenant with its user count — the superadmin
// platform overview. platform:overview is superadmin-only.
type ListTenantsQuery struct{}

func (ListTenantsQuery) RequiredPermission() string { return "platform:overview" }

type ListTenantsHandler struct {
	tenants tenant.Repository
}

func NewListTenantsHandler(tenants tenant.Repository) *ListTenantsHandler {
	return &ListTenantsHandler{tenants: tenants}
}

func (h *ListTenantsHandler) Handle(
	ctx context.Context,
	_ ListTenantsQuery,
) ([]tenant.Overview, error) {
	return h.tenants.FindAllWithUserCount(ctx)
}
