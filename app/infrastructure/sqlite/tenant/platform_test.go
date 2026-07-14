package tenant_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/internal/testfx"
)

// OverviewAcrossTenants is the cross-tenant GROUP BY aggregate behind
// the superadmin tenant overview. It must count each tenant's users correctly,
// INCLUDING a tenant with zero users (the LEFT JOIN must yield 0, not drop it) —
// the exact shape the product reuses to SUM the tenant_usage ledger.
func TestTenantRepository_OverviewAcrossTenants(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "tenant_overview.db"))

	tenantA := fx.SeedTenant(t, "Acme")
	tenantB := fx.SeedTenant(t, "Globex")
	empty := fx.SeedTenant(t, "Empty")
	fx.SeedUserInTenant(t, "alice", "admin", tenantA.ID)
	fx.SeedUserInTenant(t, "anna", "user", tenantA.ID)
	fx.SeedUserInTenant(t, "bob", "admin", tenantB.ID)

	rows, err := fx.PlatformTenants.OverviewAcrossTenants(ctx)
	if err != nil {
		t.Fatalf("OverviewAcrossTenants: %v", err)
	}

	counts := map[string]int{}
	plans := map[string]string{}
	for _, r := range rows {
		counts[r.ID] = r.UserCount
		plans[r.ID] = r.Plan
	}

	// The bootstrap "Default" tenant also exists, so assert per-tenant rather than
	// a total. Acme=2, Globex=1, Empty=0 (LEFT JOIN must keep the empty tenant).
	if counts[tenantA.ID] != 2 {
		t.Fatalf("Acme must report 2 users, got %d", counts[tenantA.ID])
	}
	if counts[tenantB.ID] != 1 {
		t.Fatalf("Globex must report 1 user, got %d", counts[tenantB.ID])
	}
	if _, ok := counts[empty.ID]; !ok {
		t.Fatal("a tenant with zero users must still appear (LEFT JOIN), it was dropped")
	}
	if counts[empty.ID] != 0 {
		t.Fatalf("Empty must report 0 users, got %d", counts[empty.ID])
	}
	if plans[tenantA.ID] != "free" {
		t.Fatalf("default plan must be 'free', got %q", plans[tenantA.ID])
	}
}
