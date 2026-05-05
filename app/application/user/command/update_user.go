package command

import (
	"context"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type UpdateUserCommand struct {
	ID       string
	Nickname string
	Password string // prázdné = beze změny
	Email    string // prázdné = bez emailu
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
	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	nickname, err := user.NewNickname(cmd.Nickname)
	if err != nil {
		return err
	}

	role, err := user.NewRole(cmd.Role)
	if err != nil {
		return err
	}

	email, err := user.NewEmail(cmd.Email)
	if err != nil {
		return err
	}

	if string(nickname) != target.Nickname {
		conflict, err := h.users.FindByNickname(ctx, string(nickname))
		if err != nil {
			return err
		}
		if conflict != nil && conflict.ID != target.ID {
			return &shared.ValidationError{
				Field:   "nickname",
				Message: "uživatel s tímto nickname už existuje",
			}
		}
	}

	if cmd.Password != "" {
		newPassword, err := user.NewPassword(cmd.Password)
		if err != nil {
			return err
		}
		hash, err := h.password.Hash(string(newPassword))
		if err != nil {
			return err
		}
		target.PasswordHash = hash
	}

	target.Nickname = string(nickname)
	target.Email = string(email)
	target.Role = string(role)
	target.UpdatedAt = time.Now()

	return h.users.Update(ctx, target)
}
