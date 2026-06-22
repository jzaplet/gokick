package security

import (
	"context"

	"gokick/app/domain/shared"
)

// DefaultTenantResolver is the single-tenant resolver: it yields the tenant id
// carried by the request's AuthClaims when present, otherwise DefaultTenantID.
//
// In single-tenant mode (multitenancy off) the JWT carries no tenant, so this
// always returns DefaultTenantID. When the JWT does carry a tenant claim (a
// multi-tenant deployment), this resolver honors it with no further wiring.
// Binding a different resolver (subdomain,
// header, …) is how a multi-tenant deployment overrides single-tenant mode.
type DefaultTenantResolver struct{}

func NewDefaultTenantResolver() *DefaultTenantResolver {
	return &DefaultTenantResolver{}
}

func (*DefaultTenantResolver) Resolve(ctx context.Context) (string, error) {
	if claims := shared.ClaimsFromContext(ctx); claims != nil && claims.TenantID != "" {
		return claims.TenantID, nil
	}
	return shared.DefaultTenantID, nil
}
