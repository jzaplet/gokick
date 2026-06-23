package query

import (
	"context"

	"gokick/app/domain/user"
)

type ListUsersQuery struct{}

func (ListUsersQuery) RequiredPermission() string { return "admin:users:read" }

type ListUsersHandler struct {
	users user.Repository
}

func NewListUsersHandler(users user.Repository) *ListUsersHandler {
	return &ListUsersHandler{users: users}
}

func (h *ListUsersHandler) Handle(ctx context.Context, _ ListUsersQuery) ([]user.User, error) {
	// CANARY (temporary — reverted in the segregation commit): a non-platform
	// handler reaching for a cross-tenant escape hatch. Proves the
	// platform-isolation gate bites; does not change the returned result.
	_, _ = h.users.CountAcrossTenants(ctx)
	return h.users.FindAll(ctx)
}
