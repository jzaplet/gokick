package token_test

import (
	"testing"
	"time"

	"gokick/app/internal/testfx"
)

// TestRefreshTokens_CascadeOnUserDelete pins the schema's ON DELETE CASCADE on
// refresh_tokens.user_id (claim infra-db-security-13): deleting a user removes
// its refresh tokens. The user is deleted with a raw statement on purpose, so the
// test pins the schema rule rather than whatever a repository might clean up
// itself. Without the cascade (or with foreign keys off) the orphan row would
// survive and the count would stay 1.
func TestRefreshTokens_CascadeOnUserDelete(t *testing.T) {
	fx := testfx.New(t)
	u := fx.SeedUser(t, "cascadeuser", "pwd", "user")
	fx.SeedRefreshToken(t, u.ID, time.Now().Add(time.Hour))

	if before := fx.Count(t, "refresh_tokens", "user_id = ?", u.ID); before != 1 {
		t.Fatalf("precondition: expected 1 refresh token before delete, got %d", before)
	}

	if _, err := fx.RawExec(`DELETE FROM users WHERE id = ?`, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	if after := fx.Count(t, "refresh_tokens", "user_id = ?", u.ID); after != 0 {
		t.Fatalf("expected refresh_tokens cascade-deleted, got %d remaining", after)
	}
}
