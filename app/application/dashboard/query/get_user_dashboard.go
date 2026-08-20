package query

import (
	"context"

	"gokick/app/domain/shared"
)

type GetUserDashboardQuery struct{}

func (GetUserDashboardQuery) RequiredPermission() string { return "dashboard:read" }

// UserDashboard carries raw data only — the greeting sentence is composed on
// the frontend from a translation key (no server-rendered prose in
// data DTOs).
type UserDashboard struct {
	Nickname string
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

	return UserDashboard{Nickname: claims.Nickname}, nil
}
