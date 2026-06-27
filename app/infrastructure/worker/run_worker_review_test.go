package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/internal/testfx"
)

// flakyRepo decorates a real run.Repository so a test can inject specific
// failures (a lease lost exactly at finalize, repeated heartbeat errors, a
// checkpoint that reports ownership lost) that timing alone can't reliably force.
type flakyRepo struct {
	run.Repository
	failRenewAfter atomic.Int32 // RenewLease calls past this index return an error (0 = never)
	failRenewFrom  atomic.Int32 // RenewLease calls at/after this index error (1 = even the initial re-lease)
	renewOkFalse   atomic.Bool  // RenewLease returns (false, false, nil) — lease lost, no error
	panicRenewFrom atomic.Int32 // RenewLease calls at/after this index panic (heartbeat path; >1 skips the initial re-lease)
	failCheckpoint atomic.Bool  // Checkpoint returns (false, nil)
	failComplete   atomic.Bool  // MarkComplete returns (false, nil)
	panicComplete  atomic.Bool  // MarkComplete panics (process()-level, non-handler path)
	renewCalls     atomic.Int32
	completeCalls  atomic.Int32
}

var _ run.Repository = (*flakyRepo)(nil)

func (f *flakyRepo) RenewLease(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
) (bool, bool, error) {
	n := f.renewCalls.Add(1)
	// A panic here (heartbeat or initial re-lease, per panicRenewFrom) exercises the
	// heartbeat-goroutine recover; >1 skips the initial re-lease so only the heartbeat
	// panics. RenewLease now carries the cancel flag, so it is the heartbeat's repo call.
	if p := f.panicRenewFrom.Load(); p > 0 && n >= p {
		panic("injected RenewLease panic")
	}
	if f.renewOkFalse.Load() {
		return false, false, nil
	}
	if from := f.failRenewFrom.Load(); from > 0 && n >= from {
		return false, false, errors.New("injected renew error")
	}
	if after := f.failRenewAfter.Load(); after > 0 && n > after {
		return false, false, errors.New("injected renew error")
	}
	return f.Repository.RenewLease(ctx, id, owner, lease)
}

func (f *flakyRepo) Checkpoint(
	ctx context.Context,
	id, owner string,
	state []byte,
	lease time.Duration,
) (bool, error) {
	if f.failCheckpoint.Load() {
		return false, nil
	}
	return f.Repository.Checkpoint(ctx, id, owner, state, lease)
}

// MarkComplete runs ONLY from finalize on the process() linear path (never the
// handler), so a panic here exercises the process()-level recover.
func (f *flakyRepo) MarkComplete(ctx context.Context, id, owner string) (bool, error) {
	f.completeCalls.Add(1)
	if f.panicComplete.Load() {
		panic("injected MarkComplete panic")
	}
	if f.failComplete.Load() {
		return false, nil
	}
	return f.Repository.MarkComplete(ctx, id, owner)
}

func newRunWorkerWithRepo(
	t *testing.T,
	repo run.Repository,
	regs map[string]runapp.Registration,
	cfg RunWorkerConfig,
) (*RunWorker, *countingReporter) {
	t.Helper()
	reg, err := runapp.NewHandlerRegistry(regs, time.Second)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	reporter := &countingReporter{}
	return NewRunWorker(silentLogger(), reporter, repo, reg, nil, nil, cfg), reporter
}

// #1 — the headline invariant for the handler shape that can violate it: a
// cancelled handler that returns nil must be MarkCancelled, never MarkComplete.
func TestRunWorker_CancelNilReturn_MarksCancelledNotCompleted(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_cancel_nil.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		<-ctx.Done()
		return nil // clean stop — NOT ctx.Err()
	}
	w, _ := newRunWorker(
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
	waitFor(t, "cancelled", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CancelledAt != nil
	})
	if findW(t, fx, r.ID).CompletedAt != nil {
		t.Fatal("a cancelled handler that returns nil must NOT be completed")
	}
}

// #1b — a cancel requested AFTER the run is already executing must be observed by
// the heartbeat and end the run cancelled. The sibling above requests the cancel
// BEFORE start (caught at claim time); this exercises the mid-run cancel-read path —
// the cancel flag folded into the heartbeat's RenewLease.
func TestRunWorker_MidRunCancel_Observed(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_midcancel.db")
	started := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-ctx.Done() // unblocks when the heartbeat observes the cancel and cancels us
		return nil
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
	<-started // run is executing; cancel was NOT requested at claim time
	if err := fx.Runs.RequestCancel(context.Background(), r.ID); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	waitFor(t, "cancelled", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CancelledAt != nil
	})
	if findW(t, fx, r.ID).CompletedAt != nil {
		t.Fatal("a mid-run cancel must end cancelled, not completed")
	}
}

// #2 — unknown kind with retries available is PARKED (rescheduled), not killed.
func TestRunWorker_UnknownKind_WithRetries_Parks(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_unknown_park.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	w, _ := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"known": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "ghost", 2) // unknown kind → park, don't fail

	stop := startWorker(w)
	defer stop()
	waitFor(t, "parked (parks bumped)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Parks == 1
	})
	got := findW(t, fx, r.ID)
	if got.FailedAt != nil || got.CompletedAt != nil || got.CancelledAt != nil {
		t.Fatal("an unknown kind within the park budget must be parked, not terminal")
	}
	if got.Attempts != 0 {
		t.Fatal("parking must NOT bump attempts (the logic-retry counter)")
	}
	if got.LockedBy != nil {
		t.Fatal("park must clear the lock")
	}
	if !got.RunAt.After(time.Now()) {
		t.Fatal("park must push run_at into the future")
	}
}

// #5 — retry then exhaustion: a run that keeps failing reschedules until the
// budget is spent, then fails terminally (reported exactly once).
func TestRunWorker_RetryThenExhaust_Fails(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_exhaust.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return errors.New("always")
	}
	w, reporter := newRunWorker(
		t,
		fx,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 1) // one retry

	stop := startWorker(w)
	defer stop()
	waitFor(t, "rescheduled once", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Attempts == 1 && g.FailedAt == nil
	})
	// Clear the backoff so the retry is due immediately.
	if _, err := fx.DB.DB().ExecContext(context.Background(),
		`UPDATE runs SET run_at = strftime('%Y-%m-%d %H:%M:%f','now','-1 second') WHERE id = ?`, r.ID); err != nil {
		t.Fatalf("clear backoff: %v", err)
	}
	waitFor(t, "exhausted → failed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.FailedAt != nil
	})
	if c := reporter.Count(); c != 1 {
		t.Fatalf("terminal failure must report exactly once, got %d", c)
	}
}

// #6 — a lease lost exactly at finalize (MarkComplete returns false) must abandon
// the run (no terminal write, no retry loop), not crash or double-finalize.
func TestRunWorker_LeaseLostAtComplete_Abandons(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_lostfinalize.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.failComplete.Store(true)
	done := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(done)
		return nil // succeeds → worker will try MarkComplete (intercepted → false)
	}
	w, _ := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-done
	waitFor(t, "MarkComplete attempted", func() bool { return repo.completeCalls.Load() >= 1 })
	time.Sleep(80 * time.Millisecond) // let finalize settle
	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil || got.CancelledAt != nil {
		t.Fatal("a lease lost at finalize must abandon the run, not finalize it")
	}
	if c := repo.completeCalls.Load(); c != 1 {
		t.Fatalf("MarkComplete must be attempted exactly once (no retry loop), got %d", c)
	}
}

// #7 — the drain is bounded: an uncooperative handler that ignores ctx cannot
// block Run() past DrainTimeout.
func TestRunWorker_UncooperativeHandler_DrainBounded(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_drain.db")
	block := make(chan struct{})
	started := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-block // ignores ctx entirely
		return nil
	}
	cfg := fastCfg()
	cfg.DrainTimeout = 200 * time.Millisecond
	w, _ := newRunWorker(t, fx, map[string]runapp.Registration{"agent": {Handler: handler}}, cfg)
	enqueueRunW(t, fx, "agent", 0)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() { defer close(runDone); w.Run(ctx) }()
	<-started
	cancel() // shutdown while the handler is stuck
	select {
	case <-runDone:
	case <-time.After(3 * time.Second):
		close(block)
		t.Fatal("drain did not bound an uncooperative handler (Run blocked past DrainTimeout)")
	}
	close(block) // release the leaked handler goroutine
}

// #11 — the heartbeat keeps a run alive past its (short) lease, so another worker
// cannot reclaim a still-running long run.
func TestRunWorker_Heartbeat_KeepsLongRunAlive(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_hb_alive.db")
	started := make(chan struct{})
	release := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-release
		return nil
	}
	cfg := fastCfg()
	cfg.HeartbeatInterval = 50 * time.Millisecond
	// Explicit 300ms kind lease — this is the lease actually renewed (kindLease), so
	// WITHOUT heartbeat renewal it lapses at 300ms and the 700ms-later claim below
	// would succeed. With heartbeat (50ms << 300ms) it stays held. An unset Lease
	// would fall back to the 1s registry default and make this test vacuous (the
	// initial re-lease alone would cover the 700ms window — heartbeat not load-bearing).
	regs := map[string]runapp.Registration{
		"agent": {Handler: handler, Lease: 300 * time.Millisecond},
	}
	w, _ := newRunWorker(t, fx, regs, cfg)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started
	time.Sleep(
		700 * time.Millisecond,
	) // > 2× the 300ms lease; only continuous renewal keeps it held
	// reclaims==0 is the clean, race-free signal that the lease never lapsed (a
	// competing ClaimDue is confounded by the worker's own self-reclaim once the
	// lease expires). With a broken heartbeat the 300ms lease lapses and the run is
	// reclaimed before the 700ms mark, bumping reclaims.
	reclaims := findW(t, fx, r.ID).Reclaims
	close(release)
	if reclaims != 0 {
		t.Fatalf("the heartbeat must keep a running run alive past its lease, but it lapsed and "+
			"was reclaimed (reclaims=%d)", reclaims)
	}
	waitFor(t, "completed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt != nil
	})
}

// #12 — the per-kind lease is applied right after claim (an explicit Registration
// lease, far larger than DefaultLease, shows up on locked_until).
func TestRunWorker_PerKindLease_AppliedAfterClaim(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_perkind.db")
	started := make(chan struct{})
	release := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-release
		return nil
	}
	cfg := fastCfg()
	cfg.DefaultLease = 300 * time.Millisecond
	regs := map[string]runapp.Registration{"agent": {Handler: handler, Lease: 8 * time.Second}}
	w, _ := newRunWorker(t, fx, regs, cfg)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started
	got := findW(t, fx, r.ID)
	if got.LockedUntil == nil || got.LockedUntil.Before(time.Now().Add(5*time.Second)) {
		t.Fatalf(
			"per-kind lease (8s) must be applied after claim: locked_until=%v",
			got.LockedUntil,
		)
	}
	close(release)
}

// #11b — a per-kind lease SHORTER than the global heartbeat interval must still
// be kept alive. The heartbeat cadence must derive from the EFFECTIVE lease
// (kindLease), not from the global cfg.HeartbeatInterval that was sized off
// DefaultLease — otherwise the re-leased short lease expires before the first
// heartbeat ever fires and another worker reclaims a still-running run (double
// execution). Sibling of #11, which only covers the safe direction (kindLease >>
// heartbeat). Fails on the pre-fix code where the ticker uses cfg.HeartbeatInterval.
func TestRunWorker_PerKindLeaseShorterThanHeartbeat_StaysAlive(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_shortlease.db")
	started := make(chan struct{})
	release := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-release
		return nil
	}
	cfg := fastCfg()
	cfg.DefaultLease = 6 * time.Second
	cfg.HeartbeatInterval = 2 * time.Second // deliberately LARGER than the 300ms kind lease
	regs := map[string]runapp.Registration{
		"agent": {Handler: handler, Lease: 300 * time.Millisecond},
	}
	w, _ := newRunWorker(t, fx, regs, cfg)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started
	// Well past the 300ms kind lease, but before the 2s global heartbeat would tick
	// on the buggy code. On the buggy code the lease lapsed ~600ms ago, so the worker
	// self-reclaims its own still-running run → reclaims bumps. A correctly-derived
	// heartbeat (lease/3 = 100ms) renews in time, so the lease never lapses and
	// reclaims stays 0. Reclaims is the clean, race-free signal (a competing ClaimDue
	// is confounded by the worker's own self-reclaim).
	time.Sleep(900 * time.Millisecond)
	reclaims := findW(t, fx, r.ID).Reclaims
	close(release) // unblock the handler regardless of outcome
	if reclaims != 0 {
		t.Fatalf("a per-kind lease shorter than the heartbeat interval lapsed and was reclaimed "+
			"(reclaims=%d); the heartbeat must derive its cadence from the effective lease, not the "+
			"global interval", reclaims)
	}
	waitFor(t, "completed", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.CompletedAt != nil
	})
}

// #13 — the heartbeat abandons after maxHeartbeatErrors consecutive RenewLease
// errors (the initial re-lease is allowed to succeed).
func TestRunWorker_HeartbeatErrors_Abandon(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_hb_errors.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.failRenewAfter.Store(1) // call 1 (the initial re-lease) succeeds; heartbeat renews fail
	started := make(chan struct{})
	returned := make(chan struct{})
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		close(started)
		<-ctx.Done() // unblocks when the heartbeat gives up and cancels us
		close(returned)
		return nil
	}
	w, _ := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started
	<-returned // heartbeat hit maxHeartbeatErrors → abandon → cancelled the handler
	time.Sleep(60 * time.Millisecond)
	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil || got.CancelledAt != nil {
		t.Fatal("a run abandoned on heartbeat errors must not be finalized")
	}
}

// #14 — Checkpointer.Save surfaces ErrLeaseLost to the handler when ownership was
// lost (Checkpoint returns false).
func TestRunWorker_Checkpoint_SurfacesErrLeaseLost(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_errleaselost.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.failCheckpoint.Store(true)
	saveErr := make(chan error, 1)
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		saveErr <- ck.Save(ctx, []byte(`{"x":1}`))
		return nil
	}
	w, _ := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	err := <-saveErr
	if !errors.Is(err, runapp.ErrLeaseLost) {
		t.Fatalf("ck.Save must return ErrLeaseLost when ownership is lost, got %v", err)
	}
}

// #15 — the backpressure slot is released after a failure, so a single-slot pool
// keeps draining a queue of failing runs.
func TestRunWorker_SlotReleasedAfterFailure(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_slot.db")
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		return errors.New("fail")
	}
	cfg := fastCfg()
	cfg.MaxInFlight = 1
	w, _ := newRunWorker(t, fx, map[string]runapp.Registration{"agent": {Handler: handler}}, cfg)
	ids := make([]string, 3)
	for i := range ids {
		ids[i] = enqueueRunW(t, fx, "agent", 0).ID // each fails terminally
	}

	stop := startWorker(w)
	defer stop()
	waitFor(t, "all runs failed (slot released each time)", func() bool {
		for _, id := range ids {
			if g := findW(t, fx, id); g == nil || g.FailedAt == nil {
				return false
			}
		}
		return true
	})
}

// #16 — the initial re-lease to kindLease fails with an ERROR right after claim
// (run_worker.go:217): the worker must abandon WITHOUT running the handler and
// WITHOUT finalizing (the run is left for lease-lapse reclaim). This is the
// claim-gap guard that exists to stop double execution under a short DefaultLease.
func TestRunWorker_InitialReleaseError_AbandonsWithoutRunningHandler(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_initrelease_err.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.failRenewFrom.Store(1) // call #1 (the initial re-lease) errors
	var handlerRan atomic.Bool
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		handlerRan.Store(true)
		return nil
	}
	w, _ := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "initial re-lease attempted", func() bool { return repo.renewCalls.Load() >= 1 })
	time.Sleep(60 * time.Millisecond) // settle: a broken guard would run the handler by now
	if handlerRan.Load() {
		t.Fatal("handler must NOT run when the initial re-lease errors — the worker must abandon")
	}
	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil || got.CancelledAt != nil {
		t.Fatal(
			"an initial-re-lease error must abandon (leave non-finalized for lease-lapse reclaim)",
		)
	}
}

// #17 — the initial re-lease returns ok=false (another worker reclaimed the row
// in the claim gap; run_worker.go:220): same contract — abandon without running
// the handler or finalizing.
func TestRunWorker_InitialReleaseLost_AbandonsWithoutRunningHandler(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_initrelease_lost.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.renewOkFalse.Store(true) // the initial re-lease reports the lease already lost
	var handlerRan atomic.Bool
	handler := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		handlerRan.Store(true)
		return nil
	}
	w, _ := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{"agent": {Handler: handler}},
		fastCfg(),
	)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "initial re-lease attempted", func() bool { return repo.renewCalls.Load() >= 1 })
	time.Sleep(60 * time.Millisecond)
	if handlerRan.Load() {
		t.Fatal("handler must NOT run when the initial re-lease reports the lease lost")
	}
	got := findW(t, fx, r.ID)
	if got.CompletedAt != nil || got.FailedAt != nil || got.CancelledAt != nil {
		t.Fatal("a lost initial re-lease must abandon (leave non-finalized for reclaim)")
	}
}

// #18 — a panic inside the HEARTBEAT goroutine (a repo call panicking, here
// RenewLease — which now also carries the cancel flag) must be recovered there and
// must NOT crash the pool: a second healthy run on a single-slot pool still
// completes, proving the panic recovered AND the backpressure slot was released.
func TestRunWorker_HeartbeatPanic_DoesNotCrashPool(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_hbpanic.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.panicRenewFrom.Store(2) // heartbeat renew panics; 2 skips the initial re-lease
	blocking := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error {
		<-ctx.Done() // unblocks when the heartbeat recover cancels the handler
		return ctx.Err()
	}
	healthy := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	cfg := fastCfg()
	cfg.MaxInFlight = 1 // single slot → the healthy run can only run if the slot was released
	w, reporter := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{
			"panicky": {Handler: blocking},
			"healthy": {Handler: healthy},
		},
		cfg,
	)
	enqueueRunW(t, fx, "panicky", 3)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "heartbeat panic captured", func() bool { return reporter.Count() >= 1 })
	repo.panicRenewFrom.Store(0) // stop panicking so the healthy run can heartbeat normally
	healthyRun := enqueueRunW(t, fx, "healthy", 0)
	waitFor(t, "healthy run completes after the heartbeat panic", func() bool {
		g := findW(t, fx, healthyRun.ID)
		return g != nil && g.CompletedAt != nil
	})
}

// #19 — a panic OUTSIDE the handler (here MarkComplete in finalize) must be
// recovered at the process() top (run_worker.go:181) and must NOT crash the pool:
// a second healthy run on a single-slot pool still completes (recover + slot release).
func TestRunWorker_FinalizePanic_DoesNotCrashPool(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/rwr_finalizepanic.db")
	repo := &flakyRepo{Repository: fx.Runs}
	repo.panicComplete.Store(true) // MarkComplete (finalize) panics
	succeeds := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	healthy := func(ctx context.Context, r *run.Run, ck runapp.Checkpointer) error { return nil }
	cfg := fastCfg()
	cfg.MaxInFlight = 1
	w, reporter := newRunWorkerWithRepo(
		t,
		repo,
		map[string]runapp.Registration{
			"panicky": {Handler: succeeds},
			"healthy": {Handler: healthy},
		},
		cfg,
	)
	enqueueRunW(t, fx, "panicky", 3)

	stop := startWorker(w)
	defer stop()
	waitFor(t, "finalize panic captured", func() bool { return reporter.Count() >= 1 })
	repo.panicComplete.Store(false) // stop panicking so the healthy run can complete
	healthyRun := enqueueRunW(t, fx, "healthy", 0)
	waitFor(t, "healthy run completes after the finalize panic", func() bool {
		g := findW(t, fx, healthyRun.ID)
		return g != nil && g.CompletedAt != nil
	})
}
