package command

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// The platform twin of the admin edit's lost-update test: a superadmin's edit of
// a user in any tenant writes the whole row back, so a deactivation committing
// between the edit's read and its write must survive it (the edit loads the row
// locked).
func TestUpdatePlatformUser_ConcurrentDeactivationSurvivesTheEdit(t *testing.T) {
	fx := testfx.New(t)
	root := fx.SeedUser(t, "root", "password123", "superadmin")
	tenantB := fx.SeedTenant(t, "Beta")
	target := fx.SeedUserInTenant(t, "bob", "user", tenantB.ID)
	cmdBus, _, _ := fx.NewBuses()
	cmd := UpdatePlatformUserCommand{
		ID: target.ID, Nickname: "bob", Email: "bob@new.example.com", Role: "user",
		Password: "newpass123",
	}

	editErr, deactivateErr := fx.RaceEdit(t,
		func(hasher shared.PasswordHasher) error {
			h := NewUpdatePlatformUserHandler(fx.PlatformUsers, hasher)
			_, err := testfx.ExecCommand(
				superadminCtx(root.ID),
				cmdBus,
				"UpdatePlatformUser",
				cmd,
				func(ctx context.Context) (struct{}, error) { return struct{}{}, h.Handle(ctx, cmd) },
			)
			return err
		},
		func() error {
			_, err := fx.PlatformUsers.BulkSetActiveAcrossTenants(testfx.PlatformCtx(),
				user.PlatformBulkSelection{IDs: []string{target.ID}}, false)
			return err
		})
	if editErr != nil || deactivateErr != nil {
		t.Fatalf("edit: %v, deactivate: %v", editErr, deactivateErr)
	}
	got, err := fx.Users.FindByID(testfx.SystemCtx(), target.ID)
	if err != nil || got == nil {
		t.Fatalf("find: %v %v", got, err)
	}
	if got.Email != "bob@new.example.com" {
		t.Fatalf("email %q: the edit was lost", got.Email)
	}
	if got.Active {
		t.Fatal("the account is active again: the edit overwrote a concurrent deactivation")
	}
}
