package command

import (
	"context"
	"time"

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

	// A superadmin (platform) account is never editable through the API — it is
	// managed out-of-band only. The repo write also excludes superadmin rows.
	if user.Role(target.Role).IsSuperAdmin() {
		return &shared.PermissionError{Message: "cannot modify a superadmin account"}
	}

	nickname, err := user.NewNickname(cmd.Nickname)
	if err != nil {
		return err
	}

	role, err := user.NewRole(cmd.Role)
	if err != nil {
		return err
	}
	if role.IsSuperAdmin() {
		return &shared.ValidationError{
			Field:   "role",
			Message: "cannot assign the superadmin role",
		}
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
				Message: "user with this nickname already exists",
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

	roleChanged := target.Role != string(role)

	// target.Active is left untouched — the edit form carries no active flag.
	target.Nickname = string(nickname)
	target.Email = string(email)
	target.Role = string(role)
	target.UpdatedAt = time.Now()

	if err := h.users.UpdateAcrossTenants(ctx, target); err != nil {
		return err
	}

	if roleChanged {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
			Action:     "user.role_changed",
			TargetType: "user",
			TargetID:   target.ID,
			Metadata:   map[string]any{"new_role": target.Role},
		})
	}

	return nil
}
