package command

import (
	"context"
	"errors"
	"time"

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
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	u, err := h.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return err
	}
	if u == nil {
		// Self-lookup by the authenticated id; a gone row is the same rare
		// mid-session-deletion edge as get_profile. Preserve the pre-F-011 400
		// here (this is a form submit, not a session read) rather than 401.
		return &shared.ValidationError{Field: "id", Message: "user not found"}
	}

	if err := h.password.Verify(cmd.OldPassword, u.PasswordHash); err != nil {
		return &shared.AuthError{Message: "current password is incorrect"}
	}

	newHash, err := user.HashNewPassword(cmd.NewPassword, h.password)
	if err != nil {
		// `user.NewPassword` reports a generic `password` field; remap so the
		// error lands on the form's New Password input rather than `general`.
		var ve *shared.ValidationError
		if errors.As(err, &ve) {
			return &shared.ValidationError{Field: "new_password", Message: ve.Message}
		}
		return err
	}

	// Self-service password write: scoped to the caller's own id, so it works for
	// a superadmin too (the full-row Update excludes superadmin rows and would
	// silently no-op — F-039). UpdatePassword also stamps updated_at, which the
	// old full-row Update left stale here.
	if err := h.users.UpdatePassword(ctx, u.ID, newHash, time.Now()); err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.password_changed",
		TargetType: "user",
		TargetID:   u.ID,
	})

	return nil
}
