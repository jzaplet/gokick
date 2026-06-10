package token_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/internal/testfx"
)

// DeleteExpired once had a TZ-format bug (Go's time.Time serialises with a
// zone offset that doesn't lex-compare to SQLite's UTC datetime('now')),
// making the F2 cleanup a silent no-op. The fix wraps expires_at in
// datetime(...) so the comparison normalises to UTC. The load-bearing
// assertion here is that the past-dated row is actually GONE (count drops
// 2 -> 1): under the old no-op bug nothing would be deleted and the count
// would stay at 2. The survivor-identity check additionally guards against
// deleting the wrong row.
func TestDeleteExpired_RemovesPastDatedTokensKeepsFuture(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "delete_expired_basic.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")

	expiredRaw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(-1*time.Hour))
	futureRaw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(time.Hour))

	// Belt-and-suspenders: both rows are really present before cleanup, so a
	// silent seed failure can't make a later count==1 pass for the wrong reason.
	fx.AssertTokenCount(t, 2)

	if err := fx.Tokens.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}

	// The fix: the expired row is gone (would survive under the TZ no-op bug).
	fx.AssertTokenCount(t, 1)

	// The survivor is the future-dated token, not the expired one.
	expiredTok, err := fx.Tokens.FindByHash(ctx, fx.HashToken(expiredRaw))
	if err != nil {
		t.Fatalf("FindByHash(expired): %v", err)
	}
	if expiredTok != nil {
		t.Fatal("expired token must be deleted by DeleteExpired")
	}

	futureTok, err := fx.Tokens.FindByHash(ctx, fx.HashToken(futureRaw))
	if err != nil {
		t.Fatalf("FindByHash(future): %v", err)
	}
	if futureTok == nil {
		t.Fatal("future-dated token must survive DeleteExpired")
	}
}

// DeleteExpired's WHERE clause keys solely on expires_at < datetime('now');
// used_at is intentionally NOT part of the cleanup condition. A token that
// has been used (used_at set) but is not yet expired must therefore survive.
// The guard that keeps this test from degenerating into the plain
// "future token survives" case is asserting used_at is genuinely non-NULL
// BEFORE the cleanup runs — otherwise we'd merely be re-proving roadmap-43.
func TestDeleteExpired_KeepsUsedButUnexpiredToken(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "delete_expired_used.db"))
	u := fx.SeedUser(t, "bob", "pwd", "user")

	expiredRaw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(-1*time.Hour))
	usedRaw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(time.Hour))
	usedHash := fx.HashToken(usedRaw)

	// Mark the unexpired token used so used_at is set. Verify the mark took —
	// if MarkUsed silently returned false, used_at would stay NULL and this
	// test would no longer distinguish "used survives" from "future survives".
	marked, err := fx.Tokens.MarkUsed(ctx, usedHash)
	if err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if !marked {
		t.Fatal("MarkUsed must mark a fresh unexpired token")
	}

	preTok, err := fx.Tokens.FindByHash(ctx, usedHash)
	if err != nil {
		t.Fatalf("FindByHash(used, pre-cleanup): %v", err)
	}
	if preTok == nil {
		t.Fatal("used token must exist before cleanup")
	}
	if preTok.UsedAt == nil {
		t.Fatal("used token must have used_at set before cleanup")
	}

	fx.AssertTokenCount(t, 2)

	if err := fx.Tokens.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}

	// Only the expired row is gone; the used-but-unexpired row survives
	// because used_at plays no part in the cleanup condition.
	fx.AssertTokenCount(t, 1)

	expiredTok, err := fx.Tokens.FindByHash(ctx, fx.HashToken(expiredRaw))
	if err != nil {
		t.Fatalf("FindByHash(expired): %v", err)
	}
	if expiredTok != nil {
		t.Fatal("expired token must be deleted regardless of used_at")
	}

	survivor, err := fx.Tokens.FindByHash(ctx, usedHash)
	if err != nil {
		t.Fatalf("FindByHash(used, post-cleanup): %v", err)
	}
	if survivor == nil {
		t.Fatal(
			"used-but-unexpired token must survive DeleteExpired (used_at not part of condition)",
		)
	}
	if survivor.UsedAt == nil {
		t.Fatal("surviving token must still have used_at set")
	}
}
