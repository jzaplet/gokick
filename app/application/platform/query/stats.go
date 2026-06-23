package query

import (
	"context"

	"gokick/app/domain/tenant"
	"gokick/app/domain/user"
)

// PlatformStats is the platform dashboard read model: top-level counts.
type PlatformStats struct {
	TenantCount int
	UserCount   int
}

// GetStatsQuery powers the superadmin dashboard cards (tenant + user totals).
type GetStatsQuery struct{}

func (GetStatsQuery) RequiredPermission() string { return "platform:overview" }

type GetStatsHandler struct {
	tenants tenant.Repository
	users   user.PlatformRepository
}

func NewGetStatsHandler(tenants tenant.Repository, users user.PlatformRepository) *GetStatsHandler {
	return &GetStatsHandler{tenants: tenants, users: users}
}

func (h *GetStatsHandler) Handle(ctx context.Context, _ GetStatsQuery) (PlatformStats, error) {
	tenantCount, err := h.tenants.Count(ctx)
	if err != nil {
		return PlatformStats{}, err
	}

	userCount, err := h.users.CountAcrossTenants(ctx)
	if err != nil {
		return PlatformStats{}, err
	}

	return PlatformStats{TenantCount: tenantCount, UserCount: userCount}, nil
}
