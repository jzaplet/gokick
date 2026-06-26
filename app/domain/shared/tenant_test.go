package shared_test

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
)

// AssertTenantScope rejects a write that places a row in a tenant other than the
// active ctx scope (multitenant). No scope (system/seed) is trusted; single-tenant
// never restricts.
func TestAssertTenantScope(t *testing.T) {
	t.Parallel()
	scopedA := shared.ContextWithTenantID(context.Background(), "tenant-a")

	// Single-tenant never restricts, even a mismatched row tenant.
	if err := shared.AssertTenantScope(scopedA, "tenant-b", false); err != nil {
		t.Fatalf("single-tenant must not restrict, got %v", err)
	}
	// Multitenant, row matches the active scope → ok.
	if err := shared.AssertTenantScope(scopedA, "tenant-a", true); err != nil {
		t.Fatalf("matching tenant must pass, got %v", err)
	}
	// Multitenant, row in a different tenant → cross-tenant write rejected.
	if err := shared.AssertTenantScope(scopedA, "tenant-b", true); err == nil {
		t.Fatal("cross-tenant write must be rejected")
	}
	// Multitenant, NO active scope (system/seed) → trusted, no error.
	if err := shared.AssertTenantScope(context.Background(), "tenant-b", true); err != nil {
		t.Fatalf("no active scope must be trusted (seed/system path), got %v", err)
	}
}

// RequireTenant is the write-side fail-closed resolver: a non-empty tenant is kept
// in either mode; an empty one defaults in single-tenant mode but errors in
// multitenant mode (a tenant-owned row must never be silently born in the default
// tenant because it was created outside a tenant context).
func TestRequireTenant(t *testing.T) {
	t.Parallel()

	for _, mt := range []shared.Multitenancy{false, true} {
		if got, err := shared.RequireTenant("tenant-x", mt); err != nil || got != "tenant-x" {
			t.Fatalf(
				"multitenant=%v: a present tenant must pass through, got %q err=%v",
				mt,
				got,
				err,
			)
		}
	}

	if got, err := shared.RequireTenant("", false); err != nil || got != shared.DefaultTenantID {
		t.Fatalf("empty + single-tenant must default, got %q err=%v", got, err)
	}

	if _, err := shared.RequireTenant("", true); err == nil {
		t.Fatal("empty + multitenant must fail closed, got nil error")
	}
}
