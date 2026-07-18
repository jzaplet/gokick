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
// about. An empty role means "the spec does not name one", which is how
// create_superadmin.go builds its spec; it is not routed through NewRole, since
// that (correctly) rejects the empty string.
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

	s := CreateSpec{
		Nickname: n,
		Password: p,
		Email:    e,
		TenantID: shared.DefaultTenantID,
	}
	if role != "" {
		r, err := user.NewRole(role)
		if err != nil {
			t.Fatalf("role: %v", err)
		}
		s.Role = r
	}

	return s
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
// sanctioned minter. CreateSuperAdmin stamps the role itself — the entry point
// decides, not the call site — so a spec that never mentions a role still yields a
// superadmin. This is how create_superadmin.go calls it.
func TestCreateSuperAdmin_MintsDespiteCreatesRefusal(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_minted.db"))

	u, err := CreateSuperAdmin(
		ctx,
		Deps{Repo: fx.Users, Hasher: fx.Hasher},
		spec(t, "root", "password123", ""),
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

// Stamping the role must not mean SILENTLY overwriting a conflicting one.
// CreateSuperAdmin shares Create's exact signature, so the two are
// interchangeable wherever one is held in a variable — a spec meaning RoleUser
// that reaches the wrong branch would otherwise come out a superadmin with
// neither the compiler nor a guard objecting, defeating the refusal this whole
// split exists to make unforgettable.
func TestCreateSuperAdmin_RefusesASpecAskingForAnotherRole(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_conflict.db"))

	_, err := CreateSuperAdmin(
		ctx,
		Deps{Repo: fx.Users, Hasher: fx.Hasher},
		spec(t, "confused", "password123", "user"),
		fx.Users.Save,
	)
	if err == nil {
		t.Fatal("a spec asking for a non-superadmin role must be refused, not overwritten")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected *shared.ValidationError{Field:\"role\"}, got %T: %v", err, err)
	}

	got, err := fx.Users.FindByNickname(ctx, "confused")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != nil {
		t.Fatal("no user may be persisted by a refused create")
	}
}

// An explicit superadmin role agrees with what the entry point does, so it is
// accepted — the refusal above is about a CONFLICT, not about mentioning the role.
func TestCreateSuperAdmin_AcceptsAnExplicitSuperAdminRole(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_superadmin_explicit.db"))

	u, err := CreateSuperAdmin(
		ctx,
		Deps{Repo: fx.Users, Hasher: fx.Hasher},
		spec(t, "root", "password123", "superadmin"),
		fx.Users.Save,
	)
	if err != nil {
		t.Fatalf("an explicit superadmin role must be accepted: %v", err)
	}
	if u.Role != string(user.RoleSuperAdmin) {
		t.Fatalf("expected a superadmin, got %q", u.Role)
	}
}
