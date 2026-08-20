package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/user"
)

// DeletePlatformUserCommand deletes a user in ANY tenant (superadmin plane).
type DeletePlatformUserCommand struct {
	ID string
}

func (DeletePlatformUserCommand) RequiredPermission() string { return "platform:users:delete" }

type DeletePlatformUserHandler struct {
	users user.PlatformRepository
}

func NewDeletePlatformUserHandler(users user.PlatformRepository) *DeletePlatformUserHandler {
	return &DeletePlatformUserHandler{users: users}
}

func (h *DeletePlatformUserHandler) Handle(
	ctx context.Context,
	cmd DeletePlatformUserCommand,
) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}
	if claims.UserID == cmd.ID {
		return &shared.ValidationError{Key: msgkey.UserOwnAccountUndeletable}
	}

	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if target == nil {
		//gkerrf:exempt path-param lookup - the edit/list view redirects on failure, no form field maps id
		return &shared.ValidationError{Field: "id", Key: msgkey.UserNotFound}
	}

	// A superadmin (platform) account is never deletable through the API — the
	// repo delete also excludes superadmin rows.
	if user.Role(target.Role).IsSuperAdmin() {
		return &shared.PermissionError{Key: msgkey.PermissionSuperadminUndeletable}
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
