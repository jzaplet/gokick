package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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
	return NewRunWorker(silentLogger(), reporter, fx.Runs, reg, nil, nil, cfg), reporter
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

// stealLeaseW atomically reassigns a run's lease to newOwner with a fresh future
// expiry — simulating another worker reclaiming it. Unlike expire-then-ClaimDue it
// is a SINGLE write, so it cannot be interleaved by a live incumbent's heartbeat
// (RenewLease), which makes the steal deterministic even under load. The incumbent's
// next owner-checked write then matches zero rows (locked_by no longer matches).
func stealLeaseW(t *testing.T, fx *testfx.Fixture, id, newOwner string) {
	t.Helper()
	if _, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET locked_by = ?, locked_until = strftime('%Y-%m-%d %H:%M:%f','now','+1 hour') WHERE id = ?`,
		newOwner, id); err != nil {
		t.Fatalf("steal lease: %v", err)
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
	// A different worker steals the run by atomically reassigning ownership. We flip
	// locked_by in one UPDATE rather than expire-then-ClaimDue because the incumbent
	// is still alive and heartbeating: RenewLease is owner-only and ignores
	// locked_until by design (so an owner can rescue a not-yet-stolen lease), so an
	// expire+claim races that renew on SQLite's single write lock — and under load the
	// incumbent's heartbeat wins that race every time. A single ownership flip cannot
	// be interleaved, so the incumbent's next RenewLease matches zero rows (owner
	// mismatch), detects the loss, and abandons the run.
	stealer := "stealer-" + uuid.NewString()
	stealLeaseW(t, fx, r.ID, stealer)
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

// A run-once (max_retries=0) unknown kind must be PARKED for the rolling-deploy
// registry-skew window, NOT failed on the first skewed claim — otherwise a long run
// with checkpointed state is discarded just because an old binary (no handler)
// claimed it. The park budget (minUnknownKindParks) is independent of max_retries.
func TestRunWorker_UnknownKind_RunOnce_ParksNotFailed(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_unknown_park.db")
	// Registry has "known" but the enqueued run is "ghost".
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"known": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "ghost", 0) // run-once, but unknown → parked, not failed

	stop := startWorker(w)
	defer stop()
	// Parked = rescheduled: parks bumped (NOT attempts), lock cleared, never failed_at.
	waitFor(t, "unknown-kind run parked", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Parks >= 1 && g.FailedAt == nil
	})
	if g := findW(t, fx, r.ID); g.FailedAt != nil || g.Attempts != 0 {
		t.Fatal(
			"a run-once unknown kind must be parked via the parks counter, not failed and not on attempts",
		)
	}
}

// Once the deploy-window park budget is spent, a genuinely-removed kind fails
// terminally. Pre-setting attempts to the budget exercises the terminal path
// without waiting out minUnknownKindParks exponential backoffs.
func TestRunWorker_UnknownKind_ExhaustedBudget_Fails(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_unknown_fail.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"known": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "ghost", 0)
	if _, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET parks = ? WHERE id = ?`, minUnknownKindParks, r.ID); err != nil {
		t.Fatalf("pre-bump parks: %v", err)
	}

	stop := startWorker(w)
	defer stop()
	waitFor(t, "unknown-kind run failed after budget", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
}

// Regression for the deploy-skew/retry-budget coupling the re-review caught: parking
// an unknown kind must NOT consume the handler's logic-retry budget. A run whose
// parks counter is already maxed (as if parked through a long rolling deploy) must,
// once a handler-bearing binary runs it, still get its full max_retries — its first
// failure reschedules, it is not terminal-failed.
func TestRunWorker_ParkingDoesNotConsumeRetryBudget(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_park_budget.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return errors.New("transient")
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 2) // 2 logic retries
	if _, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET parks = ? WHERE id = ?`, minUnknownKindParks, r.ID); err != nil {
		t.Fatalf("pre-set parks: %v", err)
	}

	stop := startWorker(w)
	defer stop()
	// Despite parks being maxed, the handler's first failure RESCHEDULES (attempts→1)
	// rather than terminal-failing — parks never touched the logic-retry budget.
	waitFor(t, "first failure rescheduled (retry budget intact)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Attempts == 1
	})
	if g := findW(t, fx, r.ID); g.FailedAt != nil {
		t.Fatal(
			"parking must not consume the logic-retry budget: the run must reschedule, not fail",
		)
	}
}

// A run handler runs OUTSIDE a tx and offloads transactional side-work by enqueuing
// a fire-and-forget run or a child run. The worker bypasses the bus, so it must inject the
// dispatchers into the handler ctx — without that shared.RunDispatcherFromContext
// returns the silent no-op and the enqueue vanishes. Here a parent handler enqueues a
// child run and we prove the child row actually lands and runs.
func TestRunWorker_HandlerCanEnqueueChildRun(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_enqueue_child.db")
	childRan := make(chan struct{}, 1)
	parent := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return shared.RunDispatcherFromContext(ctx).
			Enqueue(ctx, "child", 0, map[string]string{"k": "v"})
	}
	child := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		childRan <- struct{}{}
		return nil
	}
	reg, err := runapp.NewHandlerRegistry(map[string]runapp.Registration{
		"parent": {Handler: parent},
		"child":  {Handler: child},
	}, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	dispatcher := runapp.NewDispatcher(fx.Runs, reg)
	w := NewRunWorker(silentLogger(), &countingReporter{}, fx.Runs, reg, dispatcher, nil, fastCfg())

	enqueueRunW(t, fx, "parent", 0)
	stop := startWorker(w)
	defer stop()
	select {
	case <-childRan:
	case <-time.After(3 * time.Second):
		t.Fatal("child run enqueued by the parent handler never ran — dispatcher not injected")
	}
}

// The run worker bypasses the bus, so it must restore the tenant the run was
// enqueued for into the handler's context from the claimed row. Uses a non-default
// tenant so it proves propagation rather than a tautology. A handler that
// saw the wrong tenant would query another tenant's data (the multitenant-agent
// correctness this whole engine exists for).
func TestRunWorker_RestoresRunTenantIntoHandlerContext(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_tenant.db")
	seenCh := make(chan string, 1) // channel receive → happens-before the read (race-safe)
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		seenCh <- shared.TenantIDFromContext(ctx)
		return nil
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)

	r := run.NewRun("agent", []byte(`{}`), 0)
	r.TenantID = "tenant-A"
	if err := fx.Runs.Enqueue(context.Background(), r); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	stop := startWorker(w)
	defer stop()
	if seen := <-seenCh; seen != "tenant-A" {
		t.Fatalf(
			"handler ctx tenant = %q, want %q — the run worker must restore the run's tenant",
			seen,
			"tenant-A",
		)
	}
}

// A durable run handler must NOT open a transaction: the worker marks the handler
// ctx no-tx (shared.ContextForbidTx), so BeginTx fails closed — a long handler in a
// transaction would hold the global SQLite write lock for the run's lifetime and
// freeze every other write. End-to-end proof the marker reaches the handler ctx.
func TestRunWorker_HandlerCannotOpenTransaction(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_notx.db")
	beginErr := make(chan error, 1)
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		_, err := fx.DB.BeginTx(ctx) // fx.DB is the SqliteManager (shared.Transactor)
		beginErr <- err
		return nil
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	enqueueRunW(t, fx, "agent", 0)

	stop := startWorker(w)
	defer stop()
	if err := <-beginErr; err == nil || !strings.Contains(err.Error(), "no-transaction zone") {
		t.Fatalf("a run handler must be forbidden from opening a transaction, got %v", err)
	}
}

// The headline durable-execution promise, end-to-end THROUGH the worker pool: a run
// that a prior worker claimed, checkpointed, then crashed on (lease lapsed) is
// RECLAIMED by the pool (Reclaims 0->1), re-leased, and the handler RE-RUNS seeing
// the carried checkpoint state, then completes. The two halves (repo carries state;
// worker resumes from pre-seeded state) were each tested in isolation; this splices
// them through a real reclaim — the claim -> poison-check -> re-lease -> resume ->
// complete path on a run with Reclaims>=1, which no other test drives to completion.
func TestRunWorker_ResumesAfterReclaim_Completes(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rw_resume_reclaim.db")
	ctx := context.Background()

	// Simulate a prior worker: claim (reclaims stays 0, lease was NULL), checkpoint
	// {"step":1}, then crash — modelled by expiring the lease via real methods so the
	// seed cannot drift from production behavior.
	r := enqueueRunW(t, fx, "agent", 3)
	if _, err := fx.Runs.ClaimDue(ctx, "dead-worker", time.Minute); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	if ok, err := fx.Runs.Checkpoint(ctx, r.ID, "dead-worker", []byte(`{"step":1}`), time.Minute); err != nil ||
		!ok {
		t.Fatalf("seed checkpoint: ok=%v err=%v", ok, err)
	}
	forceExpireW(t, fx, r.ID) // the dead worker's lease lapses -> claimable again

	resumed := make(chan string, 1)
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		resumed <- string(r.State)
		return nil
	}
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)

	stop := startWorker(w)
	defer stop()
	if got := <-resumed; got != `{"step":1}` {
		t.Fatalf("resumed handler saw state %q, want the carried checkpoint %q", got, `{"step":1}`)
	}
	waitFor(t, "reclaimed run completes", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt != nil
	})
	if got := findW(t, fx, r.ID); got.Reclaims != 1 {
		t.Fatalf("a run reclaimed once must carry Reclaims=1, got %d", got.Reclaims)
	}
}

// withDefaults turns a zero/misconfigured config into safe runtime values. Pure and
// deterministic, but every clamp/default branch was unexercised (tests build cfg by
// hand with all-valid fields). Covers the six defaults + the misconfigured-heartbeat
// clamp (HeartbeatInterval > Lease/2 -> Lease/3) that guards against false expiry.
func TestRunWorkerConfig_WithDefaults(t *testing.T) {
	t.Parallel()

	// All-zero → every documented default applies; HeartbeatInterval derives Lease/3.
	d := RunWorkerConfig{}.withDefaults()
	if d.DefaultLease != 5*time.Minute {
		t.Fatalf("DefaultLease default: got %s want 5m", d.DefaultLease)
	}
	if d.HeartbeatInterval != d.DefaultLease/3 {
		t.Fatalf("HeartbeatInterval default: got %s want Lease/3", d.HeartbeatInterval)
	}
	if d.PollInterval != time.Second {
		t.Fatalf("PollInterval default: got %s want 1s", d.PollInterval)
	}
	if d.DrainTimeout != 10*time.Second {
		t.Fatalf("DrainTimeout default: got %s want 10s", d.DrainTimeout)
	}
	if d.MaxInFlight != 1 {
		t.Fatalf("MaxInFlight default: got %d want 1", d.MaxInFlight)
	}
	if d.MaxReclaims != 20 {
		t.Fatalf("MaxReclaims default: got %d want 20", d.MaxReclaims)
	}

	// A heartbeat NOT comfortably shorter than the lease (> Lease/2) is clamped to
	// Lease/3 — the misconfigured-heartbeat guard against false lease expiry.
	clamped := RunWorkerConfig{
		DefaultLease:      6 * time.Second,
		HeartbeatInterval: 5 * time.Second,
	}.withDefaults()
	if clamped.HeartbeatInterval != 2*time.Second {
		t.Fatalf(
			"oversized HeartbeatInterval must clamp to Lease/3 (2s), got %s",
			clamped.HeartbeatInterval,
		)
	}

	// A valid explicit heartbeat (<= Lease/2) is preserved untouched.
	kept := RunWorkerConfig{
		DefaultLease:      6 * time.Second,
		HeartbeatInterval: time.Second,
	}.withDefaults()
	if kept.HeartbeatInterval != time.Second {
		t.Fatalf("a valid HeartbeatInterval must be preserved, got %s", kept.HeartbeatInterval)
	}
}
