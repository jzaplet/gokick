package user_test

import (
	"context"
	"errors"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// newUnsavedUser builds a valid user that is not persisted yet.
func newUnsavedUser(t *testing.T, fx *testfx.Fixture, nickname, tenantID string) *user.User {
	t.Helper()
	nn, err := user.NewNickname(nickname)
	if err != nil {
		t.Fatalf("nickname: %v", err)
	}
	em, err := user.NewEmail(nickname + "@example.com")
	if err != nil {
		t.Fatalf("email: %v", err)
	}
	role, err := user.NewRole("user")
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	hash, err := fx.Hasher.Hash("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return user.NewUser(nn, hash, em, role, tenantID)
}

func assertNicknameTaken(t *testing.T, what string, err error) {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "nickname" || ve.Key != msgkey.UserNicknameTaken {
		t.Fatalf("%s: got %T %v, want the nickname-taken field error", what, err, err)
	}
}

// The nickname constraint is the truth behind the handlers' pre-check: a write
// that reaches it with a taken nickname — two concurrent creates that both passed
// the check — gets the same field error the check returns, never a raw constraint
// error (a 500). Across tenants too: nickname is unique across all of them.
func TestUserRepository_TakenNicknameIsAFieldError(t *testing.T) {
	fx := testfx.New(t)
	ctx := testfx.SystemCtx()
	other := fx.SeedTenant(t, "Acme")
	fx.SeedUser(t, "alice", "password123", "user")
	bob := fx.SeedUser(t, "bob", "password123", "user")

	assertNicknameTaken(t, "Save",
		fx.Users.Save(ctx, newUnsavedUser(t, fx, "alice", shared.DefaultTenantID)))
	assertNicknameTaken(t, "SaveAcrossTenants",
		fx.PlatformUsers.SaveAcrossTenants(ctx, newUnsavedUser(t, fx, "alice", other.ID)))

	bob.Nickname = "alice"
	assertNicknameTaken(t, "Update", fx.Users.Update(ctx, bob))
	assertNicknameTaken(t, "UpdateAcrossTenants", fx.PlatformUsers.UpdateAcrossTenants(ctx, bob))
}

// An id that is no UUID at all names a row that is not there: every id column
// is a UUID, and Postgres refuses a malformed one outright where SQLite matches
// nothing — the adapters agree on "not found", never an error (a 500).
func TestUserRepository_MalformedIDIsARowThatIsNotThere(t *testing.T) {
	fx := testfx.New(t)
	ctx := context.Background()
	const bad = "not-a-uuid"

	if got, err := fx.Users.FindByID(ctx, bad); got != nil || err != nil {
		t.Fatalf("FindByID: got %v, %v; want nil, nil", got, err)
	}
	if got, err := fx.Users.FindScopedByID(ctx, bad); got != nil || err != nil {
		t.Fatalf("FindScopedByID: got %v, %v; want nil, nil", got, err)
	}
	if got, err := fx.PlatformUsers.FindByIDAcrossTenants(ctx, bad); got != nil || err != nil {
		t.Fatalf("FindByIDAcrossTenants: got %v, %v; want nil, nil", got, err)
	}

	ghost := newUnsavedUser(t, fx, "ghost", shared.DefaultTenantID)
	ghost.ID = bad
	var ve *shared.ValidationError
	for what, err := range map[string]error{
		"Update":              fx.Users.Update(ctx, ghost),
		"Delete":              fx.Users.Delete(ctx, bad),
		"UpdatePassword":      fx.Users.UpdatePassword(ctx, bad, "hash", ghost.UpdatedAt),
		"UpdateAcrossTenants": fx.PlatformUsers.UpdateAcrossTenants(ctx, ghost),
		"DeleteAcrossTenants": fx.PlatformUsers.DeleteAcrossTenants(ctx, bad),
	} {
		if !errors.As(err, &ve) || ve.Field != "id" {
			t.Errorf("%s: got %T %v, want the not-found id error", what, err, err)
		}
	}

	survivor := fx.SeedUser(t, "survivor", "password123", "user")
	n, err := fx.Users.BulkDelete(ctx, user.BulkSelection{IDs: []string{bad, survivor.ID}})
	if err != nil || n != 1 {
		t.Fatalf("BulkDelete with one malformed id: got %d, %v; want the valid one deleted", n, err)
	}
	if err := fx.Users.RecordLogin(ctx, bad); err != nil {
		t.Fatalf("RecordLogin: %v", err)
	}
}
