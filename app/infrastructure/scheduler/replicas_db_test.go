package scheduler

import (
	"testing"
	"time"

	"gokick/app/infrastructure/database"
	"gokick/app/internal/testfx"
)

// Two replicas on one Postgres database, each with its own connection pools and
// Locker: the job runs on one of them only, and the other takes it over once the
// first stops. SQLite is served by one process, and its Locker grants every
// lock; the fake-locker tests cover the Scheduler's side on any database.
func TestScheduler_ReplicasOnOneDatabaseRunAJobOnce(t *testing.T) {
	if testfx.ActiveDriver() != database.DriverPostgres {
		t.Skip("SQLite is served by one process: its Locker grants every lock")
	}
	fx := testfx.New(t)

	holder := startReplica(t, silentLogger(), fx.Locker, 10*time.Millisecond)
	if !eventually(func() bool { return holder.runs.Load() > 0 }) {
		t.Fatal("the first replica never ran the job")
	}
	other := startReplica(t, silentLogger(), fx.Replica(t).Locker, 10*time.Millisecond)

	time.Sleep(80 * time.Millisecond) // several ticks of both
	if got := other.runs.Load(); got != 0 {
		t.Fatalf("the second replica ran the job %d times while the first held it", got)
	}

	holder.stop()
	if !eventually(func() bool { return other.runs.Load() > 0 }) {
		t.Fatal("the second replica did not take the job over after the first stopped")
	}
}
