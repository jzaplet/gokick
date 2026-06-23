package shared

import "context"

type AuthClaims struct {
	UserID   string
	Role     string
	Nickname string
	Email    string
	// TenantID carries the tenant minted into the JWT at login/refresh. The
	// default single-tenant resolver falls back to shared.DefaultTenantID when it
	// is empty, so a single-tenant deployment (JWT carries no tenant) is unchanged.
	TenantID string
}

type authClaimsKeyType struct{}

var authClaimsKey = authClaimsKeyType{}

func ClaimsFromContext(ctx context.Context) *AuthClaims {
	claims, _ := ctx.Value(authClaimsKey).(*AuthClaims)
	return claims
}

func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context {
	return context.WithValue(ctx, authClaimsKey, claims)
}
