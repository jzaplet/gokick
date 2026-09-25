package user_test

import (
	"context"
	"testing"
	"time"

	"gokick/app/domain/user"
	"gokick/app/internal/testfx"
)

// These tests pin the raw-connection-pool behaviour of RecordFailedLogin and
// ResetFailedLogin (claims app-events-audit-29, guide-auth-perm-37, roadmap-71):
// both methods write on the pool rather than on the tx-aware connection, so their
// writes autocommit on a separate connection and SURVIVE a rollback of whatever
// business transaction is in the caller's context.
//
// The discriminator is cross-connection contention. The open outer tx writes the
// user row on connection A and so holds a lock the method's write needs — on
// SQLite the database write lock (BEGIN IMMEDIATE), on Postgres the row lock. The
// method, using the raw pool, checks out connection B and waits for that lock (a
// synchronous call would wait out the lock timeout and fail). So the only way to
// observe the write completing is to run it concurrently and release the outer
// lock (via rollback) while it is pending; the raw-pool write then autocommits
// independently of that rollback.
//
// If either method regressed to r.Conn(ctx) it would instead join the outer tx
// and the write would vanish with the rollback (counter unchanged), failing
// these tests. A no-tx "counter increments" assertion would NOT discriminate —
// raw-pool and tx-aware behave identically without a tx — which is why these
// tests deliberately pass the tx-carrying context into the method under test.

// beginWriteLockedTx opens a real transaction and makes it hold the lock on the
// user row by writing that row through the tx-aware connection. Returns the
// tx-carrying context. Any goroutine that subsequently writes the row via the raw
// pool will block on this lock until the tx is committed or rolled back.
func beginWriteLockedTx(t *testing.T, fx *testfx.Fixture, userID string) context.Context {
	t.Helper()
	txCtx, err := fx.Tx.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	// Touch the row through the tx so the lock is unambiguously held by THIS
	// connection (and so we exercise the same row the method writes).
	if err := fx.Users.Update(txCtx, mustFindByID(t, fx, userID), ""); err != nil {
		t.Fatalf("write inside tx to take lock: %v", err)
	}
	return txCtx
}

func mustFindByID(t *testing.T, fx *testfx.Fixture, userID string) *user.User {
	t.Helper()
	u, err := fx.Users.FindByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	return u
}

// TestRecordFailedLogin_SurvivesOuterTxRollback proves RecordFailedLogin writes
// on the raw pool: invoked with a tx-carrying context whose outer tx is then
// rolled back, the incremented counter still persists.
func TestRecordFailedLogin_SurvivesOuterTxRollback(t *testing.T) {
	fx := testfx.New(t)
	u := fx.SeedUser(t, "alice", "secret12", "user")

	// Outer tx takes the write lock.
	txCtx := beginWriteLockedTx(t, fx, u.ID)

	// Fire RecordFailedLogin WITH the tx-carrying context. Because it uses the
	// raw pool (separate connection), it blocks on the outer write lock instead
	// of joining the tx — that blocking is the proof it isn't tx-aware.
	type result struct {
		locked *time.Time
		err    error
	}
	done := make(chan result, 1)
	go func() {
		locked, err := fx.Users.RecordFailedLogin(txCtx, u.ID, 5, time.Minute, time.Hour)
		done <- result{locked: locked, err: err}
	}()

	// Let the goroutine reach (and block on) the write lock, then release it by
	// rolling the outer tx back. The pending raw-pool write now proceeds and
	// autocommits on its own connection.
	time.Sleep(150 * time.Millisecond)
	if err := fx.Tx.Rollback(txCtx); err != nil {
		t.Fatalf("rollback outer tx: %v", err)
	}

	var res result
	select {
	case res = <-done:
	case <-time.After(8 * time.Second):
		t.Fatal(
			"RecordFailedLogin did not complete after outer tx rollback (the lock wait would be ~5s)",
		)
	}
	if res.err != nil {
		t.Fatalf("RecordFailedLogin returned error after rollback: %v", res.err)
	}

	// Re-read on a CLEAN context (FindByID is tx-aware; the rolled-back txCtx
	// would error). The counter increment must have survived the rollback.
	got := mustFindByID(t, fx, u.ID)
	if got.FailedLoginAttempts != 1 {
		t.Fatalf(
			"counter must survive outer-tx rollback (raw pool): got %d want 1",
			got.FailedLoginAttempts,
		)
	}
}

// TestResetFailedLogin_SurvivesOuterTxRollback proves ResetFailedLogin likewise
// writes on the raw pool: with a non-zero counter seeded, calling it inside a
// tx-carrying context whose tx is rolled back still leaves the counter cleared.
func TestResetFailedLogin_SurvivesOuterTxRollback(t *testing.T) {
	fx := testfx.New(t)
	u := fx.SeedUser(t, "alice", "secret12", "user")

	// Seed a non-zero counter via a plain-ctx call (no tx → ordinary raw write).
	if _, err := fx.Users.RecordFailedLogin(context.Background(), u.ID, 5, time.Minute, time.Hour); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
	if got := mustFindByID(t, fx, u.ID); got.FailedLoginAttempts != 1 {
		t.Fatalf("precondition: counter should be 1 before reset, got %d", got.FailedLoginAttempts)
	}

	// Outer tx takes the write lock.
	txCtx := beginWriteLockedTx(t, fx, u.ID)

	done := make(chan error, 1)
	go func() {
		done <- fx.Users.ResetFailedLogin(txCtx, u.ID)
	}()

	time.Sleep(150 * time.Millisecond)
	if err := fx.Tx.Rollback(txCtx); err != nil {
		t.Fatalf("rollback outer tx: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResetFailedLogin returned error after rollback: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("ResetFailedLogin did not complete after outer tx rollback")
	}

	got := mustFindByID(t, fx, u.ID)
	if got.FailedLoginAttempts != 0 {
		t.Fatalf(
			"counter clear must survive outer-tx rollback (raw pool): got %d want 0",
			got.FailedLoginAttempts,
		)
	}
}
