package userwrite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// spec builds a CreateSpec from raw strings, failing the test on invalid input —
// the value-object parsing is the callers' job and is not what these tests are
// about.
func spec(t *testing.T, nickname, password, role string) CreateSpec {
	t.Helper()

	n, err := user.NewNickname(nickname)
	if err != nil {
		t.Fatalf("nickname: %v", err)
	}
	p, err := user.NewPassword(password)
	if err != nil {
		t.Fatalf("password: %v", err)
	}
	e, err := user.NewEmail("")
	if err != nil {
		t.Fatalf("email: %v", err)
	}
	r, err := user.NewRole(role)
	if err != nil {
		t.Fatalf("role: %v", err)
	}

	return CreateSpec{
		Nickname: n,
		Password: p,
		Email:    e,
		Role:     r,
		TenantID: shared.DefaultTenantID,
	}
}

// The floor under every create handler: the shared body refuses the superadmin
// role on its own, so a future create path that forgets the check cannot mint one.
// Both current handlers refuse it first (their order fixes error precedence) —
// which is exactly why this test calls Create DIRECTLY. Through a handler it would
// pass with this guard deleted.
func TestCreate_RefusesTheSuperAdminRole(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_refused.db"))

	u, err := Create(
		ctx,
		Deps{Repo: fx.Users, Hasher: fx.Hasher},
		spec(t, "sneaky", "password123", "superadmin"),
		fx.Users.Save,
	)
	if err == nil {
		t.Fatal("Create must refuse the superadmin role")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected *shared.ValidationError{Field:\"role\"}, got %T: %v", err, err)
	}
	if u != nil {
		t.Fatal("a refused create must not return a user")
	}

	// The refusal must be a refusal, not a message over a completed insert.
	got, err := fx.Users.FindByNickname(ctx, "sneaky")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatal("no user may be persisted by a refused create")
	}
}

// The refusal runs BEFORE the uniqueness lookup, which is what keeps error
// precedence identical to the handlers that already refuse up front: they never
// reach Create with a superadmin role, so the role error must win here too. Were
// the guard placed after FindByNickname, this same input would answer "nickname
// already exists" and the two paths would disagree.
func TestCreate_RefusesSuperAdminBeforeTheUniquenessLookup(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_order.db"))

	fx.SeedUser(t, "taken", "password123", "user")

	_, err := Create(
		ctx,
		Deps{Repo: fx.Users, Hasher: fx.Hasher},
		spec(t, "taken", "password123", "superadmin"),
		fx.Users.Save,
	)
	if err == nil {
		t.Fatal("Create must refuse the superadmin role")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "role" {
		t.Fatalf("the role refusal must beat the taken nickname, got field %q: %v", ve.Field, ve)
	}
}

// The other half of the rule: making Create default-deny must not break the one
// sanctioned minter. CreateSuperAdmin stamps the role itself, so the caller cannot
// ask for anything else — and the account it writes really is a superadmin.
func TestCreateSuperAdmin_MintsDespiteCreatesRefusal(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_minted.db"))

	// Role deliberately "user": CreateSuperAdmin overrides it, which is what makes
	// the entry point — not the call site — the thing that decides.
	u, err := CreateSuperAdmin(
		ctx,
		Deps{Repo: fx.Users, Hasher: fx.Hasher},
		spec(t, "root", "password123", "user"),
		fx.Users.Save,
	)
	if err != nil {
		t.Fatalf("CreateSuperAdmin must still mint a superadmin: %v", err)
	}
	if u.Role != string(user.RoleSuperAdmin) {
		t.Fatalf("CreateSuperAdmin must stamp the superadmin role, got %q", u.Role)
	}

	got, err := fx.Users.FindByNickname(ctx, "root")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the superadmin must be persisted")
	}
	if got.Role != string(user.RoleSuperAdmin) {
		t.Fatalf("the persisted row must be a superadmin, got %q", got.Role)
	}
}
