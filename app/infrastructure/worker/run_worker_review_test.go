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

	"github.com/google/uuid"
)

// flakyRepo decorates a real run.Repository so a test can inject specific
// failures (a lease lost exactly at finalize, repeated heartbeat errors, a
// checkpoint that reports ownership lost) that timing alone can't reliably force.
type flakyRepo struct {
	run.Repository
	failRenewAfter atomic.Int32 // RenewLease calls past this index return an error (0 = never)
	failCheckpoint atomic.Bool  // Checkpoint returns (false, nil)
	failComplete   atomic.Bool  // MarkComplete returns (false, nil)
	renewCalls     atomic.Int32
	completeCalls  atomic.Int32
}

var _ run.Repository = (*flakyRepo)(nil)

func (f *flakyRepo) RenewLease(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
) (bool, error) {
	n := f.renewCalls.Add(1)
	if after := f.failRenewAfter.Load(); after > 0 && n > after {
		return false, errors.New("injected renew error")
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

func (f *flakyRepo) MarkComplete(ctx context.Context, id, owner string) (bool, error) {
	f.completeCalls.Add(1)
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
	return NewRunWorker(silentLogger(), reporter, repo, reg, cfg), reporter
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
// the heartbeat's cancel-read and end the run cancelled. The sibling above requests
// the cancel BEFORE start (caught at claim time, run_worker.go:262); this exercises
// the mid-run cancel-read path (the IsCancelRequested call inside heartbeat()).
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
	r := enqueueRunW(t, fx, "ghost", 2) // retries available → park, don't fail

	stop := startWorker(w)
	defer stop()
	waitFor(t, "parked (attempts bumped)", func() bool {
		g := findW(t, fx, r.ID)
		return g != nil && g.Attempts == 1
	})
	got := findW(t, fx, r.ID)
	if got.FailedAt != nil || got.CompletedAt != nil || got.CancelledAt != nil {
		t.Fatal("an unknown kind with retries must be parked, not terminal")
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
	cfg.DefaultLease = 300 * time.Millisecond
	cfg.HeartbeatInterval = 50 * time.Millisecond
	w, _ := newRunWorker(t, fx, map[string]runapp.Registration{"agent": {Handler: handler}}, cfg)
	r := enqueueRunW(t, fx, "agent", 3)

	stop := startWorker(w)
	defer stop()
	<-started
	time.Sleep(700 * time.Millisecond) // > 2× the lease; the heartbeat must keep renewing
	other, err := fx.Runs.ClaimDue(context.Background(), "other-"+uuid.NewString(), time.Minute)
	if err != nil {
		t.Fatalf("concurrent claim: %v", err)
	}
	if other != nil {
		t.Fatal("the heartbeat must keep a running run un-reclaimable past its lease")
	}
	close(release)
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
