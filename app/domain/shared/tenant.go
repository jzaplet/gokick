package shared

import (
	"context"
	"fmt"
)

// Multitenancy is the configured enforcement mode (APP_MULTITENANCY) as a
// Wire-distinct type, so it can be injected into application-layer constructors
// (which may not import infrastructure) without a bare-bool ambiguity.
type Multitenancy bool

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

// RequireTenant resolves the tenant to stamp on a NEW tenant-owned row (a job, a
// run, a user). It is the write-side mirror of BaseRepository.Tenant: a non-empty
// tenantID is kept; an empty one yields DefaultTenantID in single-tenant mode but a
// FAIL-CLOSED error in multitenant mode — so a tenant-owned row is never silently
// born in the default tenant just because it was created outside a tenant context
// (a non-bus path that forgot to resolve the tenant). The bus path always carries a
// tenant (TenantMiddleware), so this error only fires on a genuine bug. Pass either
// TenantIDFromContext(ctx) (ctx-based callers) or the row's own TenantID (repos).
func RequireTenant(tenantID string, multitenant Multitenancy) (string, error) {
	if tenantID != "" {
		return tenantID, nil
	}
	if multitenant {
		return "", fmt.Errorf(
			"shared: tenant required but absent (APP_MULTITENANCY=true) — a tenant-owned row " +
				"must be created within a tenant context",
		)
	}
	return DefaultTenantID, nil
}

// AssertTenantScope guards a tenant-scoped write against placing a row in a tenant
// OTHER than the active one. In multitenant mode, when ctx carries a tenant scope,
// the row's tenant must equal it — otherwise it is a cross-tenant write (a bug or an
// attack: a handler running in tenant A persisting a row stamped tenant B). When ctx
// carries NO scope (system/seed paths that never went through TenantMiddleware), the
// row's explicit tenant is trusted — there is no active scope to violate. Single-
// tenant mode never restricts. It is the write-side complement of RequireTenant:
// RequireTenant guarantees a row HAS a tenant; this guarantees it is the RIGHT one.
func AssertTenantScope(ctx context.Context, rowTenant string, multitenant Multitenancy) error {
	if !multitenant {
		return nil
	}
	if scope := TenantIDFromContext(ctx); scope != "" && rowTenant != scope {
		return fmt.Errorf(
			"shared: cross-tenant write rejected — row tenant %q does not match the active tenant %q",
			rowTenant,
			scope,
		)
	}
	return nil
}
