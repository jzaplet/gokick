package command

import (
	"context"
	"errors"

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

func NewChangePasswordHandler(
	users user.Repository,
	password shared.PasswordHasher,
) *ChangePasswordHandler {
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

	newPassword, err := user.NewPassword(cmd.NewPassword)
	if err != nil {
		// `user.NewPassword` reports a generic `password` field; remap so the
		// error lands on the form's New Password input rather than `general`.
		var ve *shared.ValidationError
		if errors.As(err, &ve) {
			return &shared.ValidationError{Field: "new_password", Message: ve.Message}
		}
		return err
	}

	newHash, err := h.password.Hash(string(newPassword))
	if err != nil {
		return err
	}

	u.PasswordHash = newHash

	if err := h.users.Update(ctx, u); err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.password_changed",
		TargetType: "user",
		TargetID:   u.ID,
	})

	return nil
}
