package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type ChangePasswordCommand struct {
	OldPassword string
	NewPassword string
}

func (ChangePasswordCommand) RequiredPermission() string { return "profile:update" }

type ChangePasswordHandler struct {
	users    user.Repository
	password shared.PasswordHasher
}

func NewChangePasswordHandler(users user.Repository, password shared.PasswordHasher) *ChangePasswordHandler {
	return &ChangePasswordHandler{
		users:    users,
		password: password,
	}
}

func (h *ChangePasswordHandler) Handle(ctx context.Context, cmd ChangePasswordCommand) error {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return &shared.AuthError{Message: "authentication required"}
	}

	u, err := h.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return err
	}

	if err := h.password.Verify(cmd.OldPassword, u.PasswordHash); err != nil {
		return &shared.AuthError{Message: "current password is incorrect"}
	}

	newHash, err := h.password.Hash(cmd.NewPassword)
	if err != nil {
		return err
	}

	u.PasswordHash = newHash

	return h.users.Update(ctx, u)
}
