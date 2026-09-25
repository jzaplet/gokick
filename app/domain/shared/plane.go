package shared

import (
	"context"
	"strings"
)

// Plane names the kind of access a unit of work has to the data — which rows it
// may see. It rides in ctx next to the tenant: the adapter reads it when it opens
// a transaction. On Postgres it picks the database role: the tenant plane connects
// as a role bound by row-level security (it sees exactly the active tenant), the
// other two as a role that bypasses it. On SQLite, which has no roles, it changes
// nothing — isolation there is the repositories' WHERE tenant_id alone.
type Plane uint8

const (
	// PlaneTenant is work on behalf of one tenant: every user-facing command and
	// query, and a run handler. It is the zero value, so work that never declared
	// a plane gets the narrowest one — fail closed.
	PlaneTenant Plane = iota
	// PlanePlatform is the superadmin's cross-tenant work: a command or query whose
	// permission is platform:* (see PlaneForPermission).
	PlanePlatform
	// PlaneSystem is operator and background work outside any tenant: the CLI
	// commands on the SystemCommandBus (create-user, seed, …).
	PlaneSystem
)

// CrossTenant reports whether the plane may reach every tenant's rows. Only the
// two named cross-tenant planes do: an unknown value fails closed, like the zero
// value.
func (p Plane) CrossTenant() bool { return p == PlanePlatform || p == PlaneSystem }

func (p Plane) String() string {
	switch p {
	case PlaneTenant:
		return "tenant"
	case PlanePlatform:
		return "platform"
	case PlaneSystem:
		return "system"
	default:
		return "unknown"
	}
}

// PlaneForPermission is the plane a bus operation runs on, derived from the one
// declaration every operation already makes: a platform:* permission is the
// platform plane, anything else (incl. SkipPermission) the tenant plane. There is
// no second list to keep in sync — the same prefix already gates the role ladder
// (IsPermissionAllowedForRole).
func PlaneForPermission(permission string) Plane {
	if strings.HasPrefix(permission, PlatformPermissionPrefix) {
		return PlanePlatform
	}
	return PlaneTenant
}

// PreTenant marks a command that runs on the system plane although it arrives
// through the CommandBus unauthenticated: it acts before any tenant is known.
// Login finds the account by its nickname — unique across every tenant — and a
// refresh finds the session by its token, so each has to reach the account and
// its tokens in whichever tenant they live. PlaneMiddleware honors the marker only
// on a command that skips the permission check (SkipPermission): a permissioned
// command runs on the plane its permission names, whatever it implements. The
// implementers are an allow-list (app/application/zz_pretenant_test.go).
type PreTenant interface {
	PreTenant()
}

type planeKey struct{}

// ContextWithPlane stores the plane in ctx. PlaneMiddleware sets it for every
// command and query; the SystemCommandBus marks its commands PlaneSystem.
func ContextWithPlane(ctx context.Context, p Plane) context.Context {
	return context.WithValue(ctx, planeKey{}, p)
}

// PlaneFromContext returns the plane stored in ctx, or PlaneTenant when none was
// set — the narrowest plane.
func PlaneFromContext(ctx context.Context) Plane {
	p, _ := ctx.Value(planeKey{}).(Plane)
	return p
}
