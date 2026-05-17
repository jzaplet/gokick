package token_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/internal/testfx"
)

// MarkUsed is the lynchpin of refresh-token theft detection. The
// `AND used_at IS NULL` guard plus the RowsAffected check make double
// rotation observable: two concurrent rotation requests carrying the
// same raw token both pass FindByHash (used_at still NULL in their
// snapshot), but only one wins the UPDATE — the loser sees marked=false
// and the handler revokes the session.
func TestMarkUsed_AtomicallyMarksTokenUsedOnce(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "mark_used_guard.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(time.Hour))
	hash := fx.HashToken(raw)

	marked, err := fx.Tokens.MarkUsed(ctx, hash)
	if err != nil {
		t.Fatalf("first MarkUsed: %v", err)
	}
	if !marked {
		t.Fatal("first MarkUsed must return true on a fresh token")
	}

	marked, err = fx.Tokens.MarkUsed(ctx, hash)
	if err != nil {
		t.Fatalf("second MarkUsed: %v", err)
	}
	if marked {
		t.Fatal("second MarkUsed must return false — token already used (race guard)")
	}
}

func TestMarkUsed_UnknownHashReturnsFalse(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "mark_used_unknown.db"))

	marked, err := fx.Tokens.MarkUsed(ctx, "not-a-real-hash")
	if err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if marked {
		t.Fatal("MarkUsed must return false for an unknown hash")
	}
}
