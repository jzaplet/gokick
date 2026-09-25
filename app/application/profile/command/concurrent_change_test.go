package command

import (
	"context"
	"errors"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// A password change reads the hash, verifies the old password against it and
// writes the new one. An admin reset that commits in between must survive it:
// the change writes only over the hash it verified, so on Postgres it refuses as
// a wrong current password instead of overwriting the reset with a password set
// by someone who knew only the old one. SQLite serializes the two, and the reset
// lands after the change.
func TestChangePassword_ConcurrentResetSurvives(t *testing.T) {
	fx := testfx.New(t)
	bob := fx.SeedUser(t, "bob", "old-password", "user")
	cmdBus, _, _ := fx.NewBuses()
	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: bob.ID, Role: bob.Role, Nickname: bob.Nickname,
	})
	cmd := ChangePasswordCommand{OldPassword: "old-password", NewPassword: "brand-new-password"}

	changeErr, resetErr := fx.RaceEdit(t,
		func(hasher shared.PasswordHasher) error {
			h := NewChangePasswordHandler(fx.Users, hasher)
			_, err := testfx.ExecCommand(
				ctx,
				cmdBus,
				"ChangePassword",
				cmd,
				func(ctx context.Context) (struct{}, error) { return struct{}{}, h.Handle(ctx, cmd) },
			)
			return err
		},
		func() error {
			return fx.Users.Update(testfx.TenantCtx(shared.DefaultTenantID), bob, "reset")
		})
	if resetErr != nil {
		t.Fatalf("reset: %v", resetErr)
	}
	var authErr *shared.AuthError
	if changeErr != nil && !errors.As(changeErr, &authErr) {
		t.Fatalf("change: %v, want success or a wrong current password", changeErr)
	}
	got, err := fx.Users.FindByID(testfx.SystemCtx(), bob.ID)
	if err != nil || got == nil {
		t.Fatalf("find: %v %v", got, err)
	}
	if got.PasswordHash != "reset" {
		t.Fatalf("password hash %q: the change overwrote a concurrent reset", got.PasswordHash)
	}
}
