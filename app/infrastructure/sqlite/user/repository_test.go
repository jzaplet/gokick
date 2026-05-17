package user_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gokick/app/internal/testfx"
)

func TestRecordFailedLogin_IncrementsBelowThreshold(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lock_inc.db"))
	u := fx.SeedUser(t, "alice", "secret12", "user")

	locked, err := fx.Users.RecordFailedLogin(ctx, u.ID, 5, time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("RecordFailedLogin: %v", err)
	}
	if locked != nil {
		t.Fatalf("should not be locked after 1 failure, got %v", locked)
	}
	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 1 {
		t.Fatalf("counter: got %d want 1", got.FailedLoginAttempts)
	}
	if got.LockedUntil.Valid {
		t.Fatal("LockedUntil must be NULL below threshold")
	}
}

func TestRecordFailedLogin_LocksAtThreshold(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lock_threshold.db"))
	u := fx.SeedUser(t, "alice", "secret12", "user")

	threshold := 3
	for i := 0; i < threshold-1; i++ {
		if _, err := fx.Users.RecordFailedLogin(ctx, u.ID, threshold, time.Minute, time.Hour); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != threshold-1 {
		t.Fatalf("pre-lock counter: got %d want %d", got.FailedLoginAttempts, threshold-1)
	}
	if got.LockedUntil.Valid {
		t.Fatal("must not lock before threshold")
	}

	// Threshold attempt → locked, counter reset.
	locked, err := fx.Users.RecordFailedLogin(ctx, u.ID, threshold, time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("trigger lock: %v", err)
	}
	if locked == nil {
		t.Fatal("threshold attempt must return non-nil locked_until")
	}
	got, _ = fx.Users.FindByID(ctx, u.ID)
	if !got.LockedUntil.Valid {
		t.Fatal("LockedUntil must be set after threshold reached")
	}
	if got.FailedLoginAttempts != 0 {
		t.Fatalf("counter must reset to 0 after lock, got %d", got.FailedLoginAttempts)
	}
}

func TestRecordFailedLogin_ResetsCounterOutsideWindow(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lock_window.db"))
	u := fx.SeedUser(t, "alice", "secret12", "user")

	// First failure with a tiny window so the next failure is "outside" it.
	if _, err := fx.Users.RecordFailedLogin(ctx, u.ID, 5, time.Millisecond, time.Hour); err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Second failure, also with window=1ms → should reset to 1, not 2.
	if _, err := fx.Users.RecordFailedLogin(ctx, u.ID, 5, time.Millisecond, time.Hour); err != nil {
		t.Fatalf("second: %v", err)
	}
	got, _ := fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 1 {
		t.Fatalf("counter must reset after window: got %d want 1", got.FailedLoginAttempts)
	}
}

func TestResetFailedLogin_ClearsCounterAndLock(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "lock_reset.db"))
	u := fx.SeedUser(t, "alice", "secret12", "user")

	// Push to threshold so both counter and lock are populated, then reset.
	for i := 0; i < 3; i++ {
		_, _ = fx.Users.RecordFailedLogin(ctx, u.ID, 3, time.Minute, time.Hour)
	}
	got, _ := fx.Users.FindByID(ctx, u.ID)
	if !got.LockedUntil.Valid {
		t.Fatal("setup: account should be locked")
	}

	if err := fx.Users.ResetFailedLogin(ctx, u.ID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	got, _ = fx.Users.FindByID(ctx, u.ID)
	if got.FailedLoginAttempts != 0 || got.LockedUntil.Valid {
		t.Fatalf("after reset: counter=%d locked_until=%v",
			got.FailedLoginAttempts, got.LockedUntil)
	}
}
