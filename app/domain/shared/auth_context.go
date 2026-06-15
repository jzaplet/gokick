package shared

import "context"

type AuthClaims struct {
	UserID   string
	Role     string
	Nickname string
	Email    string
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
