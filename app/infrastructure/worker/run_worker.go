package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"

	"github.com/google/uuid"
)

// Run-worker-local log keys (logKeyKinds lives in common.go; panic/stack use shared.LogKey*).
const (
	logKeyRunID       = "run_id"
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

// winddownGraceLeaseMultiple bounds, as a multiple of the lease, how long the heartbeat
// keeps renewing for a run whose handler should have stopped but hasn't — cancelled, or
// past its per-attempt timeout. A ctx-aware handler returns promptly and never reaches
// this; the bound only fires for a handler that IGNORES ctx (a contract violation) —
// past it the worker abandons, the lease lapses, and reclaim + the poison-reclaim cap
// terminate the run instead of renewing it forever.
const winddownGraceLeaseMultiple = 4

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

// RunWorker executes durable runs OUTSIDE a transaction: it claims a
// run with a per-claim owner nonce, runs the registered handler in a goroutine
// while a heartbeat renews the lease, and on worker death the lease lapses so
// another worker reclaims and resumes from the last checkpoint.
type RunWorker struct {
	id       string // per-process id; owner tokens are id + "-" + per-claim uuid
	logger   *slog.Logger
	reporter shared.ErrorReporter
	repo     run.Repository
	registry *runapp.HandlerRegistry
	// Capabilities the worker injects into the handler ctx (it bypasses the bus, where
	// these are normally injected): runDispatcher lets a handler enqueue a child task;
	// transactor backs shared.WithTx so a handler can make a few writes atomically in a
	// SHORT transaction it scopes itself. Both may be nil (a worker built without them);
	// the handler-side helpers then fall through to a no-op / an error.
	runDispatcher shared.RunDispatcher
	transactor    shared.Transactor
	cfg           RunWorkerConfig
}

func NewRunWorker(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
	repo run.Repository,
	registry *runapp.HandlerRegistry,
	runDispatcher shared.RunDispatcher,
	transactor shared.Transactor,
	cfg RunWorkerConfig,
) *RunWorker {
	return &RunWorker{
		id:            uuid.NewString(),
		logger:        logger,
		reporter:      reporter,
		repo:          repo,
		registry:      registry,
		runDispatcher: runDispatcher,
		transactor:    transactor,
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
