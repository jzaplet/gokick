package middleware

import (
	"context"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// TenantMiddleware resolves the active tenant once per request (right after
// AuthorizeMiddleware) and stores it in ctx, so every downstream handler,
// repository and synchronously-dispatched event sees the same tenant. It lives
// in BaseChain, so both the CommandBus and the QueryBus get it — reads need
// scoping just as much as writes.
//
// A resolver error aborts the command/query before any handler runs: a tenant
// that cannot be resolved is never silently treated as "no tenant".
//
// The worker bypasses the bus, so this middleware never runs for job handlers —
// the worker restores the tenant from the claimed job row instead.
func TenantMiddleware(resolver shared.TenantResolver) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		tenantID, err := resolver.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		return next(shared.ContextWithTenantID(ctx, tenantID))
	}
}
