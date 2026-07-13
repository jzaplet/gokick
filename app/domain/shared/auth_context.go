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

// RequireClaims returns the authenticated claims, or an AuthError if none are in
// ctx. Every claims-reading handler goes through it so a handler ever reached
// without Authorize (a future SkipPermission command that starts reading claims,
// a direct unit call, an off-bus worker path) fails closed with a clean 401
// instead of a nil-deref panic recovered into a 500.
func RequireClaims(ctx context.Context) (*AuthClaims, error) {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return nil, &AuthError{Message: "authentication required"}
	}
	return claims, nil
}

func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context {
	return context.WithValue(ctx, authClaimsKey, claims)
}
