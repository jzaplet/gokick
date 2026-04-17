package query

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type GetProfileQuery struct{}

func (GetProfileQuery) RequiredPermission() string { return "profile:read" }

type GetProfileHandler struct {
	users user.Repository
}

func NewGetProfileHandler(users user.Repository) *GetProfileHandler {
	return &GetProfileHandler{users: users}
}

func (h *GetProfileHandler) Handle(ctx context.Context, _ GetProfileQuery) (*user.User, error) {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, &shared.AuthError{Message: "authentication required"}
	}

	return h.users.FindByID(ctx, claims.UserID)
}
