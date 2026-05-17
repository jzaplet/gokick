package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type DeleteUserCommand struct {
	ID string
}

func (DeleteUserCommand) RequiredPermission() string { return "admin:users:delete" }

type DeleteUserHandler struct {
	users user.Repository
}

func NewDeleteUserHandler(users user.Repository) *DeleteUserHandler {
	return &DeleteUserHandler{users: users}
}

func (h *DeleteUserHandler) Handle(ctx context.Context, cmd DeleteUserCommand) error {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return &shared.AuthError{Message: "authentication required"}
	}

	if claims.UserID == cmd.ID {
		return &shared.ValidationError{Message: "cannot delete your own account"}
	}

	if _, err := h.users.FindByID(ctx, cmd.ID); err != nil {
		return err
	}

	if err := h.users.Delete(ctx, cmd.ID); err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.deleted",
		TargetType: "user",
		TargetID:   cmd.ID,
	})

	return nil
}
