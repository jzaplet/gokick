package shared

import (
	"context"
	"testing"
)

// Work that never declared a plane gets the narrowest one: fail closed.
func TestPlaneFromContext_DefaultsToTheTenantPlane(t *testing.T) {
	if got := PlaneFromContext(context.Background()); got != PlaneTenant {
		t.Fatalf("plane of a bare ctx = %s, want tenant", got)
	}
	if PlaneTenant.CrossTenant() || Plane(99).CrossTenant() {
		t.Fatal("the tenant plane, and any unknown plane, must not be cross-tenant")
	}
	for _, p := range []Plane{PlanePlatform, PlaneSystem} {
		if !p.CrossTenant() {
			t.Errorf("%s plane must be cross-tenant", p)
		}
		if got := PlaneFromContext(ContextWithPlane(context.Background(), p)); got != p {
			t.Errorf("round trip: got %s, want %s", got, p)
		}
	}
}

func TestPlaneForPermission(t *testing.T) {
	for perm, want := range map[string]Plane{
		"platform:overview":    PlanePlatform,
		"platform:users:read":  PlanePlatform,
		"admin:users:read":     PlaneTenant,
		"profile:read":         PlaneTenant,
		"":                     PlaneTenant,
		"platformish:not-real": PlaneTenant,
	} {
		if got := PlaneForPermission(perm); got != want {
			t.Errorf("PlaneForPermission(%q) = %s, want %s", perm, got, want)
		}
	}
}
