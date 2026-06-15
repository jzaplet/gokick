package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// stubFindByIDUsers wraps a real user.Repository but forces FindByID to fail with
// a transient (non-ValidationError) error, to exercise the durable-logout guard.
type stubFindByIDUsers struct {
	user.Repository
	err error
}

func (s stubFindByIDUsers) FindByID(context.Context, string) (*user.User, error) {
	return nil, s.err
}

// stubSaveFailsTokens wraps a real token.TokenRepository but forces Save (the
// persist of the freshly-rotated token) to fail, to prove the rotation order:
// the new token is saved BEFORE the old one is marked used.
type stubSaveFailsTokens struct {
	token.TokenRepository
	err error
}

func (s stubSaveFailsTokens) Save(context.Context, *token.RefreshToken) error {
	return s.err
}

// stubDeleteFailsTokens wraps a real token.TokenRepository but forces
// DeleteByUserID (the theft-response revocation) to fail, to prove a failed
// revocation surfaces a raw error instead of a laundered AuthError.
type stubDeleteFailsTokens struct {
	token.TokenRepository
	err error
}

func (s stubDeleteFailsTokens) DeleteByUserID(context.Context, string) error {
	return s.err
}

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

// A transient DB error during the user lookup must propagate as a RAW error (→
// 5xx → cookie kept), NOT be laundered into *AuthError. Otherwise the HTTP layer
// (clearRefreshCookie only on *AuthError) and the SPA (clearSessionHint only on
// 401) would clear the refresh cookie + session hint and durably log out a
// still-valid session on a momentary blip — the regression this changeset fixes.
func TestRefreshTokenHandler_TransientFindByIDErrorIsNotAuthError(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_transient.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	dbBlip := errors.New("database is locked")
	handler := NewRefreshTokenHandler(
		stubFindByIDUsers{Repository: fx.Users, err: dbBlip},
		fx.Tokens, fx.Jwt,
	)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})

	var authErr *shared.AuthError
	if errors.As(err, &authErr) {
		t.Fatalf(
			"transient FindByID error must NOT become *AuthError (would clear cookie), got %v",
			err,
		)
	}
	if !errors.Is(err, dbBlip) {
		t.Fatalf("expected the raw DB error to propagate, got %T: %v", err, err)
	}
	// The lookup fails before MarkUsed, so the token is not consumed and the
	// next attempt can still succeed once the DB recovers.
	fx.AssertTokenCount(t, 1)
}

// Rotation order invariant: the new token is persisted (Save) BEFORE the old one
// is marked used (MarkUsed). So when Save fails transiently, the old token must
// remain UNCONSUMED (used_at == nil) — the next attempt can rotate it cleanly.
// Under the reverse order a failed Save would leave the old token already marked
// used, and the retry presenting the same cookie would trip theft detection and
// force-log-out a legitimate client. This test pins the order that prevents that.
func TestRefreshTokenHandler_SaveFailureLeavesOldTokenUnconsumed(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_save_fail.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	dbBlip := errors.New("database is locked")
	handler := NewRefreshTokenHandler(
		fx.Users,
		stubSaveFailsTokens{TokenRepository: fx.Tokens, err: dbBlip},
		fx.Jwt,
	)

	_, err := handler.Handle(ctx, RefreshTokenCommand{RawToken: raw})
	if !errors.Is(err, dbBlip) {
		t.Fatalf("expected the raw Save error to propagate, got %T: %v", err, err)
	}

	// The old token must still exist AND be unused, so a retry can rotate it
	// instead of being treated as a reused (stolen) token.
	old, _ := fx.Tokens.FindByHash(ctx, fx.HashToken(raw))
	if old == nil {
		t.Fatal("old token must still exist after a failed Save")
	}
	if old.UsedAt != nil {
		t.Fatal("old token must NOT be marked used when Save fails (would force-logout on retry)")
	}
	// Only the original token remains — the failed Save persisted nothing.
	fx.AssertTokenCount(t, 1)
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

// When the theft-response revocation (DeleteByUserID) itself fails, the handler
// must surface that RAW error (→ 5xx, cookie kept) rather than laundering it into
// an *AuthError — returning "reuse detected" while the tokens are still live
// would falsely tell the client it is logged out, and the next retry would
// re-attempt the revocation. The theft audit must STILL fire, because it is
// recorded before the delete is attempted. This locks in both halves of the
// surface-revocation-failure fix; without it a refactor could re-swallow the
// error and the gate would stay green.
func TestRefreshTokenHandler_TheftDeleteFailureSurfacesErrorAndStillAudits(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_theft_delete_fail.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")
	raw := fx.SeedRefreshToken(t, u.ID, time.Now().Add(24*time.Hour))

	// First rotation marks the raw token used (real repo, so it persists).
	if _, err := NewRefreshTokenHandler(fx.Users, fx.Tokens, fx.Jwt).
		Handle(ctx, RefreshTokenCommand{RawToken: raw}); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	// Reuse the already-rotated token, but with a repo whose DeleteByUserID fails.
	deleteBlip := errors.New("database is locked")
	handler := NewRefreshTokenHandler(
		fx.Users,
		stubDeleteFailsTokens{TokenRepository: fx.Tokens, err: deleteBlip},
		fx.Jwt,
	)
	collector := &shared.AuditCollector{}
	auditCtx := shared.ContextWithAuditCollector(ctx, collector)

	_, err := handler.Handle(auditCtx, RefreshTokenCommand{RawToken: raw})

	// The raw delete error must propagate (→ 5xx, cookie kept)...
	if !errors.Is(err, deleteBlip) {
		t.Fatalf("expected the raw delete error to propagate, got %T: %v", err, err)
	}
	// ...and must NOT be laundered into an *AuthError (which would clear the cookie).
	var authErr *shared.AuthError
	if errors.As(err, &authErr) {
		t.Fatalf("a failed revocation must NOT become *AuthError (would clear cookie), got %v", err)
	}
	// The theft was audited regardless — recorded BEFORE the delete was attempted.
	var sawTheft bool
	for _, e := range collector.Drain() {
		if e.Action == "auth.token.theft_detected" {
			sawTheft = true
		}
	}
	if !sawTheft {
		t.Fatal("theft must be audited even when the revocation delete fails")
	}
}
