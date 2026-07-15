package command

import (
	"context"

	"gokick/app/application/userwrite"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type UpdateUserCommand struct {
	ID       string
	Nickname string
	Password string // empty = unchanged
	Email    string // empty = no email
	Role     string
}

func (UpdateUserCommand) RequiredPermission() string { return "admin:users:update" }

type UpdateUserHandler struct {
	users    user.Repository
	password shared.PasswordHasher
}

func NewUpdateUserHandler(
	users user.Repository,
	password shared.PasswordHasher,
) *UpdateUserHandler {
	return &UpdateUserHandler{users: users, password: password}
}

func (h *UpdateUserHandler) Handle(ctx context.Context, cmd UpdateUserCommand) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if target == nil {
		return &shared.ValidationError{Field: "id", Message: "user not found"}
	}

	// Admin-only guard: don't let an admin demote themselves out of admin and lock
	// the org out of admin ops (self-update of nickname/password/email stays OK).
	// Plane-specific, so it rides userwrite.Update's guard hook rather than the
	// shared body. The platform handler passes nil (no self-demote concern there).
	selfDemoteGuard := func(role user.Role) error {
		if claims.UserID == target.ID && string(role) != string(user.RoleAdmin) {
			return &shared.ValidationError{
				Field:   "role",
				Message: "cannot change your own role",
			}
		}
		return nil
	}

	return userwrite.Update(
		ctx,
		userwrite.Deps{Repo: h.users, Hasher: h.password},
		target,
		userwrite.Fields{
			Nickname: cmd.Nickname,
			Email:    cmd.Email,
			Role:     cmd.Role,
			Password: cmd.Password,
		},
		userwrite.Plane{Guard: selfDemoteGuard, Save: h.users.Update},
	)
}
