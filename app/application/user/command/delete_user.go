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
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	if claims.UserID == cmd.ID {
		return &shared.ValidationError{Message: "cannot delete your own account"}
	}

	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	// A superadmin (platform) account is managed out-of-band — mirror the platform
	// delete handler. Without this the repo's role != 'superadmin' filter turns the
	// Delete into a 0-row no-op that (pre-F-039) reported success and emitted a
	// phantom user.deleted audit for a row that was never removed.
	if user.Role(target.Role).IsSuperAdmin() {
		return &shared.PermissionError{Message: "cannot delete a superadmin account"}
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
