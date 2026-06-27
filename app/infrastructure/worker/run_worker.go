package worker

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

// Run-worker-local log keys (logKeyKinds/logKeyPanic/logKeyStack are shared with
// the job worker in this package).
const (
	logKeyRunID       = "run_id"
	logKeyRunKind     = "run_kind"
	logKeyOwner       = "owner"
	logKeyReclaims    = "reclaims"
	logKeyMaxInflight = "max_in_flight"
)

// maxHeartbeatErrors is how many consecutive RenewLease errors the heartbeat
// tolerates before treating the lease as untrustworthy and abandoning the run.
const maxHeartbeatErrors = 3

// minUnknownKindParks bounds how many times a run whose kind this binary has no
// handler for is parked (via Park, bumping the dedicated `parks` counter) before
// failing terminally — a rolling-deploy registry-skew budget gated INDEPENDENTLY of
// the handler's max_retries (parking never touches `attempts`). Without it a
// run-once run (max_retries=0) would fail on the first skewed claim, discarding a
// long run that a binary WITH the handler could still resume.
const minUnknownKindParks = 5

// cancelGraceLeaseMultiple bounds, as a multiple of the lease, how long the
// heartbeat keeps renewing for a cancelled run whose handler has not yet returned.
// A ctx-aware handler returns promptly on cancel and never reaches this; the bound
// only fires for a handler that ignores ctx cancellation (a contract violation) —
// past it the worker abandons, the lease lapses, and reclaim + the poison-reclaim
// cap terminate the run instead of renewing it forever.
const cancelGraceLeaseMultiple = 4

// RunWorkerConfig tunes the durable run worker. Zero fields take safe defaults.
type RunWorkerConfig struct {
	DefaultLease      time.Duration // initial claim lease; per-kind leases (registry) override it via an immediate re-lease
	HeartbeatInterval time.Duration // must be << min lease
	PollInterval      time.Duration // claim cadence when idle
	DrainTimeout      time.Duration // bound on the graceful-shutdown drain
	MaxInFlight       int           // backpressure: max runs executing concurrently
	MaxReclaims       int           // poison guard: a run reclaimed more than this is failed without running
}

func (c RunWorkerConfig) withDefaults() RunWorkerConfig {
	if c.DefaultLease <= 0 {
		c.DefaultLease = 5 * time.Minute
	}
	// A heartbeat must stay comfortably shorter than the lease it renews, else it
	// can't keep it alive. Both an unset (<=0) interval and a misconfigured too-large
	// one (> Lease/2) fall back to the same Lease/3; per-kind leases must likewise
	// stay >> this.
	if c.HeartbeatInterval <= 0 || c.HeartbeatInterval > c.DefaultLease/2 {
		c.HeartbeatInterval = c.DefaultLease / 3
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.DrainTimeout <= 0 {
		c.DrainTimeout = 10 * time.Second
	}
	if c.MaxInFlight <= 0 {
		c.MaxInFlight = 1
	}
	if c.MaxReclaims <= 0 {
		c.MaxReclaims = 20
	}
	return c
}

// RunWorker executes durable runs OUTSIDE a transaction (ADR-0001): it claims a
// run with a per-claim owner nonce, runs the registered handler in a goroutine
// while a heartbeat renews the lease, and on worker death the lease lapses so
// another worker reclaims and resumes from the last checkpoint.
type RunWorker struct {
	id       string // per-process id; owner tokens are id + "-" + per-claim uuid
	logger   *slog.Logger
	reporter shared.ErrorReporter
	repo     run.Repository
	registry *runapp.HandlerRegistry
	// Dispatchers a run handler may use for transactional side-work: a run handler
	// runs OUTSIDE a tx (ContextForbidTx), so it persists progress via the
	// Checkpointer and offloads anything that needs a transaction by enqueuing a
	// short job or a child run. The worker bypasses the bus, so it injects these
	// into the handler ctx itself (mirroring the job worker) — otherwise
	// shared.*DispatcherFromContext would return the silent no-op and drop the
	// enqueue. Either may be nil (a worker built without dispatchers); the inject
	// then falls through to the no-op, same as before.
	jobDispatcher shared.JobDispatcher
	runDispatcher shared.RunDispatcher
	cfg           RunWorkerConfig
}

func NewRunWorker(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
	repo run.Repository,
	registry *runapp.HandlerRegistry,
	jobDispatcher shared.JobDispatcher,
	runDispatcher shared.RunDispatcher,
	cfg RunWorkerConfig,
) *RunWorker {
	return &RunWorker{
		id:            uuid.NewString(),
		logger:        logger,
		reporter:      reporter,
		repo:          repo,
		registry:      registry,
		jobDispatcher: jobDispatcher,
		runDispatcher: runDispatcher,
		cfg:           cfg.withDefaults(),
	}
}

// newOwner returns a fresh per-claim owner nonce. Per-claim (not stable per
// worker) is required so the fence works even against this worker's own earlier
// abandoned goroutine.
func (w *RunWorker) newOwner() string { return w.id + "-" + uuid.NewString() }

// Run claims and executes runs until ctx is cancelled, then drains in-flight runs
// (bounded by DrainTimeout) and returns.
func (w *RunWorker) Run(ctx context.Context) {
	w.logger.Info("run worker: starting",
		logKeyMaxInflight, w.cfg.MaxInFlight, logKeyKinds, w.registry.Kinds())

	sem := make(chan struct{}, w.cfg.MaxInFlight)
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	var wg sync.WaitGroup

loop:
	for {
		// Backpressure: take a slot (or shut down).
		select {
		case <-ctx.Done():
			break loop
		case sem <- struct{}{}:
		}

		owner := w.newOwner()
		r, err := w.repo.ClaimDue(ctx, owner, w.cfg.DefaultLease)
		switch {
		case err != nil:
			w.logger.Error("run worker: claim failed", shared.LogKeyError, err)
			<-sem
			if !w.idleWait(ctx, ticker) {
				break loop
			}
		case r == nil:
			<-sem
			if !w.idleWait(ctx, ticker) {
				break loop
			}
		default:
			wg.Add(1)
			go func(r *run.Run, owner string) {
				defer wg.Done()
				defer func() { <-sem }()
				w.process(ctx, r, owner)
			}(r, owner)
		}
	}

	w.drain(&wg)
	w.logger.Info("run worker: stopped")
}

// idleWait blocks for one poll interval, returning false if ctx is cancelled.
func (w *RunWorker) idleWait(ctx context.Context, ticker *time.Ticker) bool {
	select {
	case <-ctx.Done():
		return false
	case <-ticker.C:
		return true
	}
}

// drain waits for in-flight runs to finish, bounded by DrainTimeout. Stragglers
// past the deadline are left running; they are owner-fenced, their leases lapse,
// and another process reclaims + resumes them.
//
// The monitor goroutine below is the canonical WaitGroup-with-timeout: on a
// DrainTimeout it outlives drain() and self-reaps the instant the last straggler's
// process() returns (every process() is bounded — workerCtx is cancelled, so
// ctx-aware handlers return promptly). It holds only the WaitGroup + a channel, so
// the lingering is a brief, bounded tail, not a leak.
func (w *RunWorker) drain(wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(w.cfg.DrainTimeout):
		w.logger.Warn("run worker: drain timeout, abandoning in-flight runs for reclaim")
	}
}

// process runs one claimed run end-to-end. A top-level recover keeps a panic
// anywhere (not just the handler) from crashing the pool; on panic the run is
// abandoned (no finalize) and left for lease-lapse reclaim.
func (w *RunWorker) process(workerCtx context.Context, r *run.Run, owner string) {
	log := w.logger.With(logKeyRunID, r.ID, logKeyRunKind, r.Kind, logKeyOwner, owner)
	defer func() {
		if rec := recover(); rec != nil {
			log.LogAttrs(workerCtx, slog.LevelError, "run worker: process panicked",
				slog.Any(logKeyPanic, rec), slog.String(logKeyStack, string(debug.Stack())))
			w.reporter.Capture(workerCtx,
				&shared.PanicError{Value: rec, Message: fmt.Sprintf("run process panic: %v", rec)},
				slog.String(logKeyRunID, r.ID), slog.String(logKeyRunKind, r.Kind))
		}
	}()

	// Poison-crash-loop bound at the CLAIM boundary: a run that keeps killing the
	// worker process never reaches handleFailure, so the cap must fire here, before
	// the handler runs. ClaimDue already returned the post-increment reclaims.
	if r.Reclaims > w.cfg.MaxReclaims {
		log.Error("run worker: exceeded max reclaims, failing (poison)", logKeyReclaims, r.Reclaims)
		if ok, err := w.repo.MarkFailed(workerCtx, r.ID, owner,
			fmt.Sprintf("exceeded max reclaims (%d > %d) — poison run", r.Reclaims, w.cfg.MaxReclaims)); err != nil {
			log.Error("run worker: poison mark-failed errored", shared.LogKeyError, err)
		} else if ok {
			w.reporter.Capture(workerCtx,
				fmt.Errorf("run %q exceeded max reclaims (poison-crash-loop)", r.Kind),
				slog.String(logKeyRunID, r.ID))
		}
		return
	}

	handler, kindLease, known := w.registry.Lookup(r.Kind)
	if !known {
		w.handleUnknownKind(workerCtx, log, r, owner)
		return
	}

	// Close the claim gap: ClaimDue already stamped DefaultLease, so re-lease to the
	// kind's lease ONLY when it differs — a per-kind lease (shorter or longer) must
	// take effect before the first heartbeat tick, but for the common default-lease
	// kind the claim's lease is already correct and a second write is wasted. The
	// heartbeat keeps its interval sound against kindLease by deriving it from the
	// lease inside heartbeat() (not here). alive=false = already lost.
	if kindLease != w.cfg.DefaultLease {
		if alive, _, err := w.repo.RenewLease(workerCtx, r.ID, owner, kindLease); err != nil {
			log.Error("run worker: initial re-lease errored, abandoning", shared.LogKeyError, err)
			return
		} else if !alive {
			log.Warn("run worker: lost lease immediately after claim, abandoning")
			return
		}
	}

	// Restore the tenant the run was enqueued for (the worker bypasses the bus) and
	// mark the ctx no-transaction: a durable run runs OUTSIDE a tx, so an accidental
	// BeginTx in the handler must fail closed, not freeze the DB (shared.ContextForbidTx).
	runCtx := shared.ContextForbidTx(shared.ContextWithTenantID(workerCtx, r.TenantID))
	// Inject the dispatchers the handler needs for transactional side-work (enqueue a
	// short job or a child run). The worker bypasses the bus, so without this
	// shared.*DispatcherFromContext would return the silent no-op and drop the
	// enqueue. Skip a nil dispatcher so the ctx falls through to the no-op cleanly.
	if w.jobDispatcher != nil {
		runCtx = shared.ContextWithJobDispatcher(runCtx, w.jobDispatcher)
	}
	if w.runDispatcher != nil {
		runCtx = shared.ContextWithRunDispatcher(runCtx, w.runDispatcher)
	}

	handlerCtx, cancelHandler := context.WithCancel(runCtx)
	defer cancelHandler()
	// hbCtx is a SIBLING of handlerCtx under workerCtx: a shutdown (workerCtx)
	// stops the heartbeat, but cancelHandler (lease loss / cancel) does NOT — the
	// heartbeat must keep renewing while a cancelled handler winds down.
	hbCtx, cancelHeartbeat := context.WithCancel(runCtx)
	defer cancelHeartbeat()

	var abandon, cancelled atomic.Bool
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		defer func() {
			// A panic in the heartbeat (e.g. a repo call) must not crash the pool:
			// recover, report, and abandon the run (left for lease-lapse reclaim).
			if rec := recover(); rec != nil {
				w.logger.LogAttrs(hbCtx, slog.LevelError, "run worker: heartbeat panicked",
					slog.Any(logKeyPanic, rec), slog.String(logKeyStack, string(debug.Stack())))
				w.reporter.Capture(
					hbCtx,
					&shared.PanicError{
						Value:   rec,
						Message: fmt.Sprintf("heartbeat panic: %v", rec),
					},
					slog.String(logKeyRunID, r.ID),
				)
				abandon.Store(true)
				cancelHandler()
			}
		}()
		w.heartbeat(hbCtx, r.ID, owner, kindLease, cancelHandler, &abandon, &cancelled)
	}()

	// Honor a cancel already requested at claim time (immutable claimed-row value;
	// no shared state read).
	if r.CancelRequested {
		cancelled.Store(true)
		cancelHandler()
	}

	start := time.Now()
	hErr := w.runHandler(handlerCtx, r, owner, kindLease, handler)

	cancelHeartbeat() // stop the heartbeat
	<-hbDone          // JOIN: establishes happens-before before reading the atomics

	w.finalize(
		workerCtx,
		log,
		r,
		owner,
		hErr,
		abandon.Load(),
		cancelled.Load(),
		time.Since(start),
	)
}

// runHandler invokes the handler with a panic-scoped recover so a handler panic
// becomes an error on the linear path — heartbeat-stop and finalize always run.
func (w *RunWorker) runHandler(
	ctx context.Context,
	r *run.Run,
	owner string,
	lease time.Duration,
	handler runapp.HandlerFunc,
) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			w.logger.LogAttrs(ctx, slog.LevelError, "run worker: handler panicked",
				slog.Any(logKeyPanic, rec), slog.String(logKeyStack, string(debug.Stack())))
			err = &shared.PanicError{Value: rec, Message: fmt.Sprintf("handler panic: %v", rec)}
		}
	}()
	ck := &checkpointer{repo: w.repo, id: r.ID, owner: owner, lease: lease}
	return handler(ctx, r, ck)
}

// heartbeat renews the lease until ctx is cancelled (shutdown or main's stop). On
// a lost or untrustworthy lease it sets abandon + cancels the handler and exits;
// on an observed cancel it sets cancelled + cancels the handler but KEEPS renewing
// so the lease holds while the handler winds down — bounded by a grace deadline so
// a handler that ignores ctx cannot pin the lease forever (it is then abandoned for
// reclaim). The cancel signal rides back on the renew write itself (RETURNING
// cancel_requested), so there is no separate poll.
func (w *RunWorker) heartbeat(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
	cancelHandler context.CancelFunc,
	abandon, cancelled *atomic.Bool,
) {
	// The ticker must fire well within the lease it renews — which is the per-kind
	// lease, NOT the global DefaultLease that sized cfg.HeartbeatInterval. Derive the
	// effective interval from the actual lease (cap at lease/3, matching withDefaults'
	// DefaultLease/3) so a per-kind lease shorter than the global interval can't expire
	// before the first renewal and let another worker reclaim a still-running run.
	interval := w.cfg.HeartbeatInterval
	if third := lease / 3; third < interval {
		interval = third
	}
	// Defense-in-depth: NewHandlerRegistry rejects a sub-ms per-kind lease, so lease/3
	// is normally well above zero; this floor only guards NewTicker against a 0 from an
	// unvalidated path (e.g. a bare RunWorker built outside DI).
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	errs := 0
	// cancelDeadline bounds the post-cancel winddown for a handler that ignores ctx:
	// zero until a cancel is observed, then now + grace. It is a LOCAL safety timer
	// (single goroutine), not a lease decision — those stay DB-clock sourced.
	var cancelDeadline time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		alive, cancelRequested, err := w.repo.RenewLease(ctx, id, owner, lease)
		if err != nil {
			if ctx.Err() != nil { // shutdown / stop racing the write — leave it to the select
				return
			}
			errs++
			w.logger.Warn("run worker: heartbeat renew errored",
				logKeyRunID, id, shared.LogKeyError, err)
			if errs >= maxHeartbeatErrors {
				abandon.Store(true)
				cancelHandler()
				return
			}
			continue
		}
		errs = 0
		if !alive {
			abandon.Store(true)
			cancelHandler()
			return
		}

		// Observe an operator cancel — carried back by the same owner-checked renew, so
		// no extra round-trip and never the payload/state BLOB. Once seen (here, or
		// already by process() at claim time) cancel the handler and KEEP renewing so
		// the lease holds while it winds down.
		if cancelRequested && !cancelled.Load() {
			cancelled.Store(true)
			cancelHandler()
		}
		// Bound the winddown. A ctx-aware handler returns promptly and the heartbeat
		// exits via ctx.Done() long before this fires; the deadline only catches a
		// handler that IGNORES ctx — past it abandon, so the lease lapses for reclaim
		// (and the poison-reclaim cap) rather than renewing forever.
		if cancelled.Load() {
			switch {
			case cancelDeadline.IsZero():
				cancelDeadline = time.Now().Add(cancelGraceLeaseMultiple * lease)
			case time.Now().After(cancelDeadline):
				w.logger.Warn(
					"run worker: cancelled handler exceeded grace, abandoning for reclaim",
					logKeyRunID,
					id,
				)
				abandon.Store(true)
				cancelHandler()
				return
			}
		}
	}
}

// finalize records the run's terminal state exactly once. Order matters: a run we
// abandoned or that shut down mid-flight must NEVER be completed/failed (it is
// owner-fenced and will be reclaimed + resumed). cancelled is ranked ABOVE success
// on purpose — a handler whose ctx was cancelled and returns nil cannot be trusted
// to have finished (it may have stopped early), so it is recorded Cancelled, not
// Complete. Every finalizer is owner-checked; a false return means the lease was
// lost at the last moment → abandon.
func (w *RunWorker) finalize(
	workerCtx context.Context,
	log *slog.Logger,
	r *run.Run,
	owner string,
	hErr error,
	abandon, cancelled bool,
	dur time.Duration,
) {
	switch {
	case abandon:
		log.Warn(
			"run worker: lease lost/uncertain, abandoning for reclaim",
			shared.DurationMsAttr(dur),
		)
	case workerCtx.Err() != nil:
		// Graceful shutdown interrupted an unfinished run. Abandon — even if a
		// cancel was requested (cancel_requested survives reclaim; the next owner
		// re-observes it and finalizes cleanly).
		log.Info(
			"run worker: shutdown, abandoning in-flight run for reclaim",
			shared.DurationMsAttr(dur),
		)
	case cancelled:
		ok, err := w.repo.MarkCancelled(workerCtx, r.ID, owner)
		switch {
		case err != nil:
			log.Error("run worker: mark-cancelled errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at cancel finalize, abandoning")
		default:
			log.Info("run worker: run cancelled", shared.DurationMsAttr(dur))
		}
	case hErr == nil:
		ok, err := w.repo.MarkComplete(workerCtx, r.ID, owner)
		switch {
		case err != nil:
			log.Error("run worker: mark-complete errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at complete finalize, abandoning")
		default:
			log.Info("run worker: run completed", shared.DurationMsAttr(dur))
		}
	default:
		w.handleFailure(workerCtx, log, r, owner, hErr, dur)
	}
}

// handleFailure reschedules a retryable handler error with backoff, or fails
// terminally when the logic-retry budget is exhausted. attempts is the claim-time
// snapshot (pre this failure); Reschedule bumps it. The terminal report fires
// exactly once — only after MarkFailed actually lands.
func (w *RunWorker) handleFailure(
	ctx context.Context,
	log *slog.Logger,
	r *run.Run,
	owner string,
	hErr error,
	dur time.Duration,
) {
	if r.Attempts >= r.MaxRetries {
		ok, err := w.repo.MarkFailed(ctx, r.ID, owner, hErr.Error())
		switch {
		case err != nil:
			log.Error("run worker: mark-failed errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at fail finalize, abandoning")
		default:
			log.Error("run worker: run exhausted retries, failed",
				shared.DurationMsAttr(dur), slog.Any(shared.LogKeyError, hErr))
			w.reporter.Capture(ctx, fmt.Errorf("run %q exhausted retries: %w", r.Kind, hErr),
				slog.String(logKeyRunID, r.ID), slog.String(logKeyRunKind, r.Kind))
		}
		return
	}

	// +1: runs count attempts post-Reschedule (0 at first failure), so shift the
	// curve so the first two retries are base then 2×base, not base then base.
	delay := backoff(r.Attempts + 1)
	ok, err := w.repo.Reschedule(ctx, r.ID, owner, time.Now().Add(delay), hErr.Error())
	switch {
	case err != nil:
		log.Error("run worker: reschedule errored", shared.LogKeyError, err)
	case !ok:
		log.Warn("run worker: lease lost at reschedule, abandoning")
	default:
		log.Warn("run worker: run failed, retry scheduled",
			shared.DurationMsAttr(dur), shared.MillisAttr(shared.LogKeyRetryInMs, delay),
			slog.Any(shared.LogKeyError, hErr))
	}
}

// handleUnknownKind parks a run whose kind this binary has no handler for (a
// rolling-deploy registry skew) via Park, rather than discarding a long run's
// checkpointed state — a binary WITH the handler picks it up. Parking is gated by
// the dedicated `parks` counter (NOT attempts) against minUnknownKindParks, so the
// budget is INDEPENDENT of the handler's max_retries: a run-once run (max_retries=0)
// survives the deploy window, and parking never consumes the handler's logic-retry
// budget. A genuinely-removed kind still fails terminally once the parks budget is
// spent.
func (w *RunWorker) handleUnknownKind(
	ctx context.Context,
	log *slog.Logger,
	r *run.Run,
	owner string,
) {
	reason := fmt.Sprintf("no handler registered for kind %q (registry skew)", r.Kind)
	if r.Parks >= minUnknownKindParks {
		if ok, err := w.repo.MarkFailed(ctx, r.ID, owner, reason); err != nil {
			log.Error("run worker: unknown-kind mark-failed errored", shared.LogKeyError, err)
		} else if ok {
			log.Error("run worker: unknown kind exhausted park budget, failed", logKeyRunKind, r.Kind)
			w.reporter.Capture(ctx, fmt.Errorf("run worker: %s", reason), slog.String(logKeyRunID, r.ID))
		}
		return
	}
	delay := backoff(r.Parks + 1)
	if ok, err := w.repo.Park(ctx, r.ID, owner, time.Now().Add(delay), reason); err != nil {
		log.Error("run worker: unknown-kind park errored", shared.LogKeyError, err)
	} else if ok {
		log.Warn("run worker: unknown kind, parked for retry (registry skew)", logKeyRunKind, r.Kind)
	}
}

// checkpointer binds run.Repository.Checkpoint to one claimed run for the handler.
type checkpointer struct {
	repo  run.Repository
	id    string
	owner string
	lease time.Duration
}

func (c *checkpointer) Save(ctx context.Context, state []byte) error {
	ok, err := c.repo.Checkpoint(ctx, c.id, c.owner, state, c.lease)
	if err != nil {
		return err
	}
	if !ok {
		return runapp.ErrLeaseLost
	}
	return nil
}
