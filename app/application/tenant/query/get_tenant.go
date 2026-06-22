package query

import (
	"context"

	"gokick/app/domain/tenant"
)

// GetTenantQuery loads a tenant by id (or nil when none) — used by the CLI to
// validate a --tenant-id before creating a user in it.
type GetTenantQuery struct {
	ID string
}

func (GetTenantQuery) RequiredPermission() string { return "platform:tenants:read" }

type GetTenantHandler struct {
	tenants tenant.Repository
}

func NewGetTenantHandler(tenants tenant.Repository) *GetTenantHandler {
	return &GetTenantHandler{tenants: tenants}
}

func (h *GetTenantHandler) Handle(ctx context.Context, q GetTenantQuery) (*tenant.Tenant, error) {
	return h.tenants.FindByID(ctx, q.ID)
}
