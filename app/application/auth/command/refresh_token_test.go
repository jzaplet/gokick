package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"
)

func TestRefreshTokenHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_success.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	result, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.RefreshToken == "" || result.RefreshToken == raw {
		t.Fatal("expected new (rotated) refresh token")
	}
	if result.User.ID != u.ID {
		t.Fatalf("user id mismatch: got %s want %s", result.User.ID, u.ID)
	}

	// Old token must remain but be marked used (so reuse can be detected).
	old, _ := fx.Tokens.FindByHash(ctx, fx.HashToken(raw))
	if old == nil {
		t.Fatal("expected old refresh token retained for theft detection")
	}
	if old.UsedAt == nil {
		t.Fatal("expected old refresh token marked as used")
	}
	// New token must exist and be unused.
	stored, _ := fx.Tokens.FindByHash(ctx, fx.HashToken(result.RefreshToken))
	if stored == nil || stored.UserID != u.ID {
		t.Fatal("expected new refresh token persisted for user")
	}
	if stored.UsedAt != nil {
		t.Fatal("expected new refresh token to be unused")
	}
	fx.AssertTokenCount(t, 2)
}

func TestRefreshTokenHandler_Expired(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_expired.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(-1*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
	// Expired token must be cleaned up.
	fx.AssertTokenCount(t, 0)
}

func TestRefreshTokenHandler_UnknownToken(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_unknown.db"))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: "not-a-real-token"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
	fx.AssertTokenCount(t, 0)
}

func TestRefreshTokenHandler_UserDeletedAfterIssue(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_user_gone.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	// Simulate the user being deleted after the token was issued (cascades to refresh_tokens).
	if err := fx.Users.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestRefreshTokenHandler_ReuseTriggersForceLogout(t *testing.T) {
	// Using an already-rotated refresh token signals theft: drop all tokens for that user.
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_theft.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	// First refresh rotates the token (legitimate client or attacker).
	result, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	fx.AssertTokenCount(t, 2) // old (used) + new

	// Presenting the already-used raw token again triggers force logout.
	_, err = handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError on reuse, got %T: %v", err, err)
	}

	// All user tokens (including the freshly-issued one) must be gone.
	fx.AssertTokenCount(t, 0)
	stillValid, _ := fx.Tokens.FindByHash(ctx, fx.HashToken(result.RefreshToken))
	if stillValid != nil {
		t.Fatal("expected newly-issued token to be revoked after theft detection")
	}
}

// The theft path must emit auth.token.theft_detected with metadata {reason}
// (audit.md table + app-events-audit-40). The behavioral test above proves the
// force-logout; this proves the security event operators rely on to SEE it.
func TestRefreshTokenHandler_ReuseRecordsTheftAudit(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_theft_audit.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	handler := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt)

	// First rotation consumes the raw token (no collector needed here).
	if _, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw}); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	// Reuse the already-rotated token with a collector attached.
	collector := &shared.AuditCollector{}
	auditCtx := shared.ContextWithAuditCollector(ctx, collector)
	if _, err := handler.Handle(auditCtx, RefreshTokenCommand{RawToken: raw}); err == nil {
		t.Fatal("expected AuthError on token reuse")
	}

	var theft *shared.AuditEvent
	for _, e := range collector.Drain() {
		if e.Action == "auth.token.theft_detected" {
			ev := e
			theft = &ev
		}
	}
	if theft == nil {
		t.Fatal("expected an auth.token.theft_detected audit event")
	}
	if theft.TargetID != u.ID {
		t.Fatalf("theft target_id: got %q want %q", theft.TargetID, u.ID)
	}
	if theft.Metadata["reason"] != "reused_after_rotation" {
		t.Fatalf(
			"theft reason metadata: got %v want reused_after_rotation",
			theft.Metadata["reason"],
		)
	}
}
