package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"

	"github.com/google/uuid"
)

// countingReporter counts Capture calls (to assert exactly-once terminal reports);
// it inherits the other no-op methods from NopReporter.
type countingReporter struct {
	shared.NopReporter
	captures atomic.Int32
}

func (c *countingReporter) Capture(context.Context, error, ...slog.Attr) { c.captures.Add(1) }

func (c *countingReporter) Count() int { return int(c.captures.Load()) }

func fastCfg() RunWorkerConfig {
	return RunWorkerConfig{
		DefaultLease:      3 * time.Second,
		HeartbeatInterval: 40 * time.Millisecond,
		PollInterval:      10 * time.Millisecond,
		DrainTimeout:      3 * time.Second,
		MaxInFlight:       4,
		MaxReclaims:       2,
	}
}

func newRunWorker(
	t *testing.T,
	fx *testfx.Fixture,
	regs map[string]runapp.Registration,
	cfg RunWorkerConfig,
) (*RunWorker, *countingReporter) {
	t.Helper()
	reg, err := runapp.NewHandlerRegistry(regs, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	reporter := &countingReporter{}
	return NewRunWorker(silentLogger(), reporter, fx.Runs, reg, cfg), reporter
}

func enqueueRunW(t *testing.T, fx *testfx.Fixture, kind string, maxRetries int) *run.Run {
	t.Helper()
	r := run.NewRun(kind, []byte(`{}`), maxRetries)
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return r
}

func findW(t *testing.T, fx *testfx.Fixture, id string) *run.Run {
	t.Helper()
	r, err := fx.Runs.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	return r
}

func forceExpireW(t *testing.T, fx *testfx.Fixture, id string) {
	t.Helper()
	if _, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET locked_until = strftime('%Y-%m-%d %H:%M:%f','now','-1 hour') WHERE id = ?`, id); err != nil {
		t.Fatalf("force-expire: %v", err)
	}
}

// startWorker runs w in the background and returns a stop func that cancels it
// and waits for Run to return.
func startWorker(w *RunWorker) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	return func() { cancel(); <-done }
}

// waitFor polls cond until true or fails after 8s (generous for -race).
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(8 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for: %s", what)
		case <-time.After(15 * time.Millisecond):
		}
	}
}

// ─── Happy path ───────────────────────────────────────────────────────────────

func TestRunWorker_Success_CompletesAndCheckpoints(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_success.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return ck.Save(ctx, []byte(`{"done":true}`))
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "run completed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt != nil
	})
	got := findW(t, fx, r.ID)
	if string(got.State) != `{"done":true}` {
		t.Fatalf("checkpointed state not persisted: %q", got.State)
	}
	if got.LockedBy != nil || got.LockedUntil != nil {
		t.Fatal("completed run must have its lock cleared")
	}
}

func TestRunWorker_ResumesFromCheckpoint(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_resume.db")
	var seen atomic.Value
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		seen.Store(string(r.State))
		return nil
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	// Pre-seed a run that already has checkpoint state (as if a prior worker crashed).
	r := run.NewRun("agent", []byte(`{}`), 3)
	r.State = []byte(`{"step":5}`)
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	stop := startWorker(w)
	defer stop()
	waitFor(t, "run completed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt != nil
	})
	if seen.Load() != `{"step":5}` {
		t.Fatalf("handler must resume from checkpoint state, saw %v", seen.Load())
	}
}

// ─── Failure / retry ──────────────────────────────────────────────────────────

func TestRunWorker_Failure_NoRetries_MarksFailed(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_fail.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return errors.New("boom")
	}
	w, reporter := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 0) // no retries → first failure is terminal

	stop := startWorker(w)
	defer stop()
	waitFor(t, "run failed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
	got := findW(t, fx, r.ID)
	if got.LastError == nil || *got.LastError != "boom" {
		t.Fatalf("last_error: %v", got.LastError)
	}
	if c := reporter.Count(); c != 1 {
		t.Fatalf("a terminal failure must be reported exactly once, got %d", c)
	}
}

func TestRunWorker_Failure_WithRetries_Reschedules(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_retry.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return errors.New("transient")
	}
	w, reporter := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3) // retries available → first failure reschedules

	stop := startWorker(w)
	defer stop()
	waitFor(t, "run rescheduled", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Attempts == 1
	})
	got := findW(t, fx, r.ID)
	if got.FailedAt != nil || got.CompletedAt != nil {
		t.Fatal("a rescheduled run is not terminal")
	}
	if got.LockedBy != nil {
		t.Fatal("reschedule must clear the lock")
	}
	if !got.RunAt.After(time.Now()) {
		t.Fatal("reschedule must push run_at into the future (backoff)")
	}
	if c := reporter.Count(); c != 0 {
		t.Fatalf("a rescheduled (retryable) run must NOT report, got %d", c)
	}
}

// ─── Cancel ───────────────────────────────────────────────────────────────────

func TestRunWorker_CancelAtClaim_MarksCancelled(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_cancel_claim.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		<-ctx.Done() // ctx-aware: stop when cancelled
		return ctx.Err()
	}
	w, reporter := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)
	if err := fx.Runs.RequestCancel(context.Background(), r.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}

	stop := startWorker(w)
	defer stop()
	waitFor(t, "run cancelled", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CancelledAt != nil
	})
	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil {
		t.Fatal("a cancelled run must not also be completed/failed")
	}
	if c := reporter.Count(); c != 0 {
		t.Fatalf("a cancelled run must NOT report, got %d", c)
	}
}

func TestRunWorker_CancelDuringRun_MarksCancelled(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_cancel_run.db")
	started := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started // handler is running
	if err := fx.Runs.RequestCancel(context.Background(), r.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	waitFor(t, "run cancelled mid-flight", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CancelledAt != nil
	})
}

// ─── Poison / reclaim ─────────────────────────────────────────────────────────

func TestRunWorker_PoisonReclaim_FailsWithoutRunningHandler(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_poison.db")
	var ran atomic.Bool
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		ran.Store(true)
		return nil
	}
	cfg := fastCfg() // MaxReclaims = 2
	w, reporter := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		cfg,
	)
	r := enqueueRunW(t, fx, "agent", 3)
	// Simulate a poison run: it has already been reclaimed past the cap.
	if _, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET reclaims = ? WHERE id = ?`, cfg.MaxReclaims+1, r.ID); err != nil {
		t.Fatalf("set reclaims: %v", err)
	}

	stop := startWorker(w)
	defer stop()
	waitFor(t, "poison run failed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
	if ran.Load() {
		t.Fatal("a poison run past MaxReclaims must NOT run the handler")
	}
	if c := reporter.Count(); c != 1 {
		t.Fatalf("the poison guard must report exactly once, got %d", c)
	}
}

// ─── Lease loss ───────────────────────────────────────────────────────────────

func TestRunWorker_LeaseLost_Abandons(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_leaselost.db")
	started := make(chan struct{})
	returned := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-ctx.Done() // blocks until the heartbeat detects the lost lease and cancels us
		close(returned)
		return ctx.Err()
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started
	// A different worker steals the run: expire the lease, then claim it.
	forceExpireW(t, fx, r.ID)
	stealer := "stealer-" + uuid.NewString()
	stolen, err := fx.Runs.ClaimDue(context.Background(), stealer, time.Minute)
	if err != nil || stolen == nil || stolen.ID != r.ID {
		t.Fatalf("steal claim: %v / %v", stolen, err)
	}
	<-returned // the original worker's heartbeat detected loss and cancelled the handler

	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil {
		t.Fatal("a worker that lost its lease must NOT finalize the run")
	}
	if got.LockedBy == nil || *got.LockedBy != stealer {
		t.Fatalf("the stealer must still own the run, got %v", got.LockedBy)
	}
}

// ─── Backpressure ─────────────────────────────────────────────────────────────

func TestRunWorker_Backpressure_CapsInFlight(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_backpressure.db")
	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		return nil
	}
	cfg := fastCfg()
	cfg.MaxInFlight = 2
	w, _ := newRunWorker(t, fx, map[string]runapp.Registration{"agent": {Handler: handler}}, cfg)
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = enqueueRunW(t, fx, "agent", 0).ID
	}

	stop := startWorker(w)
	defer stop()
	waitFor(t, "two runs in flight", func() bool { return inFlight.Load() >= 2 })
	time.Sleep(150 * time.Millisecond) // give a buggy worker a chance to exceed
	if p := peak.Load(); p > 2 {
		t.Fatalf("in-flight peaked at %d, must be <= MaxInFlight (2)", p)
	}
	close(release)
	waitFor(t, "all runs completed", func() bool {
		for _, id := range ids {
			if g := findW(t, fx, id); g == nil || g.CompletedAt == nil {
				return false
			}
		}
		return true
	})
}

// ─── Graceful shutdown ────────────────────────────────────────────────────────

func TestRunWorker_Shutdown_AbandonsInFlightForReclaim(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_shutdown.db")
	started := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-ctx.Done() // unblocks on shutdown
		return ctx.Err()
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	<-started
	cancel() // SIGTERM-like shutdown
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not drain within bound")
	}
	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil || got.CancelledAt != nil {
		t.Fatal("a run interrupted by shutdown must be abandoned (left for reclaim), not finalized")
	}
}

// ─── Panic ────────────────────────────────────────────────────────────────────

func TestRunWorker_HandlerPanic_DoesNotCrashPool_FailsRun(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_panic.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		panic("kaboom")
	}
	w, reporter := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 0) // no retries → panic is terminal

	stop := startWorker(w)
	defer stop()
	waitFor(t, "panicking run failed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
	if c := reporter.Count(); c != 1 {
		t.Fatalf("a handler panic (terminal) must be reported exactly once, got %d", c)
	}
	// The pool survived the panic (else waitFor above would have timed out).
}

// ─── Unknown kind ─────────────────────────────────────────────────────────────

func TestRunWorker_UnknownKind_ParksThenFails(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_unknown.db")
	// Registry has "known" but the enqueued run is "ghost".
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"known": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "ghost", 0) // no retries → unknown kind fails terminally

	stop := startWorker(w)
	defer stop()
	waitFor(t, "unknown-kind run failed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
}
