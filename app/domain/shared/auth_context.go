package shared

import "context"

type AuthClaims struct {
	UserID   string
	Role     string
	Nickname string
	Email    string
	// TenantID is empty until Krok 4 mints it into the JWT. The default
	// single-tenant resolver falls back to shared.DefaultTenantID when it is
	// empty, so adding the field now changes nothing — it just lets Krok 4
	// start carrying a real tenant with no further wiring.
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
