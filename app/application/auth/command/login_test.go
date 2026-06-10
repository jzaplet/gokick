package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/internal/testfx"
)

// saveFailsTokenRepo is a TokenRepository whose Save always fails; the other
// methods are never called on the success login path under test.
type saveFailsTokenRepo struct{ token.TokenRepository }

func (saveFailsTokenRepo) Save(context.Context, *token.RefreshToken) error {
	return errors.New("refresh token save failed")
}

func TestLoginHandler_Success(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_success.db"))
	u := fx.SeedUser(t, "alice", "super-secret", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	result, err := handler.Handle(ctx, LoginCommand{Nickname: "alice", Password: "super-secret"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if result.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}
	if result.User.ID != u.ID {
		t.Fatalf("user id mismatch: got %s want %s", result.User.ID, u.ID)
	}
	if result.AccessExpiresIn <= 0 {
		t.Fatalf("expected positive expiration, got %v", result.AccessExpiresIn)
	}
	if !result.RefreshExpiresAt.After(time.Now()) {
		t.Fatal("expected refresh token to expire in the future")
	}

	stored, err := fx.Tokens.FindByHash(ctx, fx.HashToken(result.RefreshToken))
	if err != nil {
		t.Fatalf("find token: %v", err)
	}
	if stored == nil {
		t.Fatal("expected refresh token persisted in DB")
	}
	if stored.UserID != u.ID {
		t.Fatalf("token user_id mismatch: got %s want %s", stored.UserID, u.ID)
	}
}

func TestLoginHandler_WrongPassword(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_wrong_pwd.db"))
	fx.SeedUser(t, "bob", "correct-password", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, err := handler.Handle(ctx, LoginCommand{Nickname: "bob", Password: "wrong-password"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestLoginHandler_UnknownUser(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_unknown.db"))

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, err := handler.Handle(ctx, LoginCommand{Nickname: "ghost", Password: "anything"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
	}
}

func TestLoginHandler_NoRefreshTokenOnFailure(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_no_token_on_fail.db"))
	fx.SeedUser(t, "charlie", "right", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "charlie", Password: "wrong"})

	fx.AssertTokenCount(t, 0)
}

// One bad password bumps the brute-force counter to 1. The handler still
// returns a neutral AuthError so the response shape gives nothing away.
func TestLoginHandler_FailedLoginIncrementsCounter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_inc.db"))
	u := fx.SeedUser(t, "dora", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "dora", Password: "wrong"})

	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 1 {
		t.Fatalf("counter: got %d want 1", got.FailedLoginAttempts)
	}
}

// Five failures inside the window should lock the account; the next
// login attempt — even with the correct password — must be rejected with
// the same neutral error.
func TestLoginHandler_LocksAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_lockout.db"))
	fx.SeedUser(t, "evan", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	// 5 bad attempts → counter resets to 0 on the 5th and lock kicks in.
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "evan", Password: "wrong"})
	}

	// Correct password but locked — still AuthError, response shape gives nothing away.
	_, err := handler.Handle(ctx, LoginCommand{Nickname: "evan", Password: "correct-pw"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("locked account must return *shared.AuthError, got %T: %v", err, err)
	}

	fx.AssertTokenCount(t, 0)
}

// Audit events are recorded into the collector exposed via context.
// Verifying both the success and the failure path here means the
// integration with bus middleware is the only remaining wiring concern.
func TestLoginHandler_RecordsSuccessfulLogin(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_audit_ok.db"))
	u := fx.SeedUser(t, "gina", "correct-pw", "user")

	collector := &shared.AuditCollector{}
	ctx = shared.ContextWithAuditCollector(ctx, collector)

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	if _, err := handler.Handle(ctx, LoginCommand{Nickname: "gina", Password: "correct-pw"}); err != nil {
		t.Fatalf("login: %v", err)
	}

	events := collector.Drain()
	if len(events) != 1 || events[0].Action != "auth.login.succeeded" ||
		events[0].TargetID != u.ID {
		t.Fatalf("expected 1 auth.login.succeeded for %s, got %+v", u.ID, events)
	}
}

func TestLoginHandler_RecordsFailedLoginWithNickname(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_audit_fail.db"))
	fx.SeedUser(t, "henry", "correct-pw", "user")

	collector := &shared.AuditCollector{}
	ctx = shared.ContextWithAuditCollector(ctx, collector)

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "henry", Password: "wrong"})

	events := collector.Drain()
	if len(events) != 1 || events[0].Action != "auth.login.failed" {
		t.Fatalf("expected 1 auth.login.failed, got %+v", events)
	}
	if events[0].Metadata["nickname"] != "henry" {
		t.Fatalf("metadata.nickname: %v", events[0].Metadata["nickname"])
	}
}

func TestLoginHandler_RecordsAccountLockedWhenThresholdReached(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_audit_lock.db"))
	fx.SeedUser(t, "ivan", "correct-pw", "user")

	collector := &shared.AuditCollector{}
	ctx = shared.ContextWithAuditCollector(ctx, collector)

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "ivan", Password: "wrong"})
	}

	var lockEvents int
	for _, e := range collector.Drain() {
		if e.Action == "auth.account.locked" {
			lockEvents++
		}
	}
	if lockEvents != 1 {
		t.Fatalf("expected exactly 1 auth.account.locked event, got %d", lockEvents)
	}
}

// Regression guard for the F4 self-deadlock: LoginHandler dispatched
// through the real CommandBus (which includes TransactionMiddleware)
// MUST NOT block. The handler's raw-pool writes (ResetFailedLogin)
// would otherwise wait forever for the SQLite write lock held by the
// wrapping tx. SkipsTransaction on LoginCommand is the safeguard;
// this test fails (deadline) if that opt-out is ever removed.
func TestLoginHandler_DoesNotDeadlockUnderCommandBus(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_deadlock.db"))
	fx.SeedUser(t, "jana", "secret123", "user")

	cmdBus, _, _ := fx.NewBuses()
	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	done := make(chan error, 1)
	go func() {
		_, err := testfx.ExecCommand(ctx, cmdBus, "Login",
			LoginCommand{Nickname: "jana", Password: "secret123"},
			func(ctx context.Context) (LoginResult, error) {
				return handler.Handle(ctx, LoginCommand{Nickname: "jana", Password: "secret123"})
			})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("login through bus: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("login deadlocked under CommandBus — SkipsTransaction is gone")
	}
}

// A successful login clears the counter so the next failure cycle
// starts fresh.
func TestLoginHandler_SuccessResetsCounter(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_reset.db"))
	u := fx.SeedUser(t, "frank", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	// Seed a couple of failures.
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "frank", Password: "wrong"})
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "frank", Password: "wrong"})

	pre, _ := fx.Users.FindByID(ctx, u.ID)
	if pre.FailedLoginAttempts != 2 {
		t.Fatalf("setup: expected counter=2, got %d", pre.FailedLoginAttempts)
	}

	if _, err := handler.Handle(ctx, LoginCommand{Nickname: "frank", Password: "correct-pw"}); err != nil {
		t.Fatalf("good login should succeed: %v", err)
	}

	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 0 {
		t.Fatalf("success must reset counter, got %d", got.FailedLoginAttempts)
	}
}

// A login attempt against a locked account with the CORRECT password is a
// neutral no-op that emits auth.login.blocked_while_locked (audit.md table +
// guide-auth-perm-36). Without this test the event has zero coverage.
func TestLoginHandler_BlockedWhileLockedEmitsAudit(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_blocked_audit.db"))
	u := fx.SeedUser(t, "mona", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "mona", Password: "wrong"})
	}

	// Correct password, but the account is locked → blocked_while_locked.
	collector := &shared.AuditCollector{}
	lockedCtx := shared.ContextWithAuditCollector(ctx, collector)
	_, err := handler.Handle(lockedCtx, LoginCommand{Nickname: "mona", Password: "correct-pw"})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("locked account must return *shared.AuthError, got %T: %v", err, err)
	}
	var blocked int
	for _, e := range collector.Drain() {
		if e.Action == "auth.login.blocked_while_locked" && e.TargetID == u.ID {
			blocked++
		}
	}
	if blocked != 1 {
		t.Fatalf("expected exactly 1 auth.login.blocked_while_locked, got %d", blocked)
	}
}

// While an account is locked, a further WRONG-password attempt must not bump
// the counter or extend the lock (auth.md: "počítadlo se nezvyšuje, lock se
// neprodlužuje"). handleFailedLogin short-circuits on the locked branch; this
// test is the only guard that the short-circuit stays.
func TestLoginHandler_WrongPasswordWhileLockedDoesNotExtendLock(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_locked_noextend.db"))
	u := fx.SeedUser(t, "nina", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "nina", Password: "wrong"})
	}

	locked, _ := fx.Users.FindByID(ctx, u.ID)
	if !locked.LockedUntil.Valid {
		t.Fatal("setup: account should be locked after threshold")
	}
	beforeCounter := locked.FailedLoginAttempts
	beforeLockedUntil := locked.LockedUntil.Time

	// One more wrong attempt while locked.
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "nina", Password: "wrong"})

	after, _ := fx.Users.FindByID(ctx, u.ID)
	if after.FailedLoginAttempts != beforeCounter {
		t.Fatalf(
			"counter must not change while locked: got %d want %d",
			after.FailedLoginAttempts,
			beforeCounter,
		)
	}
	if !after.LockedUntil.Time.Equal(beforeLockedUntil) {
		t.Fatalf(
			"lock must not be extended while locked: got %v want %v",
			after.LockedUntil.Time,
			beforeLockedUntil,
		)
	}
}

// Regression: a login with valid credentials that fails to ISSUE the token
// (GenerateAccessToken / refresh Save) must NOT leave an auth.login.succeeded
// row. The event means "an access-granting login happened" (audit.md); because
// login is SkipTransaction and AuditMiddleware flushes even on handler error,
// recording success before the token was persisted would forge a "succeeded"
// trail for a login that returned 500 and granted nothing.
func TestLoginHandler_NoSuccessAuditWhenTokenSaveFails(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "login_save_fails.db"))
	fx.SeedUser(t, "olga", "correct-pw", "user")

	collector := &shared.AuditCollector{}
	ctx = shared.ContextWithAuditCollector(ctx, collector)

	handler := NewLoginHandler(fx.Users, saveFailsTokenRepo{}, fx.Hasher, fx.Jwt)
	if _, err := handler.Handle(ctx, LoginCommand{Nickname: "olga", Password: "correct-pw"}); err == nil {
		t.Fatal("expected the refresh-token Save failure to propagate")
	}

	for _, e := range collector.Drain() {
		if e.Action == "auth.login.succeeded" {
			t.Fatal("must NOT record auth.login.succeeded when no token was issued")
		}
	}
}
