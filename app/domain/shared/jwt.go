package shared

import "time"

type JwtService interface {
	GenerateAccessToken(claims *AuthClaims) (token string, expiresIn time.Duration, err error)
	ValidateAccessToken(token string) (*AuthClaims, error)
	GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
	HashRefreshToken(raw string) string
}
