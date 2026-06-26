package shared_test

import (
	"testing"

	"gokick/app/domain/shared"
)

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
