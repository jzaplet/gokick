package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/internal/testfx"
)

func TestCreateTenantHandler_CreatesTenant(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_tenant.db"))

	h := NewCreateTenantHandler(fx.Tenants)
	tn, err := h.Handle(ctx, CreateTenantCommand{Name: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tn == nil || tn.Name != "Acme" || tn.ID == "" {
		t.Fatalf("expected a created tenant with an id, got %+v", tn)
	}

	got, err := fx.Tenants.FindByID(ctx, tn.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.Name != "Acme" {
		t.Fatal("created tenant must be persisted")
	}
}

func TestCreateTenantHandler_RejectsBlankName(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_tenant_blank.db"))

	h := NewCreateTenantHandler(fx.Tenants)
	if _, err := h.Handle(ctx, CreateTenantCommand{Name: "   "}); err == nil {
		t.Fatal("a blank tenant name must be rejected")
	} else {
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Field != "name" {
			t.Fatalf("expected name ValidationError, got %T: %v", err, err)
		}
	}
}

// A name is how an operator tells tenants apart — the grid lists by it, the
// Add-user picker offers {id, name} — so two "Acme"s are two identical, unpickable
// options, and a user filed into the wrong one cannot be moved out (an edit never
// changes tenant_id). Both floors are asserted here: the handler answers with a
// field error, and the row does not exist afterwards.
func TestCreateTenantHandler_RejectsADuplicateName(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_tenant_dup.db"))

	h := NewCreateTenantHandler(fx.Tenants)
	if _, err := h.Handle(ctx, CreateTenantCommand{Name: "Acme"}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := h.Handle(ctx, CreateTenantCommand{Name: "Acme"})
	if err == nil {
		t.Fatal("a second tenant with the same name must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("expected *shared.ValidationError{Field:\"name\"}, got %T: %v", err, err)
	}
}

// NewName trims, so " Acme " and "Acme" are the same name — the check has to run
// on the VALIDATED value, not the raw input, or whitespace slips a duplicate past
// it and the UNIQUE index turns a 400 into a 500.
func TestCreateTenantHandler_RejectsADuplicateNameAfterTrimming(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "create_tenant_dup_trim.db"))

	h := NewCreateTenantHandler(fx.Tenants)
	if _, err := h.Handle(ctx, CreateTenantCommand{Name: "Acme"}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := h.Handle(ctx, CreateTenantCommand{Name: "   Acme   "})
	if err == nil {
		t.Fatal("a whitespace variant of an existing name must be refused")
	}
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("expected *shared.ValidationError{Field:\"name\"}, got %T: %v", err, err)
	}
}

// The floor under the handler's check. The check is a check-then-act, so two
// concurrent creates can both pass it — the index is what actually refuses the
// loser, and it must be there even when no handler is involved.
func TestTenants_UniqueNameIsEnforcedBySchema(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_unique_schema.db"))

	first := fx.SeedTenant(t, "Acme")

	name, err := tenant.NewName("Acme")
	if err != nil {
		t.Fatalf("name: %v", err)
	}
	twin := tenant.NewTenant(name)
	if err := fx.Tenants.Save(ctx, twin); err == nil {
		t.Fatal("the schema must refuse a second tenant with the same name")
	}

	if got, _ := fx.Tenants.FindByID(ctx, first.ID); got == nil {
		t.Fatal("the original tenant must survive")
	}
}

// create-tenant stopped being CLIOnly when the superadmin plane put it behind
// POST /api/v1/platform/tenants. On the CLI it rides the SystemCommandBus, which
// skips Authorize entirely — so the bus's permission check is now the ONLY thing
// keeping a tenant admin from minting tenants, and platform:tenants:create is the
// only thing making that check bite. Fat-finger it to an admin:* string and every
// other test here stays green.
func TestCreateTenantCommand_AdminDeniedAtBus(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_create_authz.db"))
	cmdBus, _, _ := fx.NewBuses()

	adminCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "a1", Role: "admin"})

	h := NewCreateTenantHandler(fx.Tenants)
	cmd := CreateTenantCommand{Name: "Mallory Inc"}
	_, err := testfx.ExecCommand(adminCtx, cmdBus, "PlatformCreateTenant", cmd,
		func(ctx context.Context) (any, error) { return h.Handle(ctx, cmd) })
	if err == nil {
		t.Fatal("an admin must be denied create-tenant at the bus")
	}
	var pe *shared.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *shared.PermissionError, got %T: %v", err, err)
	}

	if got, _ := fx.Tenants.FindByName(ctx, "Mallory Inc"); got != nil {
		t.Fatal("a denied create-tenant must not have written a row")
	}
}

// ...and a superadmin IS allowed through the same gate. Without this the denial
// test above would pass just as well if the permission string were nonsense that
// denies everyone.
func TestCreateTenantCommand_SuperadminAllowedAtBus(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_create_ok.db"))
	cmdBus, _, _ := fx.NewBuses()

	superCtx := shared.ContextWithClaims(ctx, &shared.AuthClaims{UserID: "s1", Role: "superadmin"})

	h := NewCreateTenantHandler(fx.Tenants)
	cmd := CreateTenantCommand{Name: "Stark Industries"}
	_, err := testfx.ExecCommand(superCtx, cmdBus, "PlatformCreateTenant", cmd,
		func(ctx context.Context) (any, error) { return h.Handle(ctx, cmd) })
	if err != nil {
		t.Fatalf("a superadmin must be allowed create-tenant at the bus: %v", err)
	}

	if got, _ := fx.Tenants.FindByName(ctx, "Stark Industries"); got == nil {
		t.Fatal("the tenant must exist")
	}
}
