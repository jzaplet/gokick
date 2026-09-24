// Package jwtfx builds a JwtService for tests that need tokens but no database.
// It lives apart from testfx on purpose: importing testfx links a database
// adapter, and a middleware test that only mints tokens has no reason to.
package jwtfx

import (
	"testing"
	"time"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/security"
)

// Secret is the HS256 key every test JwtService signs with (testfx included), so
// a token minted by one verifies in another.
const Secret = "test-secret-32-chars-long-enough"

// New returns a JwtService with the given access expiration — a negative one
// mints already-expired tokens for expired-token scenarios.
func New(t *testing.T, accessExp time.Duration) *security.JwtService {
	t.Helper()
	svc, err := security.NewJwtService(&config.Config{
		JWTSecret:            Secret,
		JWTAccessExpiration:  accessExp,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	return svc
}
