package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gokick/app/domain/shared"
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
