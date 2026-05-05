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

func (h *GetUserDashboardHandler) Handle(ctx context.Context, _ GetUserDashboardQuery) (UserDashboard, error) {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return UserDashboard{}, &shared.AuthError{Message: "authentication required"}
	}

	return UserDashboard{
		Message: "Vítej " + claims.Nickname + " — toto je placeholder user dashboard.",
	}, nil
}
