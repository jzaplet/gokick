package postgres_test

import (
	"context"
	"testing"
	"time"

	"gokick/app/infrastructure/postgres"
)

// replicas returns the Lockers of two processes on one database: each over a
// Manager — and so a system pool — of its own, the way two `serve` replicas are.
func replicas(t *testing.T) (a, b *postgres.Locker, admin *postgres.Manager) {
	t.Helper()
	db, mgrA := migrated(t)
	mgrB := newManager(t, db.Config())
	return postgres.NewLocker(mgrA), postgres.NewLocker(mgrB), mgrB
}

func hold(t *testing.T, l *postgres.Locker, key string) bool {
	t.Helper()
	held, err := l.Hold(context.Background(), key)
	if err != nil {
		t.Fatalf("Hold(%s): %v", key, err)
	}
	return held
}

// thisDatabase narrows a pg_locks query to the test's own database: pg_locks
// shows the whole cluster, where other test packages hold locks in databases of
// their own at the same time.
const thisDatabase = `database = (SELECT oid FROM pg_database WHERE datname = current_database())`

// advisoryLocks counts the Locker locks held on the database, by any session.
func advisoryLocks(t *testing.T, mgr *postgres.Manager) int {
	t.Helper()
	return count(t, mgr.System(), `SELECT COUNT(*) FROM pg_locks
		WHERE locktype = 'advisory' AND classid = 7437261 AND granted AND `+thisDatabase)
}

// A lock one replica holds is refused to the other, stays with its holder
// between checks, and passes over once released. Different keys are independent.
func TestLocker_ExcludesTheOtherReplica(t *testing.T) {
	a, b, _ := replicas(t)
	ctx := context.Background()

	if !hold(t, a, "job:cleanup") {
		t.Fatal("the first replica must get a free lock")
	}
	if hold(t, b, "job:cleanup") {
		t.Fatal("the second replica got a lock the first one holds")
	}
	if !hold(t, a, "job:cleanup") {
		t.Fatal("the holder must keep its lock between checks")
	}
	if !hold(t, b, "job:report") {
		t.Fatal("a lock on another key must be free")
	}

	if err := a.Release(ctx, "job:cleanup"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if !hold(t, b, "job:cleanup") {
		t.Fatal("a released lock must pass to the other replica")
	}
	if hold(t, a, "job:cleanup") {
		t.Fatal("the first replica got back a lock the second one now holds")
	}
}

// When the holder's session ends — the process died, the connection broke —
// Postgres drops its locks: the other replica takes the lock over, and the old
// holder finds out on its next check instead of running on.
func TestLocker_LostSessionHandsTheLockOver(t *testing.T) {
	a, b, admin := replicas(t)
	if !hold(t, a, "job:cleanup") {
		t.Fatal("the first replica must get a free lock")
	}

	// Both replicas' system pools log in as the same role, which may end its own
	// sessions.
	if _, err := admin.System().Exec(`SELECT pg_terminate_backend(pid) FROM pg_locks
		WHERE locktype = 'advisory' AND classid = 7437261 AND ` + thisDatabase); err != nil {
		t.Fatalf("terminate the holder's session: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !hold(t, b, "job:cleanup") {
		if time.Now().After(deadline) {
			t.Fatal("the lock of an ended session never passed to the other replica")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hold(t, a, "job:cleanup") {
		t.Fatal("the old holder still claims a lock it lost with its session")
	}
}

// A Locker returns its connection to the pool only once it holds no lock, and
// with no lock on it — a pooled connection must never carry a lock to the next
// borrower.
func TestLocker_LeavesNoLockBehind(t *testing.T) {
	a, b, admin := replicas(t)
	ctx := context.Background()

	for _, key := range []string{"job:one", "job:two"} {
		if !hold(t, a, key) {
			t.Fatalf("Hold(%s) on a free lock failed", key)
		}
	}
	if err := a.Release(ctx, "job:one"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if n := advisoryLocks(t, admin); n != 1 {
		t.Fatalf("%d locks held, want job:two still held", n)
	}
	if err := a.Release(ctx, "job:two"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if n := advisoryLocks(t, admin); n != 0 {
		t.Fatalf("%d locks held after releasing everything, want none", n)
	}

	// A lock another replica holds is not taken — and the refused Locker does
	// not keep a connection pinned for it.
	if !hold(t, b, "job:one") || hold(t, a, "job:one") {
		t.Fatal("the lock must go to the replica that asked first")
	}
	if err := a.Release(ctx, "job:one"); err != nil {
		t.Fatalf("releasing a lock not held must be a no-op: %v", err)
	}
	if !hold(t, b, "job:one") {
		t.Fatal("a no-op release must not touch the other replica's lock")
	}
}

// A Hold whose context has already ended — a Scheduler stopping — reports the
// error without touching the session: the Locker's other locks, which may guard
// a job still running, stay held and out of the other replica's reach.
func TestLocker_KeepsItsLocksWhenAHoldFails(t *testing.T) {
	a, b, admin := replicas(t)
	for _, key := range []string{"job:one", "job:two"} {
		if !hold(t, a, key) {
			t.Fatalf("Hold(%s) on a free lock failed", key)
		}
	}

	ended, cancel := context.WithCancel(context.Background())
	cancel()
	for _, key := range []string{"job:one", "job:three"} { // a check, then a take
		if _, err := a.Hold(ended, key); err == nil {
			t.Fatalf("Hold(%s) with an ended context must report its error", key)
		}
	}

	if n := advisoryLocks(t, admin); n != 2 {
		t.Fatalf("%d locks held, want both kept after the failed Holds", n)
	}
	if hold(t, b, "job:one") || hold(t, b, "job:two") {
		t.Fatal("the other replica took a lock the holder still holds")
	}
	if !hold(t, a, "job:one") || !hold(t, a, "job:two") {
		t.Fatal("the holder lost its locks to a failed Hold")
	}
}
