package command

import (
	"context"

	"gokick/app/application/userwrite"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// UpdatePlatformUserCommand edits a user in ANY tenant (superadmin platform
// plane). It has NO Active field on purpose: it reuses the same form as admin
// edit (which sends no `active`), so the handler loads the target and preserves
// target.Active rather than letting a missing field deactivate the user.
type UpdatePlatformUserCommand struct {
	ID       string
	Nickname string
	Password string // empty = unchanged
	Email    string
	Role     string
}

func (UpdatePlatformUserCommand) RequiredPermission() string { return "platform:users:update" }

type UpdatePlatformUserHandler struct {
	users    user.PlatformRepository
	password shared.PasswordHasher
}

func NewUpdatePlatformUserHandler(
	users user.PlatformRepository,
	password shared.PasswordHasher,
) *UpdatePlatformUserHandler {
	return &UpdatePlatformUserHandler{users: users, password: password}
}

func (h *UpdatePlatformUserHandler) Handle(
	ctx context.Context,
	cmd UpdatePlatformUserCommand,
) error {
	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	// No self-demote guard on the platform plane (a superadmin editing tenant
	// users can't lock itself out), so guard is nil. The cross-tenant
	// UpdateAcrossTenants is the only divergence from the admin handler; the
	// shared body (incl. leaving target.Active untouched) lives in userwrite.
	return userwrite.Update(ctx, h.users, h.password, target, userwrite.Fields{
		Nickname: cmd.Nickname,
		Email:    cmd.Email,
		Role:     cmd.Role,
		Password: cmd.Password,
	}, nil, h.users.UpdateAcrossTenants)
}
