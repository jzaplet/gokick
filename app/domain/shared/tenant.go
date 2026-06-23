package shared

import "context"

// DefaultTenantID is the well-known id of the single "default" tenant used when
// multitenancy is off (single-tenant mode). A migration creates the matching
// bootstrap tenant row and every user references it, so a single-tenant
// deployment behaves exactly as if tenants did not exist.
const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

// TenantResolver resolves the active tenant for the current request. It is a
// port: the default single-tenant implementation lives in infrastructure and
// always yields DefaultTenantID, so turning multitenancy on later means binding
// a different resolver (JWT claim, subdomain, …), not rewriting handlers.
type TenantResolver interface {
	Resolve(ctx context.Context) (string, error)
}

type tenantIDKeyType struct{}

var tenantIDKey = tenantIDKeyType{}

// ContextWithTenantID stores the resolved tenant id in ctx. TenantMiddleware
// calls this once per request (right after authorization) so every downstream
// handler, repository and synchronously-dispatched event sees the same tenant.
func ContextWithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// TenantIDFromContext returns the resolved tenant id, or "" when none was set
// (e.g. work that never went through the bus). Callers that require a tenant
// treat "" as a programming error — the BaseRepository.Tenant helper panics on
// it (in multitenant mode) rather than silently run an unscoped query.
func TenantIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(tenantIDKey).(string)
	return id
}
