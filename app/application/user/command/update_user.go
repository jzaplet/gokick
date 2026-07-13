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

// ensureNicknameAvailable rejects a rename that collides with another user's
// nickname (nickname is globally unique). An unchanged nickname is a no-op.
func (h *UpdateUserHandler) ensureNicknameAvailable(
	ctx context.Context,
	nickname string,
	target *user.User,
) error {
	if nickname == target.Nickname {
		return nil
	}
	conflict, err := h.users.FindByNickname(ctx, nickname)
	if err != nil {
		return err
	}
	if conflict != nil && conflict.ID != target.ID {
		return &shared.ValidationError{
			Field:   "nickname",
			Message: "user with this nickname already exists",
		}
	}
	return nil
}

func (h *UpdateUserHandler) Handle(ctx context.Context, cmd UpdateUserCommand) error {
	target, err := h.users.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}

	// A superadmin (platform) account is managed out-of-band, never through
	// tenant-admin user management — mirror DeleteUserHandler and the platform
	// handlers. Without this the repo's role != 'superadmin' filter turns the
	// Update into a 0-row no-op that (pre-F-039) reported success and emitted a
	// phantom user.role_changed audit for a change that never persisted (F-023).
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
	// An admin must not promote anyone TO superadmin (self-escalation to the
	// platform plane). The repo's Update also excludes superadmin rows, so an
	// existing superadmin can't be demoted/edited here either.
	if role.IsSuperAdmin() {
		return &shared.ValidationError{
			Field:   "role",
			Message: "cannot assign the superadmin role",
		}
	}

	// Mirror DeleteUserHandler's self-lockout guard: don't let an admin
	// demote themselves out of admin and lock the org out of admin ops.
	// Self-update of other fields (nickname, password, email) stays allowed.
	claims := shared.ClaimsFromContext(ctx)
	if claims != nil && claims.UserID == target.ID && string(role) != string(user.RoleAdmin) {
		return &shared.ValidationError{
			Field:   "role",
			Message: "cannot change your own role",
		}
	}

	email, err := user.NewEmail(cmd.Email)
	if err != nil {
		return err
	}

	if err := h.ensureNicknameAvailable(ctx, string(nickname), target); err != nil {
		return err
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

	target.Nickname = string(nickname)
	target.Email = string(email)
	target.Role = string(role)
	target.UpdatedAt = time.Now()

	if err := h.users.Update(ctx, target); err != nil {
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
