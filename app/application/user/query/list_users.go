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
	return h.users.FindAll(ctx)
}
