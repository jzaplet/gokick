package security

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
)

// Single-tenant mode: no claims at all → default tenant. This is the path every
// public/unauthenticated command takes today, so it must stay unchanged.
func TestDefaultTenantResolver_NoClaims_ReturnsDefault(t *testing.T) {
	got, err := NewDefaultTenantResolver().Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != shared.DefaultTenantID {
		t.Fatalf("tenant = %q, want default %q", got, shared.DefaultTenantID)
	}
}

// Authenticated but TenantID empty (a single-tenant JWT carries no tenant) →
// still the default tenant. Proves a logged-in user is unaffected.
func TestDefaultTenantResolver_EmptyClaimsTenant_ReturnsDefault(t *testing.T) {
	ctx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{UserID: "u-1"})

	got, err := NewDefaultTenantResolver().Resolve(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != shared.DefaultTenantID {
		t.Fatalf("tenant = %q, want default %q", got, shared.DefaultTenantID)
	}
}

// Once the JWT populates AuthClaims.TenantID, the resolver honors it with no
// further wiring.
func TestDefaultTenantResolver_ClaimsTenant_IsHonored(t *testing.T) {
	ctx := shared.ContextWithClaims(context.Background(),
		&shared.AuthClaims{UserID: "u-1", TenantID: "tenant-7"})

	got, err := NewDefaultTenantResolver().Resolve(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "tenant-7" {
		t.Fatalf("tenant = %q, want %q", got, "tenant-7")
	}
}
