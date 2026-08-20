package shared

import (
	"context"

	"gokick/app/domain/shared/msgkey"
)

type AuthClaims struct {
	UserID   string
	Role     string
	Nickname string
	Email    string
	// TenantID carries the tenant minted into the JWT at login/refresh. The
	// default single-tenant resolver falls back to shared.DefaultTenantID when it
	// is empty, so a single-tenant deployment (JWT carries no tenant) is unchanged.
	TenantID string
	// Lang carries the user's persisted UI-language preference (users.lang)
	// minted at login/refresh. Tolerant like nickname/email: an absent claim
	// (an older token) simply means no preference override — the header /
	// Accept-Language resolution stands.
	Lang string
}

type authClaimsKey struct{}

func ClaimsFromContext(ctx context.Context) *AuthClaims {
	claims, _ := ctx.Value(authClaimsKey{}).(*AuthClaims)
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
		return nil, &AuthError{Key: msgkey.AuthRequired}
	}
	return claims, nil
}

func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context {
	return context.WithValue(ctx, authClaimsKey{}, claims)
}
