package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	runapp "gokick/app/application/run"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"
)

// process runs one claimed run end-to-end. A top-level recover keeps a panic
// anywhere (not just the handler) from crashing the pool; on panic the run is
// abandoned (no finalize) and left for lease-lapse reclaim.
// reportProcessPanic logs a recovered process panic with its stack and reports it
// to the error tracker — mirrors the bus/HTTP RecoveryMiddleware's panic handling.
func (w *RunWorker) reportProcessPanic(
	ctx context.Context,
	log *slog.Logger,
	r *run.Run,
	rec any,
) {
	log.LogAttrs(ctx, slog.LevelError, "run worker: process panicked",
		slog.Any(shared.LogKeyPanic, rec),
		slog.String(shared.LogKeyStack, string(debug.Stack())))
	w.reporter.Capture(ctx,
		&shared.PanicError{Value: rec, Message: fmt.Sprintf("run process panic: %v", rec)},
		slog.String(logKeyRunID, r.ID), slog.String(shared.LogKeyRunKind, r.Kind))
}

func (w *RunWorker) process(workerCtx context.Context, r *run.Run, owner string) {
	// Per-run reporting scope so a terminal-failure Capture carries the run's log lines
	// as breadcrumbs (as the bus/HTTP RecoveryMiddleware does per request).
	workerCtx = w.reporter.WithRequestScope(workerCtx)
	log := w.logger.With(logKeyRunID, r.ID, shared.LogKeyRunKind, r.Kind, logKeyOwner, owner)
	defer func() {
		if rec := recover(); rec != nil {
			w.reportProcessPanic(workerCtx, log, r, rec)
		}
	}()

	if w.failIfPoison(workerCtx, log, r, owner) {
		return
	}

	handler, kindLease, kindTimeout, known := w.registry.Lookup(r.Kind)
	if !known {
		w.handleUnknownKind(workerCtx, log, r, owner)
		return
	}

	if !w.initialReLease(workerCtx, log, r, owner, kindLease) {
		return
	}

	runCtx := w.buildRunContext(workerCtx, r)

	handlerCtx, cancelHandler := context.WithCancel(runCtx)
	defer cancelHandler()
	// hbCtx is a SIBLING of handlerCtx under workerCtx: a shutdown (workerCtx)
	// stops the heartbeat, but cancelHandler (lease loss / cancel) does NOT — the
	// heartbeat must keep renewing while a cancelled handler winds down.
	hbCtx, cancelHeartbeat := context.WithCancel(runCtx)
	defer cancelHeartbeat()

	// Per-attempt timeout (0 = none): execCtx is a child of handlerCtx, so a deadline OR
	// a lease-loss/cancel (cancelHandler) both stop the handler. The heartbeat is given
	// execCtx too, so a blown deadline a NON-ctx-aware handler ignores still engages the
	// winddown bound (abandon for reclaim) instead of renewing the lease forever.
	execCtx := handlerCtx
	if kindTimeout > 0 {
		var cancelTimeout context.CancelFunc
		execCtx, cancelTimeout = context.WithTimeout(handlerCtx, kindTimeout)
		defer cancelTimeout()
	}

	var abandon, cancelled atomic.Bool
	hb := hbHandles{
		execCtx:       execCtx,
		cancelHandler: cancelHandler,
		abandon:       &abandon,
		cancelled:     &cancelled,
	}
	hbDone := w.startHeartbeat(hbCtx, r, owner, kindLease, hb)

	// Honor a cancel already requested at claim time (immutable claimed-row value;
	// no shared state read).
	if r.CancelRequested {
		cancelled.Store(true)
		cancelHandler()
	}

	start := time.Now()
	hErr := w.runHandler(execCtx, r, owner, kindLease, handler)

	cancelHeartbeat() // stop the heartbeat
	<-hbDone          // JOIN: establishes happens-before before reading the atomics

	// Drain any audit events the handler recorded, whatever the run's outcome —
	// like the bus AuditMiddleware, which persists on both success and handler
	// error. (A process panic recovers above and skips this, matching the bus,
	// where a panic short-circuits the post-next() drain.)
	w.drainAudit(workerCtx, log, runCtx)

	// A fired deadline (not merely a propagated cancel/abandon) means the attempt timed
	// out. finalize ranks abandon/shutdown/cancelled above it, so this only decides an
	// otherwise-healthy over-running attempt.
	timedOut := kindTimeout > 0 && errors.Is(execCtx.Err(), context.DeadlineExceeded)

	w.finalize(workerCtx, log, r, owner, runOutcome{
		err:       hErr,
		abandon:   abandon.Load(),
		cancelled: cancelled.Load(),
		timedOut:  timedOut,
		duration:  time.Since(start),
	})
}

// failIfPoison enforces the poison-crash-loop bound at the CLAIM boundary: a run
// that keeps killing the worker never reaches handleFailure, so the cap must fire
// here, before the handler runs (ClaimDue already returned the post-increment
// reclaims). Returns true if the run was over the cap and handled (marked failed) —
// the caller must then stop processing it.
func (w *RunWorker) failIfPoison(
	workerCtx context.Context,
	log *slog.Logger,
	r *run.Run,
	owner string,
) bool {
	if r.Reclaims <= w.cfg.MaxReclaims {
		return false
	}
	log.Error("run worker: exceeded max reclaims, failing (poison)", logKeyReclaims, r.Reclaims)
	switch ok, err := w.repo.MarkFailed(workerCtx, r.ID, owner,
		fmt.Sprintf("exceeded max reclaims (%d > %d) — poison run", r.Reclaims, w.cfg.MaxReclaims)); {
	case err != nil:
		log.Error("run worker: poison mark-failed errored", shared.LogKeyError, err)
	case !ok:
		log.Warn("run worker: lease lost at poison mark-failed, abandoning")
	default:
		w.reporter.Capture(workerCtx,
			fmt.Errorf("run %q exceeded max reclaims (poison-crash-loop)", r.Kind),
			slog.String(logKeyRunID, r.ID))
	}
	return true
}

// initialReLease closes the claim gap: ClaimDue stamped DefaultLease, so re-lease to
// the kind's lease ONLY when it differs — a per-kind lease (shorter or longer) must
// take effect before the first heartbeat tick, but for the common default-lease kind
// the claim's lease is already correct and a second write is wasted (the heartbeat
// keeps its interval sound by deriving it from kindLease itself). Returns false if
// the lease was lost or the write errored — the caller must abandon.
func (w *RunWorker) initialReLease(
	workerCtx context.Context,
	log *slog.Logger,
	r *run.Run,
	owner string,
	kindLease time.Duration,
) bool {
	if kindLease == w.cfg.DefaultLease {
		return true
	}
	alive, _, err := w.repo.RenewLease(workerCtx, r.ID, owner, kindLease)
	if err != nil {
		log.Error("run worker: initial re-lease errored, abandoning", shared.LogKeyError, err)
		return false
	}
	if !alive {
		log.Warn("run worker: lost lease immediately after claim, abandoning")
		return false
	}
	return true
}

// buildRunContext assembles the ctx a run handler executes under. The worker
// bypasses the bus, so what the bus normally injects is set here: the run's tenant
// restored, transactions forbidden (a durable run runs OUTSIDE a tx — an accidental
// BeginTx must fail closed, not freeze the DB), the RunDispatcher/Transactor
// capabilities injected (a nil one is skipped so the ctx falls through cleanly:
// no-op enqueue / WithTx returns ErrTxUnavailable), a real AuditCollector installed
// so a handler's audit.Record buffers for the post-return drain (drainAudit) — the
// worker's analogue of the bus AuditMiddleware — and the event collector forbidden
// so a handler that calls Collect fails LOUD instead of silently dropping the event.
// The asymmetry is deliberate: audit events have a durable sink here (AuditLogger),
// domain events do not (their post-commit EventBus dispatch is a bus-only concern).
func (w *RunWorker) buildRunContext(workerCtx context.Context, r *run.Run) context.Context {
	runCtx := shared.ContextForbidTx(shared.ContextWithTenantID(workerCtx, r.TenantID))
	// Restore the enqueuing request's language (stamped by the dispatcher) so
	// user-facing output produced by the run — mails, generated messages —
	// speaks the user's language. Defensive parse: anything unexpected falls
	// back to the product default rather than failing the run.
	if lang, ok := shared.ParseLang(r.Lang); ok {
		runCtx = shared.WithLang(runCtx, lang)
	}
	if w.runDispatcher != nil {
		runCtx = shared.ContextWithRunDispatcher(runCtx, w.runDispatcher)
	}
	if w.transactor != nil {
		runCtx = shared.ContextWithTransactor(runCtx, w.transactor)
	}
	if w.auditLogger != nil {
		runCtx, _ = shared.ContextWithAuditCollector(runCtx)
	}
	return shared.ContextWithoutEventCollector(runCtx)
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
			w.logger.LogAttrs(
				ctx,
				slog.LevelError,
				"run worker: handler panicked",
				slog.Any(
					shared.LogKeyPanic,
					rec,
				),
				slog.String(shared.LogKeyStack, string(debug.Stack())),
			)
			err = &shared.PanicError{Value: rec, Message: fmt.Sprintf("handler panic: %v", rec)}
		}
	}()
	ck := &checkpointer{repo: w.repo, id: r.ID, owner: owner, lease: lease}
	return handler(ctx, r, ck)
}

// startHeartbeat launches the lease-renewal goroutine and returns the channel that
// closes when it exits — the JOIN point process waits on before reading abandon/
// cancelled (that read is safe only after the join: happens-before). A panic in the
// heartbeat (e.g. a repo call) must not crash the pool: it is recovered, reported,
// and the run is abandoned (left for lease-lapse reclaim).
func (w *RunWorker) startHeartbeat(
	hbCtx context.Context,
	r *run.Run,
	owner string,
	lease time.Duration,
	hb hbHandles,
) <-chan struct{} {
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		defer func() {
			if rec := recover(); rec != nil {
				w.logger.LogAttrs(
					hbCtx,
					slog.LevelError,
					"run worker: heartbeat panicked",
					slog.Any(
						shared.LogKeyPanic,
						rec,
					),
					slog.String(shared.LogKeyStack, string(debug.Stack())),
				)
				w.reporter.Capture(
					hbCtx,
					&shared.PanicError{
						Value:   rec,
						Message: fmt.Sprintf("heartbeat panic: %v", rec),
					},
					slog.String(logKeyRunID, r.ID),
				)
				hb.abandon.Store(true)
				hb.cancelHandler()
			}
		}()
		w.heartbeat(hbCtx, r.ID, owner, lease, hb)
	}()
	return hbDone
}

// finalize records the run's terminal state exactly once. Order matters: a run we
// abandoned or that shut down mid-flight must NEVER be completed/failed (it is
// owner-fenced and will be reclaimed + resumed). cancelled is ranked ABOVE success
// on purpose — a handler whose ctx was cancelled and returns nil cannot be trusted
// to have finished (it may have stopped early), so it is recorded Cancelled, not
// Complete. timedOut ranks below cancelled but above success — an attempt that blew
// its deadline is a retryable failure (handleFailure), not a completion, even if the
// handler returned nil. Every finalizer is owner-checked; a false return means the
// lease was lost at the last moment → abandon.
// runOutcome is what one handler attempt ended with: the returned error, the
// heartbeat verdicts (read only AFTER the <-hbDone join), whether the
// per-attempt deadline fired, and the measured duration. finalize and
// handleFailure consume it as one value.
type runOutcome struct {
	err       error
	abandon   bool
	cancelled bool
	timedOut  bool
	duration  time.Duration
}

func (w *RunWorker) finalize(
	workerCtx context.Context,
	log *slog.Logger,
	r *run.Run,
	owner string,
	out runOutcome,
) {
	switch {
	case out.abandon:
		log.Warn(
			"run worker: lease lost/uncertain, abandoning for reclaim",
			shared.DurationMsAttr(out.duration),
		)
	case workerCtx.Err() != nil:
		// Graceful shutdown interrupted an unfinished run. Abandon — even if a
		// cancel was requested (cancel_requested survives reclaim; the next owner
		// re-observes it and finalizes cleanly).
		log.Info(
			"run worker: shutdown, abandoning in-flight run for reclaim",
			shared.DurationMsAttr(out.duration),
		)
	case out.cancelled:
		ok, err := w.repo.MarkCancelled(workerCtx, r.ID, owner)
		switch {
		case err != nil:
			log.Error("run worker: mark-cancelled errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at cancel finalize, abandoning")
		default:
			log.Info("run worker: run cancelled", shared.DurationMsAttr(out.duration))
		}
	case out.timedOut:
		// A blown per-attempt deadline → retryable failure (reschedule with backoff, or
		// terminal once the retry budget is spent). Ranked above success so a handler
		// that returned nil exactly as the deadline fired is still treated as timed out.
		timeoutOut := out
		timeoutOut.err = fmt.Errorf(
			"run timed out (exceeded per-attempt deadline) after %s",
			out.duration.Round(time.Millisecond),
		)
		w.handleFailure(workerCtx, log, r, owner, timeoutOut)
	case out.err == nil:
		ok, err := w.repo.MarkComplete(workerCtx, r.ID, owner)
		switch {
		case err != nil:
			log.Error("run worker: mark-complete errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at complete finalize, abandoning")
		default:
			log.Info("run worker: run completed", shared.DurationMsAttr(out.duration))
		}
	default:
		w.handleFailure(workerCtx, log, r, owner, out)
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
	out runOutcome,
) {
	if r.Attempts >= r.MaxRetries {
		ok, err := w.repo.MarkFailed(ctx, r.ID, owner, out.err.Error())
		switch {
		case err != nil:
			log.Error("run worker: mark-failed errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at fail finalize, abandoning")
		default:
			log.Error("run worker: run exhausted retries, failed",
				shared.DurationMsAttr(out.duration), slog.Any(shared.LogKeyError, out.err))
			w.reporter.Capture(ctx, fmt.Errorf("run %q exhausted retries: %w", r.Kind, out.err),
				slog.String(logKeyRunID, r.ID), slog.String(shared.LogKeyRunKind, r.Kind))
		}
		return
	}

	// +1: runs count attempts post-Reschedule (0 at first failure), so shift the
	// curve so the first two retries are base then 2×base, not base then base.
	delay := backoff(r.Attempts + 1)
	ok, err := w.repo.Reschedule(ctx, r.ID, owner, time.Now().Add(delay), out.err.Error())
	switch {
	case err != nil:
		log.Error("run worker: reschedule errored", shared.LogKeyError, err)
	case !ok:
		log.Warn("run worker: lease lost at reschedule, abandoning")
	default:
		log.Warn("run worker: run failed, retry scheduled",
			shared.DurationMsAttr(out.duration), shared.MillisAttr(shared.LogKeyRetryInMs, delay),
			slog.Any(shared.LogKeyError, out.err))
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
		switch ok, err := w.repo.MarkFailed(ctx, r.ID, owner, reason); {
		case err != nil:
			log.Error("run worker: unknown-kind mark-failed errored", shared.LogKeyError, err)
		case !ok:
			log.Warn("run worker: lease lost at unknown-kind mark-failed, abandoning")
		default:
			log.Error(
				"run worker: unknown kind exhausted park budget, failed",
				shared.LogKeyRunKind,
				r.Kind,
			)
			w.reporter.Capture(
				ctx,
				fmt.Errorf("run worker: %s", reason),
				slog.String(logKeyRunID, r.ID),
			)
		}
		return
	}
	delay := backoff(r.Parks + 1)
	switch ok, err := w.repo.Park(ctx, r.ID, owner, time.Now().Add(delay), reason); {
	case err != nil:
		log.Error("run worker: unknown-kind park errored", shared.LogKeyError, err)
	case !ok:
		log.Warn("run worker: lease lost at unknown-kind park, abandoning")
	default:
		log.Warn(
			"run worker: unknown kind, parked for retry (registry skew)",
			shared.LogKeyRunKind,
			r.Kind,
		)
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
