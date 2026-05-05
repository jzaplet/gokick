package query

import (
	"context"

	"gokick/app/domain/shared"
)

type GetAdminDashboardQuery struct{}

func (GetAdminDashboardQuery) RequiredPermission() string { return "admin:dashboard:read" }

type AdminDashboard struct {
	Message string
}

type GetAdminDashboardHandler struct{}

func NewGetAdminDashboardHandler() *GetAdminDashboardHandler {
	return &GetAdminDashboardHandler{}
}

func (h *GetAdminDashboardHandler) Handle(
	ctx context.Context,
	_ GetAdminDashboardQuery,
) (AdminDashboard, error) {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return AdminDashboard{}, &shared.AuthError{Message: "authentication required"}
	}

	return AdminDashboard{
		Message: "Welcome " + claims.Nickname + " — this is a placeholder admin dashboard.",
	}, nil
}
