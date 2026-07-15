package query

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// GetUserQuery reads a single user in ANY tenant — the cross-tenant read-one
// behind GET /platform/users/{id}, the by-id sibling of ListAllUsersQuery.
// Superadmin only (platform:overview, the platform READ family); an admin is
// denied at the bus.
type GetUserQuery struct {
	ID string
}

func (GetUserQuery) RequiredPermission() string { return "platform:overview" }

type GetUserHandler struct {
	users user.PlatformRepository
}

func NewGetUserHandler(users user.PlatformRepository) *GetUserHandler {
	return &GetUserHandler{users: users}
}

func (h *GetUserHandler) Handle(ctx context.Context, q GetUserQuery) (*user.PlatformRow, error) {
	row, err := h.users.FindByIDAcrossTenants(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		// Not-found as a 400 with Field "id" — mirrors the admin read-one and the
		// platform Update/Delete paths, so the HTTP handler needs no nil-check.
		//gkerrf:exempt path-param lookup - the edit view redirects on failure, no form field maps id
		return nil, &shared.ValidationError{Field: "id", Message: "user not found"}
	}
	return row, nil
}
