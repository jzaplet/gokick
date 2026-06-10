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

// Closes app-events-audit-34 and roadmap-52: a triggered lock lasts exactly
// loginLockDuration (15m). The existing repository test
// (TestRecordFailedLogin_LocksAtThreshold) only asserts locked_until is set
// using an arbitrary time.Hour duration — it never pins the 15m policy nor
// the now+D mapping. Driving the real handler past the threshold and reading
// locked_until back from SQLite proves the constant flows through to the DB.
func TestLoginHandler_LockDurationIs15Minutes(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_lock_duration.db"))
	u := fx.SeedUser(t, "zoe", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	// Capture the wall clock immediately before the attempt that locks the
	// account so the expected window is tight.
	start := time.Now()
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "zoe", Password: "wrong"})
	}

	got, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if !got.LockedUntil.Valid {
		t.Fatal("account must be locked after threshold reached")
	}

	// Normalise both sides to UTC — SQLite round-trips can re-tag the zone,
	// and we only care about the instant, not the location.
	wantLowerBound := start.UTC().Add(loginLockDuration)
	wantUpperBound := time.Now().UTC().Add(loginLockDuration)
	gotLocked := got.LockedUntil.Time.UTC()

	if gotLocked.Before(wantLowerBound.Add(-time.Second)) ||
		gotLocked.After(wantUpperBound.Add(time.Second)) {
		t.Fatalf(
			"locked_until must be ~now+15m: got %s, expected within [%s, %s]",
			gotLocked, wantLowerBound, wantUpperBound,
		)
	}

	// Guard the magnitude explicitly so a future change of the constant to,
	// say, 15s or 15h is caught (a wide tolerance alone would not).
	if loginLockDuration != 15*time.Minute {
		t.Fatalf("loginLockDuration must be 15m, got %v", loginLockDuration)
	}
}

// countingHasher is a spy shared.PasswordHasher that tallies Verify calls.
// Hash is invoked once at handler construction for the dummy hash; only Verify
// is counted. verifyErr controls the outcome: a non-nil value simulates a wrong
// password, nil simulates a match — letting a test drive the locked branch
// (which requires Verify to SUCCEED) rather than only the wrong-password branch.
type countingHasher struct {
	verifyCalls int
	verifyErr   error
}

func (h *countingHasher) Hash(string) (string, error) { return "dummy-hash", nil }

func (h *countingHasher) Verify(string, string) error {
	h.verifyCalls++
	return h.verifyErr
}

// Closes roadmap-49 and roadmap-62: Login ALWAYS calls Verify (paying the
// bcrypt cost) before the lock decision, on both the unknown-user branch and
// the locked-account branch. This is the timing-uniformity guard. We inject a
// counting hasher and assert Verify fired exactly once per attempt; if the
// handler ever short-circuited the locked branch before Verify, the count
// would be 0 and this test fails. (Wall-clock timing is deliberately avoided —
// it would be flaky; counting Verify invocations is the deterministic proxy.)
func TestLoginHandler_AlwaysVerifiesOnUnknownAndLockedBranches(t *testing.T) {
	ctx := context.Background()

	// --- Unknown user (u == nil) ---
	t.Run("unknown user", func(t *testing.T) {
		fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_verify_unknown.db"))
		spy := &countingHasher{verifyErr: errors.New("mismatch")}
		handler := NewLoginHandler(fx.Users, fx.Tokens, spy, fx.Jwt)

		_, err := handler.Handle(ctx, LoginCommand{Nickname: "ghost", Password: "anything"})
		var authErr *shared.AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
		}
		if spy.verifyCalls != 1 {
			t.Fatalf("Verify must run once for an unknown user, got %d", spy.verifyCalls)
		}
	})

	// --- Locked account, correct password ---
	t.Run("locked account", func(t *testing.T) {
		fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_verify_locked.db"))
		u := fx.SeedUser(t, "lara", "correct-pw", "user")

		// Lock the account directly via the repository (the handler under
		// test uses the spy hasher, so we cannot lock it with wrong-password
		// handler calls). The lock can only fire once last_failed_login_at is
		// set, so with threshold 2 the SECOND call is the one that locks.
		if _, err := fx.Users.RecordFailedLogin(ctx, u.ID, 2, time.Minute, loginLockDuration); err != nil {
			t.Fatalf("force lock (1st): %v", err)
		}
		lockedAt, err := fx.Users.RecordFailedLogin(ctx, u.ID, 2, time.Minute, loginLockDuration)
		if err != nil {
			t.Fatalf("force lock (2nd): %v", err)
		}
		if lockedAt == nil {
			t.Fatal("setup: second RecordFailedLogin must report a lock")
		}

		// verifyErr nil → the password MATCHES, so Handle passes the
		// wrong-password check and genuinely reaches the locked branch instead of
		// short-circuiting on a spy that always rejects.
		spy := &countingHasher{}
		handler := NewLoginHandler(fx.Users, fx.Tokens, spy, fx.Jwt)

		// Even with the CORRECT password, a locked account returns AuthError —
		// but Verify must still have been paid first.
		_, err = handler.Handle(ctx, LoginCommand{Nickname: "lara", Password: "correct-pw"})
		var authErr *shared.AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("expected *shared.AuthError, got %T: %v", err, err)
		}
		if spy.verifyCalls != 1 {
			t.Fatalf("Verify must run before the lock check, got %d calls", spy.verifyCalls)
		}
	})
}

// Closes app-events-audit-38: the auth.login.blocked_while_locked event carries
// NO metadata. The existing TestLoginHandler_BlockedWhileLockedEmitsAudit
// asserts the Action and TargetID but never checks that Metadata is nil — that
// is the specific gap this test fills.
func TestLoginHandler_BlockedWhileLockedHasNoMetadata(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_blocked_no_meta.db"))
	u := fx.SeedUser(t, "quinn", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "quinn", Password: "wrong"})
	}

	collector := &shared.AuditCollector{}
	lockedCtx := shared.ContextWithAuditCollector(ctx, collector)
	if _, err := handler.Handle(lockedCtx, LoginCommand{Nickname: "quinn", Password: "correct-pw"}); err == nil {
		t.Fatal("locked account must return an error")
	}

	var blocked *shared.AuditEvent
	for _, e := range collector.Drain() {
		if e.Action == "auth.login.blocked_while_locked" {
			ev := e
			blocked = &ev
		}
	}
	if blocked == nil {
		t.Fatal("expected an auth.login.blocked_while_locked event")
	}
	if blocked.TargetID != u.ID {
		t.Fatalf("blocked target_id: got %q want %q", blocked.TargetID, u.ID)
	}
	if blocked.Metadata != nil {
		t.Fatalf("blocked_while_locked must carry no metadata, got %+v", blocked.Metadata)
	}
}

// Closes app-events-audit-39: the auth.account.locked event carries metadata
// {locked_until} formatted as an RFC3339 timestamp in the future. The existing
// TestLoginHandler_RecordsAccountLockedWhenThresholdReached only counts the
// event (== 1) and never inspects the locked_until metadata payload.
func TestLoginHandler_AccountLockedEventCarriesLockedUntilMetadata(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_locked_meta.db"))
	fx.SeedUser(t, "rosa", "correct-pw", "user")

	collector := &shared.AuditCollector{}
	auditCtx := shared.ContextWithAuditCollector(ctx, collector)

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)
	before := time.Now()
	for i := 0; i < loginLockThreshold; i++ {
		_, _ = handler.Handle(auditCtx, LoginCommand{Nickname: "rosa", Password: "wrong"})
	}

	var lockEvent *shared.AuditEvent
	for _, e := range collector.Drain() {
		if e.Action == "auth.account.locked" {
			ev := e
			lockEvent = &ev
		}
	}
	if lockEvent == nil {
		t.Fatal("expected an auth.account.locked event")
	}

	rawLockedUntil, ok := lockEvent.Metadata["locked_until"]
	if !ok {
		t.Fatalf(
			"auth.account.locked must carry locked_until metadata, got %+v",
			lockEvent.Metadata,
		)
	}
	lockedUntilStr, ok := rawLockedUntil.(string)
	if !ok {
		t.Fatalf("locked_until must be a string, got %T", rawLockedUntil)
	}
	parsed, err := time.Parse(time.RFC3339, lockedUntilStr)
	if err != nil {
		t.Fatalf("locked_until must be RFC3339: %v", err)
	}
	// The lock extends into the future (~15m out); at minimum it must be after
	// the moment we started driving the failures.
	if !parsed.After(before) {
		t.Fatalf("locked_until must be in the future: got %s, started at %s", parsed, before)
	}
}

// raceTokenRepo is a stub token.TokenRepository that simulates a concurrent
// rotation race: FindByHash returns a valid, unused, unexpired token, but
// MarkUsed reports (false, nil) — i.e. another request already flipped used_at
// first. Only the methods the refresh handler calls on this path are
// meaningful; the rest satisfy the interface.
type raceTokenRepo struct {
	tok            *token.RefreshToken
	deleteByUserID int
}

func (r *raceTokenRepo) Save(context.Context, *token.RefreshToken) error { return nil }

func (r *raceTokenRepo) FindByHash(context.Context, string) (*token.RefreshToken, error) {
	return r.tok, nil
}

func (r *raceTokenRepo) MarkUsed(context.Context, string) (bool, error) {
	// 0 rows updated → a concurrent request rotated this token first.
	return false, nil
}

func (r *raceTokenRepo) DeleteByUserID(context.Context, string) error {
	r.deleteByUserID++
	return nil
}

func (r *raceTokenRepo) DeleteExpired(context.Context) error { return nil }

// Closes the second half of roadmap-80: auth.token.theft_detected is emitted
// for TWO distinct reasons. TestRefreshTokenHandler_ReuseRecordsTheftAudit
// already covers reason "reused_after_rotation"; this covers the
// "concurrent_rotation_race" reason — when MarkUsed returns false because a
// parallel request rotated the same raw token first. We use a stub token repo
// so MarkUsed deterministically reports the race.
func TestRefreshTokenHandler_ConcurrentRotationRaceRecordsTheftAudit(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_race_theft.db"))
	u := fx.SeedUser(t, "sven", "pwd", "user")

	// A token that is unused (UsedAt == nil) and unexpired so the handler
	// reaches the MarkUsed step rather than the reuse/expiry branches.
	raw := "raw-refresh-token-value"
	stub := &raceTokenRepo{
		tok: &token.RefreshToken{
			ID:        "stub-token-id",
			UserID:    u.ID,
			TokenHash: fx.HashToken(raw),
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
			UsedAt:    nil,
		},
	}

	handler := NewRefreshTokenHandler(fx.Users, stub, fx.Jwt)

	collector := &shared.AuditCollector{}
	auditCtx := shared.ContextWithAuditCollector(ctx, collector)

	_, err := handler.Handle(auditCtx, RefreshTokenCommand{RawToken: raw})
	var authErr *shared.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *shared.AuthError on rotation race, got %T: %v", err, err)
	}
	// Force-logout must have been triggered.
	if stub.deleteByUserID != 1 {
		t.Fatalf("expected DeleteByUserID to revoke the session once, got %d", stub.deleteByUserID)
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
	if theft.Metadata["reason"] != "concurrent_rotation_race" {
		t.Fatalf(
			"theft reason metadata: got %v want concurrent_rotation_race",
			theft.Metadata["reason"],
		)
	}
}
