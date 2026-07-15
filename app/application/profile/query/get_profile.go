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
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return nil, err
	}

	u, err := h.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		// The authenticated user's own row is gone (deleted mid-session). Under the
		// old contract FindByID's raw *ValidationError leaked a bogus 400
		// {"id":"user not found"} to a valid JWT; a vanished session subject is an
		// auth failure → 401, so the client clears its session and re-logs in
		// instead of being shown a phantom form-field error.
		return nil, &shared.AuthError{Message: "user no longer exists"}
	}
	return u, nil
}
