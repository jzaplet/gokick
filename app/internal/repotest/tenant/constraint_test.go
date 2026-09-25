package tenant_test

import (
	"errors"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/tenant"
	"gokick/app/internal/testfx"
)

// A tenant name another tenant holds comes back as the field error
// CreateTenantHandler's pre-check returns — the index is the truth behind the
// check, and two concurrent creates can both pass it.
func TestTenantRepository_TakenNameIsAFieldError(t *testing.T) {
	fx := testfx.New(t)
	fx.SeedTenant(t, "Acme")

	name, err := tenant.NewName("Acme")
	if err != nil {
		t.Fatalf("name: %v", err)
	}
	err = fx.Tenants.Save(testfx.SystemCtx(), tenant.NewTenant(name))
	var ve *shared.ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" || ve.Key != msgkey.TenantNameTaken {
		t.Fatalf("got %T %v, want the name-taken field error", err, err)
	}
}

// A malformed id names a tenant that is not there — refused, never an error.
func TestTenantRepository_MalformedIDIsARowThatIsNotThere(t *testing.T) {
	fx := testfx.New(t)
	ctx := testfx.PlatformCtx()
	const bad = "not-a-uuid"

	if deleted, err := fx.PlatformTenants.DeleteIfEmptyAcrossTenants(ctx, bad); deleted ||
		err != nil {
		t.Fatalf("DeleteIfEmptyAcrossTenants: got %v, %v; want false, nil", deleted, err)
	}
	ids, err := fx.PlatformTenants.BulkDeleteEmptyAcrossTenants(ctx,
		tenant.BulkSelection{IDs: []string{bad}})
	if err != nil || len(ids) != 0 {
		t.Fatalf("BulkDeleteEmptyAcrossTenants: got %v, %v; want nothing deleted", ids, err)
	}
}
