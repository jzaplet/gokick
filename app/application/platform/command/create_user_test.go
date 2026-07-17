package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

// The point of the platform create: the superadmin CHOOSES the tenant. The admin
// twin inherits it from ctx, so this is the one behaviour a copy of that handler
// would get wrong.
func TestCreatePlatformUser_CreatesInTheChosenTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_chosen.db"))

	target := fx.SeedTenant(t, "Beta")

	h := NewCreatePlatformUserHandler(fx.Users, fx.Tenants, fx.Hasher, true)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob@example.com",
		Role:     "admin",
		TenantID: target.ID,
	})
	if err != nil {
		t.Fatalf("create in a chosen tenant must land: %v", err)
	}

	got, err := fx.Users.FindByNickname(ctx, "bob")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("the user must exist")
	}
	if got.TenantID != target.ID {
		t.Fatalf("user must land in the CHOSEN tenant %q, got %q", target.ID, got.TenantID)
	}
	if got.Role != "admin" {
		t.Fatalf("role must persist, got %q", got.Role)
	}
}

// An unknown tenant owes the operator a 400 against the field, not the 500 an FK
// violation would produce (users.tenant_id REFERENCES tenants(id)).
func TestCreatePlatformUser_UnknownTenantIsAFieldError(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_badtenant.db"))

	h := NewCreatePlatformUserHandler(fx.Users, fx.Tenants, fx.Hasher, true)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob@example.com",
		Role:     "user",
		TenantID: "01920000-0000-7000-8000-000000000000",
	})
	if err == nil {
		t.Fatal("an unknown tenant must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "tenant_id" {
		t.Fatalf("expected *shared.ValidationError{Field:\"tenant_id\"}, got %T: %v", err, err)
	}

	if got, _ := fx.Users.FindByNickname(ctx, "bob"); got != nil {
		t.Fatal("no user may be created when the tenant is unknown")
	}
}

// Fail-closed: with multitenancy on, a missing tenant must not silently drop the
// user into the default tenant. It is a form field here, so it earns a 400 rather
// than shared.RequireTenant's 500 (which signals a non-bus path bug instead).
func TestCreatePlatformUser_MultitenantRequiresATenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_notenant.db"))

	h := NewCreatePlatformUserHandler(fx.Users, fx.Tenants, fx.Hasher, true)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob@example.com",
		Role:     "user",
	})
	if err == nil {
		t.Fatal("multitenant mode must refuse a create with no tenant")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "tenant_id" {
		t.Fatalf("expected *shared.ValidationError{Field:\"tenant_id\"}, got %T: %v", err, err)
	}

	if got, _ := fx.Users.FindByNickname(ctx, "bob"); got != nil {
		t.Fatal("no user may be born tenant-less")
	}
}

// Single-tenant mode keeps RequireTenant's leniency: there is exactly one tenant
// to mean, so an absent tenant_id is not ambiguous.
func TestCreatePlatformUser_SingleTenantDefaultsTheTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pcreate_single.db"))

	h := NewCreatePlatformUserHandler(fx.Users, fx.Tenants, fx.Hasher, false)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob@example.com",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("single-tenant create with no tenant_id must land: %v", err)
	}

	got, _ := fx.Users.FindByNickname(ctx, "bob")
	if got == nil || got.TenantID != shared.DefaultTenantID {
		t.Fatalf("user must land in the default tenant, got %+v", got)
	}
}

// Nobody mints a superadmin over HTTP — not even a superadmin. The CLI and the
// seeder are the only paths, by design.
func TestCreatePlatformUser_RefusesTheSuperadminRole(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "pcreate_super.db"))

	h := NewCreatePlatformUserHandler(fx.Users, fx.Tenants, fx.Hasher, false)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "root2",
		Password: "correct horse battery staple",
		Email:    "r@x.com",
		Role:     "superadmin",
	})
	if err == nil {
		t.Fatal("creating a superadmin through the API must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "role" {
		t.Fatalf("expected *shared.ValidationError{Field:\"role\"}, got %T: %v", err, err)
	}

	if got, _ := fx.Users.FindByNickname(ctx, "root2"); got != nil {
		t.Fatal("no superadmin may be created through the API")
	}
}

// A nickname is globally unique (users.nickname UNIQUE), so the collision check
// must reach ACROSS tenants — a platform create into tenant B must still trip on
// a nickname held in tenant A. This is why the shared body's FindByNickname is a
// deliberately unscoped identity lookup.
func TestCreatePlatformUser_NicknameCollidesAcrossTenants(t *testing.T) {
	ctx := context.Background()
	fx := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "pcreate_collide.db"))

	tenantA := fx.SeedTenant(t, "Alpha")
	tenantB := fx.SeedTenant(t, "Beta")
	fx.SeedUserInTenant(t, "bob", "user", tenantA.ID)

	h := NewCreatePlatformUserHandler(fx.Users, fx.Tenants, fx.Hasher, true)
	err := h.Handle(ctx, CreatePlatformUserCommand{
		Nickname: "bob",
		Password: "correct horse battery staple",
		Email:    "bob2@example.com",
		Role:     "user",
		TenantID: tenantB.ID,
	})
	if err == nil {
		t.Fatal("a nickname taken in ANOTHER tenant must still collide (it is globally unique)")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "nickname" {
		t.Fatalf("expected *shared.ValidationError{Field:\"nickname\"}, got %T: %v", err, err)
	}
}
