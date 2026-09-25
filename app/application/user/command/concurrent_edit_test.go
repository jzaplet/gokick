package command

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// An admin edit writes the whole user row back, the columns it does not change
// as it read them. A deactivation that commits while the edit sits between its
// read and its write must survive the edit: the edit loads the row locked, so
// the deactivation waits for it and then applies on top. Without the lock,
// Postgres would let the deactivation commit in between and the edit would
// silently re-activate the account; SQLite serializes the two anyway.
func TestUpdateUser_ConcurrentDeactivationSurvivesTheEdit(t *testing.T) {
	fx := testfx.New(t)
	admin := fx.SeedUser(t, "admin", "password123", "admin")
	target := fx.SeedUser(t, "bob", "password123", "user")
	cmdBus, _, _ := fx.NewBuses()
	cmd := UpdateUserCommand{
		ID: target.ID, Nickname: "bob", Email: "bob@new.example.com", Role: "user",
		Password: "newpass123",
	}

	editErr, deactivateErr := fx.RaceEdit(t,
		func(hasher shared.PasswordHasher) error {
			h := NewUpdateUserHandler(fx.Users, hasher)
			_, err := testfx.ExecCommand(
				authedCtx(admin.ID, "admin"),
				cmdBus,
				"UpdateUser",
				cmd,
				func(ctx context.Context) (struct{}, error) { return struct{}{}, h.Handle(ctx, cmd) },
			)
			return err
		},
		func() error {
			_, err := fx.Users.BulkSetActive(testfx.TenantCtx(shared.DefaultTenantID),
				user.BulkSelection{IDs: []string{target.ID}}, false)
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
