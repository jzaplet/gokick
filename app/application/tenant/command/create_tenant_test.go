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
