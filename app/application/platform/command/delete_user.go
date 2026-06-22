package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// DeletePlatformUserCommand deletes a user in ANY tenant (superadmin plane).
type DeletePlatformUserCommand struct {
	ID string
}

func (DeletePlatformUserCommand) RequiredPermission() string { return "platform:users:delete" }

type DeletePlatformUserHandler struct {
	users user.Repository
}

func NewDeletePlatformUserHandler(users user.Repository) *DeletePlatformUserHandler {
	return &DeletePlatformUserHandler{users: users}
}

func (h *DeletePlatformUserHandler) Handle(
	ctx context.Context,
	cmd DeletePlatformUserCommand,
) error {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return &shared.AuthError{Message: "authentication required"}
	}
	if claims.UserID == cmd.ID {
		return &shared.ValidationError{Message: "cannot delete your own account"}
	}

	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	// A superadmin (platform) account is never deletable through the API — the
	// repo delete also excludes superadmin rows.
	if user.Role(target.Role).IsSuperAdmin() {
		return &shared.PermissionError{Message: "cannot delete a superadmin account"}
	}

	if err := h.users.DeleteAcrossTenants(ctx, cmd.ID); err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.deleted",
		TargetType: "user",
		TargetID:   cmd.ID,
	})

	return nil
}
