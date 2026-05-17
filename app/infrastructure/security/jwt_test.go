package security

import (
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
)

func newTestJwtService(secret string, accessExp, refreshExp time.Duration) *JwtService {
	svc, err := NewJwtService(&config.Config{
		JWTSecret:            secret,
		JWTAccessExpiration:  accessExp,
		JWTRefreshExpiration: refreshExp,
	})
	if err != nil {
		panic(err)
	}
	return svc
}

func TestNewJwtService_EmptySecret(t *testing.T) {
	_, err := NewJwtService(&config.Config{JWTSecret: ""})
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestNewJwtService_RejectsShortSecret(t *testing.T) {
	_, err := NewJwtService(&config.Config{JWTSecret: "too-short"})
	if err == nil {
		t.Fatal("expected error for secret shorter than 32 chars")
	}
}

func TestNewJwtService_AcceptsSecretAtFloor(t *testing.T) {
	secret := "abcdefghijklmnopqrstuvwxyz012345"
	if len(secret) != 32 {
		t.Fatalf("test setup: expected 32 chars, got %d", len(secret))
	}
	if _, err := NewJwtService(&config.Config{
		JWTSecret:            secret,
		JWTAccessExpiration:  time.Minute,
		JWTRefreshExpiration: time.Hour,
	}); err != nil {
		t.Fatalf("32-char secret should be accepted, got %v", err)
	}
}

func TestGenerateAndValidateAccessToken(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 24*time.Hour)

	claims := &shared.AuthClaims{UserID: "user-1", Role: "admin", Nickname: "john"}
	token, exp, err := svc.GenerateAccessToken(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp != 15*time.Minute {
		t.Fatalf("expected 15m expiration, got %v", exp)
	}

	parsed, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.UserID != "user-1" || parsed.Role != "admin" || parsed.Nickname != "john" {
		t.Fatalf("claims mismatch: %+v", parsed)
	}
}

func TestValidateAccessToken_InvalidToken(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 24*time.Hour)
	_, err := svc.ValidateAccessToken("invalid.token.string")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateAccessToken_WrongSecret(t *testing.T) {
	svc1 := newTestJwtService("test-secret-one-32-chars-long-x1", 15*time.Minute, 24*time.Hour)
	svc2 := newTestJwtService("test-secret-two-32-chars-long-x2", 15*time.Minute, 24*time.Hour)

	token, _, _ := svc1.GenerateAccessToken(&shared.AuthClaims{UserID: "u1", Role: "user"})
	_, err := svc2.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong secret")
	}
}

func TestValidateAccessToken_ExpiredToken(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", -1*time.Second, 24*time.Hour)
	token, _, err := svc.GenerateAccessToken(&shared.AuthClaims{UserID: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = svc.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateAccessToken_EmptyString(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 24*time.Hour)
	_, err := svc.ValidateAccessToken("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGenerateRefreshToken_Uniqueness(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 24*time.Hour)
	raw1, hash1, _, _ := svc.GenerateRefreshToken()
	raw2, hash2, _, _ := svc.GenerateRefreshToken()
	if raw1 == raw2 {
		t.Fatal("expected unique raw tokens")
	}
	if hash1 == hash2 {
		t.Fatal("expected unique hashes")
	}
}

func TestGenerateRefreshToken_HashVerifiable(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 7*24*time.Hour)
	raw, hash, expiresAt, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if HashToken(raw) != hash {
		t.Fatal("hash of raw token does not match returned hash")
	}
	if time.Until(expiresAt) < 6*24*time.Hour {
		t.Fatalf("expected expiration ~7 days from now, got %v", expiresAt)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("same-input")
	h2 := HashToken("same-input")
	if h1 != h2 {
		t.Fatal("expected same hash for same input")
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := HashToken("input-a")
	h2 := HashToken("input-b")
	if h1 == h2 {
		t.Fatal("expected different hashes for different inputs")
	}
}

func TestValidateAccessToken_NoneAlgorithm(t *testing.T) {
	// Ensure "none" algorithm tokens are rejected (alg confusion attack)
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 24*time.Hour)
	// Craft a token with "none" algorithm - this is a base64 of {"alg":"none","typ":"JWT"}
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1MSIsInJvbGUiOiJhZG1pbiJ9."
	_, err := svc.ValidateAccessToken(noneToken)
	if err == nil {
		t.Fatal("expected error for none algorithm token")
	}
}
