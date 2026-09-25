package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gokick/app/domain/shared"
)

// lockTable is the database side of fakeLocker: which replica holds which lock.
type lockTable struct {
	mu    sync.Mutex
	owner map[string]*fakeLocker
}

func newLockTable() *lockTable { return &lockTable{owner: map[string]*fakeLocker{}} }

// fakeLocker is one replica's Locker over a shared lockTable — what the
// Postgres Locker does, without a database: a lock belongs to the replica that
// took it until it releases it.
type fakeLocker struct{ table *lockTable }

func (l *fakeLocker) Hold(_ context.Context, key string) (bool, error) {
	l.table.mu.Lock()
	defer l.table.mu.Unlock()
	if holder, taken := l.table.owner[key]; taken {
		return holder == l, nil
	}
	l.table.owner[key] = l
	return true, nil
}

func (l *fakeLocker) Release(_ context.Context, key string) error {
	l.table.mu.Lock()
	defer l.table.mu.Unlock()
	if l.table.owner[key] == l {
		delete(l.table.owner, key)
	}
	return nil
}

func (l *lockTable) held() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.owner)
}

// replica is one running Scheduler and the number of times it ran the job.
type replica struct {
	runs   atomic.Int32
	cancel context.CancelFunc
	done   chan struct{}
}

// startReplica runs a Scheduler with one job of the given interval under locker.
func startReplica(
	t *testing.T,
	logger *slog.Logger,
	locker shared.Locker,
	interval time.Duration,
) *replica {
	t.Helper()
	r := &replica{done: make(chan struct{})}
	s, err := NewScheduler(logger, locker, []Job{
		{Name: "cleanup", Interval: interval, Fn: func(context.Context) error {
			r.runs.Add(1)
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() {
		defer close(r.done)
		s.Run(ctx)
	}()
	t.Cleanup(r.stop)
	return r
}

// stop cancels the replica and waits for its Scheduler to drain.
func (r *replica) stop() {
	r.cancel()
	<-r.done
}

// eventually polls cond for up to a second.
func eventually(cond func() bool) bool {
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// Two replicas with the same job: only the one holding the job's lock runs it.
// A long Interval leaves just the run-once tick of each, so without the lock the
// job would run twice.
func TestScheduler_TwoReplicasRunAJobOnce(t *testing.T) {
	t.Parallel()
	table := newLockTable()
	a := startReplica(t, silentLogger(), &fakeLocker{table}, time.Hour)
	b := startReplica(t, silentLogger(), &fakeLocker{table}, time.Hour)

	if !eventually(func() bool { return a.runs.Load()+b.runs.Load() > 0 }) {
		t.Fatal("neither replica ran the job")
	}
	time.Sleep(40 * time.Millisecond) // room for a wrong second run
	if got := a.runs.Load() + b.runs.Load(); got != 1 {
		t.Fatalf("the job ran %d times across two replicas, want once", got)
	}
}

// The lock stays with its holder between runs — the other replica never runs
// the job while the holder lives — and when the holder stops, it releases the
// lock and the other replica takes the job over on its next tick.
func TestScheduler_ReplicaTakesTheJobOverWhenTheHolderStops(t *testing.T) {
	t.Parallel()
	table := newLockTable()
	holder := startReplica(t, silentLogger(), &fakeLocker{table}, 10*time.Millisecond)
	if !eventually(func() bool { return holder.runs.Load() > 0 }) {
		t.Fatal("the first replica never ran the job")
	}
	other := startReplica(t, silentLogger(), &fakeLocker{table}, 10*time.Millisecond)

	time.Sleep(60 * time.Millisecond) // several ticks of both
	if got := other.runs.Load(); got != 0 {
		t.Fatalf("the second replica ran the job %d times while the holder lived", got)
	}
	if holder.runs.Load() < 2 {
		t.Fatal("the holder must keep running the job every interval")
	}

	holder.stop()
	if !eventually(func() bool { return other.runs.Load() > 0 }) {
		t.Fatal("the second replica did not take the job over after the holder stopped")
	}
}

// Without a lock to share — one replica per database, as on SQLite — each
// Scheduler runs its jobs itself.
func TestScheduler_GrantAllLockerRunsEveryReplica(t *testing.T) {
	t.Parallel()
	a := startReplica(t, silentLogger(), grantAll{}, time.Hour)
	b := startReplica(t, silentLogger(), grantAll{}, time.Hour)
	if !eventually(func() bool { return a.runs.Load() == 1 && b.runs.Load() == 1 }) {
		t.Fatalf("runs a=%d b=%d, want each replica to run the job once",
			a.runs.Load(), b.runs.Load())
	}
}

// A stopped Scheduler leaves no lock behind.
func TestScheduler_ReleasesItsLocksWhenItStops(t *testing.T) {
	t.Parallel()
	table := newLockTable()
	r := startReplica(t, silentLogger(), &fakeLocker{table}, time.Hour)
	if !eventually(func() bool { return r.runs.Load() > 0 }) {
		t.Fatal("the job never ran")
	}
	r.stop()
	if n := table.held(); n != 0 {
		t.Fatalf("%d locks still held after the Scheduler stopped", n)
	}
}

// failingLocker cannot reach its database.
type failingLocker struct{ holds atomic.Int32 }

func (l *failingLocker) Hold(context.Context, string) (bool, error) {
	l.holds.Add(1)
	return false, errors.New("database unreachable")
}

func (*failingLocker) Release(context.Context, string) error { return nil }

// A lock that cannot be checked is not held: the job does not run, the failure
// is logged, and the next tick tries again.
func TestScheduler_SkipsTheJobWhenTheLockFails(t *testing.T) {
	t.Parallel()
	buf := &lockedBuffer{}
	locker := &failingLocker{}
	r := startReplica(t, captureLogger(buf), locker, 10*time.Millisecond)
	ok := eventually(func() bool { return locker.holds.Load() >= 2 })
	r.stop()

	if !ok {
		t.Fatal("the Scheduler stopped trying the lock after a failure")
	}
	if n := r.runs.Load(); n != 0 {
		t.Fatalf("the job ran %d times without its lock", n)
	}
	if !strings.Contains(buf.String(), "scheduler: job lock failed") {
		t.Fatalf("the lock failure was not logged:\n%s", buf.String())
	}
}

func TestNewScheduler_RequiresALocker(t *testing.T) {
	t.Parallel()
	if _, err := NewScheduler(silentLogger(), nil, nil); err == nil {
		t.Fatal("a Scheduler without a Locker must be refused")
	}
}
