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
