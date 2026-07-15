package query

import (
	"context"

	"gokick/app/domain/shared"
)

type GetUserDashboardQuery struct{}

func (GetUserDashboardQuery) RequiredPermission() string { return "dashboard:read" }

type UserDashboard struct {
	Message string
}

type GetUserDashboardHandler struct{}

func NewGetUserDashboardHandler() *GetUserDashboardHandler {
	return &GetUserDashboardHandler{}
}

func (h *GetUserDashboardHandler) Handle(
	ctx context.Context,
	_ GetUserDashboardQuery,
) (UserDashboard, error) {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return UserDashboard{}, err
	}

	return UserDashboard{
		Message: "Welcome " + claims.Nickname + " — this is a placeholder user dashboard.",
	}, nil
}
