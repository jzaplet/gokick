package middleware

import (
	"context"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// PlaneMiddleware marks which plane the command or query runs on (shared.Plane),
// right after AuthorizeMiddleware, so the transaction the chain opens later lands
// on the right database role. The plane comes from the permission the operation
// already declares — platform:* is the platform plane, everything else the tenant
// plane — so there is nothing extra to declare and nothing to forget. It always
// sets the plane, never inherits one: a tenant command dispatched from inside
// platform work still runs as a tenant command.
func PlaneMiddleware() bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		plane := shared.PlaneTenant
		if p, ok := cmd.(shared.Permissioned); ok {
			plane = shared.PlaneForPermission(p.RequiredPermission())
		}
		return next(shared.ContextWithPlane(ctx, plane))
	}
}

// SystemPlaneMiddleware marks every command of the SystemCommandBus as
// shared.PlaneSystem: the operator-trusted CLI commands act outside any tenant
// (create a user in any tenant, seed the admin), so their transaction runs on
// the role that bypasses row-level security.
func SystemPlaneMiddleware() bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		return next(shared.ContextWithPlane(ctx, shared.PlaneSystem))
	}
}
