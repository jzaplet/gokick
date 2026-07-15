package query

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// GetUserQuery reads a single user in the caller's tenant — the read-one behind
// GET /admin/users/{id}. It is tenant-scoped in the repository (FindScopedByID),
// so an admin can never read a user outside its tenant or a superadmin row.
type GetUserQuery struct {
	ID string
}

func (GetUserQuery) RequiredPermission() string { return "admin:users:read" }

type GetUserHandler struct {
	users user.Repository
}

func NewGetUserHandler(users user.Repository) *GetUserHandler {
	return &GetUserHandler{users: users}
}

func (h *GetUserHandler) Handle(ctx context.Context, q GetUserQuery) (*user.User, error) {
	u, err := h.users.FindScopedByID(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		// Not-found as a 400 with Field "id" — the SAME shape the Update/Delete
		// paths return via requireOneRow, so every "no such user" response is
		// consistent and the HTTP handler needs no nil-check before building the DTO.
		//gkerrf:exempt path-param lookup - the edit view redirects on failure, no form field maps id
		return nil, &shared.ValidationError{Field: "id", Message: "user not found"}
	}
	return u, nil
}
