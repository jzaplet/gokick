package query

import (
	"testing"

	"gokick/app/internal/testfx"
)

func TestGetTenantHandler_FindsAndMisses(t *testing.T) {
	ctx := testfx.PlatformCtx()
	fx := testfx.New(t)
	tn := fx.SeedTenant(t, "Acme")

	h := NewGetTenantHandler(fx.Tenants)

	got, err := h.Handle(ctx, GetTenantQuery{ID: tn.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Name != "Acme" {
		t.Fatalf("expected tenant Acme, got %+v", got)
	}

	missing, err := h.Handle(ctx, GetTenantQuery{ID: "no-such-id"})
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("an unknown tenant id must return nil, got %+v", missing)
	}
}
