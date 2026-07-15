package shared

import "time"

// TokenService is the auth-token seam: it mints and validates short-lived JWT
// access tokens AND issues/hashes the OPAQUE crypto/rand refresh tokens. It is
// named TokenService, not JwtService, because half its surface
// (GenerateRefreshToken / HashRefreshToken) is not JWT at all — the old name
// over-claimed. The concrete adapter keeps the name security.JwtService, which is
// accurate inside the JWT package.
type TokenService interface {
	GenerateAccessToken(claims *AuthClaims) (token string, expiresIn time.Duration, err error)
	ValidateAccessToken(token string) (*AuthClaims, error)
	GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
	HashRefreshToken(raw string) string
}
