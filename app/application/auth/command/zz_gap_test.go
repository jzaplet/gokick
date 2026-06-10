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

// Closes guide-auth-perm-32 (boundary half): the account locks on exactly the
// FIFTH failure within the window, not the fourth. Existing tests
// (TestLoginHandler_LocksAfterFiveFailures, TestLoginHandler_LockDurationIs15Minutes)
// only drive loginLockThreshold failures and assert the account IS locked —
// none assert that ONE FEWER attempt leaves it UNlocked, so the off-by-one
// boundary is unguarded.
//
// The drive below uses LITERAL counts (4 then a 5th), not the loginLockThreshold
// symbol: a loop bounded by the symbol would move WITH a mutated constant and
// hide the very off-by-one we want to catch. The explicit magnitude guard
// (loginLockThreshold == 5) then nails the constant directly — mirroring how
// TestLoginHandler_LockDurationIs15Minutes guards the 15m duration. Together
// they fail if login.go:55 changes 5→4 (locks one attempt too early) or 5→6
// (the 5th no longer locks).
func TestLoginHandler_LocksOnFifthNotFourthFailure(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_lock_boundary.db"))
	u := fx.SeedUser(t, "tess", "correct-pw", "user")

	handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

	// Guard the constant explicitly so the literal counts below stay meaningful
	// and a change to the threshold is caught even though the drive is literal.
	if loginLockThreshold != 5 {
		t.Fatalf("loginLockThreshold must be 5 for this boundary test, got %d", loginLockThreshold)
	}

	// Four wrong-password attempts — one short of the threshold.
	for i := 0; i < 4; i++ {
		_, _ = handler.Handle(ctx, LoginCommand{Nickname: "tess", Password: "wrong"})
	}

	beforeLock, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("find user after 4 failures: %v", err)
	}
	if beforeLock.LockedUntil.Valid {
		t.Fatal("account must NOT be locked after 4 failures (threshold is the 5th)")
	}
	// Counter climbs one-per-failure right up to the threshold edge.
	if beforeLock.FailedLoginAttempts != 4 {
		t.Fatalf("counter after 4 failures: got %d want 4", beforeLock.FailedLoginAttempts)
	}

	// The fifth failure is the one that locks the account.
	_, _ = handler.Handle(ctx, LoginCommand{Nickname: "tess", Password: "wrong"})

	afterLock, err := fx.Users.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("find user after 5 failures: %v", err)
	}
	if !afterLock.LockedUntil.Valid {
		t.Fatal("account must be locked after the 5th failure")
	}
	// locked_until is in the future at the moment we read it back.
	if !afterLock.LockedUntil.Time.After(time.Now()) {
		t.Fatalf(
			"locked_until must be in the future, got %s (now ~%s)",
			afterLock.LockedUntil.Time, time.Now(),
		)
	}
}

// Closes guide-auth-perm-33 (no-oracle): a locked account given the CORRECT
// password returns the byte-identical neutral error that the wrong-password
// branch returns — so the response gives an attacker no way to distinguish
// "right password but locked" from "wrong password". Existing tests
// (TestLoginHandler_WrongPassword, TestLoginHandler_LocksAfterFiveFailures)
// only assert each branch returns *some* *shared.AuthError; neither asserts the
// two messages are EQUAL. We deliberately do NOT hardcode "invalid credentials":
// capturing both branch messages and asserting equality pins the anti-oracle
// property itself. Mutating only one branch's literal (e.g. login.go:125
// "invalid credentials" → "account locked") diverges the strings and fails this
// test; a coordinated rename of both literals survives (correct — the property
// is "the two are indistinguishable", not the exact wording).
func TestLoginHandler_LockedAndWrongPasswordReturnIdenticalError(t *testing.T) {
	ctx := context.Background()

	// --- Branch A: known user, WRONG password (account not locked). ---
	wrongPwMsg := func() string {
		fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_oracle_wrongpw.db"))
		fx.SeedUser(t, "ulla", "correct-pw", "user")
		handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

		_, err := handler.Handle(ctx, LoginCommand{Nickname: "ulla", Password: "wrong"})
		var authErr *shared.AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("wrong-password branch must return *shared.AuthError, got %T: %v", err, err)
		}
		return authErr.Error()
	}()

	// --- Branch B: known user, CORRECT password, but account is LOCKED. ---
	lockedMsg := func() string {
		fx := testfx.New(t, filepath.Join(t.TempDir(), "zz_oracle_locked.db"))
		fx.SeedUser(t, "ulla", "correct-pw", "user")
		handler := NewLoginHandler(fx.Users, fx.Tokens, fx.Hasher, fx.Jwt)

		// Lock the account by exhausting the threshold with wrong passwords.
		for i := 0; i < loginLockThreshold; i++ {
			_, _ = handler.Handle(ctx, LoginCommand{Nickname: "ulla", Password: "wrong"})
		}

		// Correct password now — but the account is in cooldown.
		_, err := handler.Handle(ctx, LoginCommand{Nickname: "ulla", Password: "correct-pw"})
		var authErr *shared.AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("locked branch must return *shared.AuthError, got %T: %v", err, err)
		}
		return authErr.Error()
	}()

	if lockedMsg != wrongPwMsg {
		t.Fatalf(
			"locked-with-correct-password error must be identical to the wrong-password error "+
				"(no oracle): locked=%q wrong=%q",
			lockedMsg, wrongPwMsg,
		)
	}
}
