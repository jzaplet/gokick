package security

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gokick/app/domain/shared"
)

// TestHash_PrehashDiscriminatesBeyond72Bytes closes domain-26 and infra-db-security-29.
//
// bcrypt silently truncates its input at 72 bytes. Without the SHA-256 prehash, two
// passwords that share the same first 72 bytes but differ afterwards would cross-verify
// (a false positive). The prehash maps the *entire* password to a 32-byte digest first,
// so a difference past byte 72 must change the hash. We also pin the bcrypt cost (12) via
// the "$2a$12$" prefix.
func TestHash_PrehashDiscriminatesBeyond72Bytes(t *testing.T) {
	h := NewPasswordHasher()

	a := strings.Repeat("a", 72) + "X"
	b := strings.Repeat("a", 72) + "Y"

	hash, err := h.Hash(a)
	if err != nil {
		t.Fatalf("unexpected error hashing: %v", err)
	}

	// Cost 12 is part of the security contract: "$2" (bcrypt) + "$12$" (cost).
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Fatalf("expected bcrypt cost-12 prefix \"$2a$12$\", got: %q", hash[:7])
	}

	// Sanity: a verifies against its own hash.
	if err := h.Verify(a, hash); err != nil {
		t.Fatalf("expected a to verify against its own hash: %v", err)
	}

	// The discriminating assertion: b differs from a only past byte 72, so it MUST NOT
	// verify. Without the prehash this would erroneously succeed (bcrypt truncation).
	if err := h.Verify(b, hash); err == nil {
		t.Fatal(
			"expected Verify to fail for a password differing only past byte 72 (prehash missing?)",
		)
	}
}

// TestCheck_RoleDichotomy closes infra-db-security-32 and guide-auth-perm-54.
//
// Two distinct failure branches must map to two distinct error types:
//   - authenticated wrong-role -> *shared.PermissionError ("insufficient permissions", HTTP 403)
//   - no claims at all         -> *shared.AuthError        (HTTP 401)
func TestCheck_RoleDichotomy(t *testing.T) {
	c := NewPermissionChecker()

	// Role mismatch: a "user" requesting an admin permission.
	userCtx := shared.ContextWithClaims(context.Background(), &shared.AuthClaims{
		UserID: "u1", Role: "user", Nickname: "jane",
	})
	err := c.Check(userCtx, "admin:users:list")
	permErr, ok := err.(*shared.PermissionError)
	if !ok {
		t.Fatalf("role mismatch: expected *shared.PermissionError, got %T (%v)", err, err)
	}
	if permErr.Message != "insufficient permissions" {
		t.Fatalf("expected message \"insufficient permissions\", got %q", permErr.Message)
	}
	if permErr.HTTPStatus() != 403 {
		t.Fatalf("expected PermissionError HTTPStatus 403, got %d", permErr.HTTPStatus())
	}

	// No claims: must be an authentication error, not a permission error.
	err = c.Check(context.Background(), "admin:users:list")
	authErr, ok := err.(*shared.AuthError)
	if !ok {
		t.Fatalf("no claims: expected *shared.AuthError, got %T (%v)", err, err)
	}
	if authErr.HTTPStatus() != 401 {
		t.Fatalf("expected AuthError HTTPStatus 401, got %d", authErr.HTTPStatus())
	}
}

// TestGenerateAccessToken_ContainsAllClaims closes guide-auth-perm-04.
//
// ValidateAccessToken only surfaces sub/role/nickname, so it cannot prove iat is set.
// Decode the JWT payload directly and assert all five documented claims are present,
// with iat ~= now (small tolerance to absorb second-boundary truncation).
func TestGenerateAccessToken_ContainsAllClaims(t *testing.T) {
	svc := newTestJwtService("test-secret-32-chars-long-enough", 15*time.Minute, 24*time.Hour)

	before := time.Now().Unix()
	token, _, err := svc.GenerateAccessToken(&shared.AuthClaims{
		UserID: "user-1", Role: "admin", Nickname: "john",
	})
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}

	// JWT payloads use unpadded base64url.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to base64url-decode payload: %v", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("failed to unmarshal payload JSON: %v", err)
	}

	for _, key := range []string{"sub", "role", "nickname", "iat", "exp"} {
		if _, present := claims[key]; !present {
			t.Fatalf("expected claim %q present in access token payload, got keys: %v", key, claims)
		}
	}

	// Numeric JSON values decode to float64.
	iat, ok := claims["iat"].(float64)
	if !ok {
		t.Fatalf("expected iat to be numeric, got %T", claims["iat"])
	}
	after := time.Now().Unix()
	if int64(iat) < before-2 || int64(iat) > after+2 {
		t.Fatalf("expected iat ~= now (%d..%d), got %d", before, after, int64(iat))
	}
}
