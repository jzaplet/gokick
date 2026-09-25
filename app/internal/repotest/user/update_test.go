package user_test

import (
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// An edit writes back only the columns it owns. Loaded before a deactivation and
// a password change and saved after them — what a concurrent edit does on
// Postgres, where nothing serializes the writes — it must not undo either: the
// active flag is never written, the password only when the edit sets a new one.
// The tenant and the platform write, on both DBs.
func TestUserRepository_UpdateWritesOnlyTheEditsColumns(t *testing.T) {
	writes := map[string]func(fx *testfx.Fixture, u *user.User, newHash string) error{
		"Update": func(fx *testfx.Fixture, u *user.User, newHash string) error {
			return fx.Users.Update(testfx.TenantCtx(shared.DefaultTenantID), u, newHash)
		},
		"UpdateAcrossTenants": func(fx *testfx.Fixture, u *user.User, newHash string) error {
			return fx.PlatformUsers.UpdateAcrossTenants(testfx.PlatformCtx(), u, newHash)
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			fx := testfx.New(t)
			bob := fx.SeedUser(t, "bob", "password123", "user")
			ctx := testfx.TenantCtx(shared.DefaultTenantID)
			stale := mustFindByID(t, fx, bob.ID)

			if _, err := fx.Users.BulkSetActive(ctx,
				user.BulkSelection{IDs: []string{bob.ID}}, false); err != nil {
				t.Fatalf("deactivate: %v", err)
			}
			if ok, err := fx.Users.UpdatePassword(ctx, bob.ID, bob.PasswordHash, "changed",
				stale.UpdatedAt); !ok || err != nil {
				t.Fatalf("change the password: %v, %v", ok, err)
			}

			stale.Nickname = "bobby"
			if err := write(fx, stale, ""); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			got := mustFindByID(t, fx, bob.ID)
			if got.Nickname != "bobby" {
				t.Fatalf("nickname %q, want the edit's", got.Nickname)
			}
			if got.Active || got.PasswordHash != "changed" {
				t.Fatalf("active=%v password=%q: the edit undid the deactivation or the "+
					"password change", got.Active, got.PasswordHash)
			}

			if err := write(fx, stale, "reset"); err != nil {
				t.Fatalf("%s with a new password: %v", name, err)
			}
			if got := mustFindByID(t, fx, bob.ID); got.PasswordHash != "reset" || got.Active {
				t.Fatalf("active=%v password=%q, want the new password, still inactive",
					got.Active, got.PasswordHash)
			}
		})
	}
}

// UpdatePassword swaps the hash only while it still is the one the caller
// verified the old password against: a password changed in between (an admin
// reset) is not overwritten, and the call reports it.
func TestUserRepository_UpdatePasswordSwapsOnlyTheVerifiedHash(t *testing.T) {
	fx := testfx.New(t)
	bob := fx.SeedUser(t, "bob", "password123", "user")
	ctx := testfx.TenantCtx(shared.DefaultTenantID)

	if ok, err := fx.Users.UpdatePassword(ctx, bob.ID, "stale", "mine", bob.UpdatedAt); ok ||
		err != nil {
		t.Fatalf("with a stale current hash: %v, %v; want false, nil", ok, err)
	}
	if got := mustFindByID(t, fx, bob.ID); got.PasswordHash != bob.PasswordHash {
		t.Fatalf("password %q: a stale swap wrote", got.PasswordHash)
	}
	if ok, err := fx.Users.UpdatePassword(ctx, bob.ID, bob.PasswordHash, "mine",
		bob.UpdatedAt); !ok || err != nil {
		t.Fatalf("with the current hash: %v, %v; want true, nil", ok, err)
	}
	if got := mustFindByID(t, fx, bob.ID); got.PasswordHash != "mine" {
		t.Fatalf("password %q, want the swapped one", got.PasswordHash)
	}
}
