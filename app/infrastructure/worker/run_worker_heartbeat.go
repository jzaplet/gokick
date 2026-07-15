package worker

import (
	"context"
	"gokick/app/domain/shared"
	"sync/atomic"
	"time"
)

// hbAction classifies the outcome of one heartbeat renew so the heartbeat loop
// stays flat: keep looping, stop cleanly (a shutdown raced the renew write), or
// abandon (lease lost or too many consecutive transient errors → hand the run to
// lease-lapse reclaim).
type hbAction int

const (
	hbContinue hbAction = iota
	hbStop
	hbAbandon
)

// renewOnce renews the lease for one tick and classifies the outcome, owning the
// transient-error counter and the renew logging so heartbeat's loop doesn't nest.
// The returned cancelRequested rides back on the SAME owner-checked renew write
// (no extra round-trip, never the payload BLOB) and is only meaningful on a
// successful renew (hbContinue). A shutdown/stop racing the write (ctx cancelled)
// is hbStop, not an error; maxHeartbeatErrors consecutive transient failures, or a
// lost lease (!alive), is hbAbandon.
func (w *RunWorker) renewOnce(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
	errs *int,
) (hbAction, bool) {
	alive, cancelRequested, err := w.repo.RenewLease(ctx, id, owner, lease)
	if err != nil {
		if ctx.Err() != nil { // shutdown / stop racing the write — leave it to the select
			return hbStop, false
		}
		*errs++
		w.logger.Warn("run worker: heartbeat renew errored",
			logKeyRunID, id, shared.LogKeyError, err)
		if *errs >= maxHeartbeatErrors {
			return hbAbandon, false
		}
		return hbContinue, false
	}
	*errs = 0
	if !alive {
		return hbAbandon, false
	}
	return hbContinue, cancelRequested
}

// effectiveHeartbeatInterval derives the ticker interval from the ACTUAL per-kind
// lease, not the global DefaultLease that sized base (cfg.HeartbeatInterval): cap
// at lease/3 (matching withDefaults' DefaultLease/3) so a per-kind lease shorter
// than the global interval can't expire before the first renewal and let another
// worker reclaim a still-running run. The sub-ms floor is defense-in-depth —
// NewHandlerRegistry already rejects a sub-ms per-kind lease, so this only guards
// NewTicker against a 0 from an unvalidated path (a bare RunWorker built outside DI).
func effectiveHeartbeatInterval(base, lease time.Duration) time.Duration {
	interval := base
	if third := lease / 3; third < interval {
		interval = third
	}
	if interval <= 0 {
		interval = time.Millisecond
	}
	return interval
}

// heartbeat renews the lease until ctx is cancelled (shutdown or main's stop). On
// a lost or untrustworthy lease it sets abandon + cancels the handler and exits;
// on an observed cancel it sets cancelled + cancels the handler but KEEPS renewing
// so the lease holds while the handler winds down. The winddown — for a cancelled
// handler OR one past its per-attempt deadline (execCtx) — is bounded by a grace
// deadline so a handler that ignores ctx cannot pin the lease forever (it is then
// abandoned for reclaim). The cancel signal rides back on the renew write itself
// (RETURNING cancel_requested), so there is no separate poll.
// hbHandles is the per-run heartbeat coordination surface: the handler-side
// ctx the heartbeat watches for winddown (execCtx), the cancel it fires on
// lease loss / operator cancel, and the two atomics it reports its verdict
// through. One value travels process -> startHeartbeat -> heartbeat so the
// coordination contract moves together, not as loose parameters.
type hbHandles struct {
	execCtx       context.Context
	cancelHandler context.CancelFunc
	abandon       *atomic.Bool
	cancelled     *atomic.Bool
}

func (w *RunWorker) heartbeat(
	ctx context.Context,
	id, owner string,
	lease time.Duration,
	hb hbHandles,
) {
	// The ticker must fire well within the lease it renews — the per-kind lease, not
	// the global DefaultLease that sized cfg.HeartbeatInterval (see
	// effectiveHeartbeatInterval).
	ticker := time.NewTicker(effectiveHeartbeatInterval(w.cfg.HeartbeatInterval, lease))
	defer ticker.Stop()
	errs := 0
	// graceDeadline bounds the winddown for a handler that ignores ctx (cancelled, or
	// past its per-attempt deadline): zero until winddown begins, then now + grace. It
	// is a LOCAL safety timer (single goroutine), not a lease decision — those stay
	// DB-clock sourced.
	var graceDeadline time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		action, cancelRequested := w.renewOnce(ctx, id, owner, lease, &errs)
		switch action {
		case hbStop: // shutdown / stop raced the renew write — the select handles exit
			return
		case hbAbandon: // lease lost/uncertain or too many transient errors → reclaim
			hb.abandon.Store(true)
			hb.cancelHandler()
			return
		}

		// Observe an operator cancel — carried back by the same owner-checked renew, so
		// no extra round-trip and never the payload/state BLOB. Once seen (here, or
		// already by process() at claim time) cancel the handler and KEEP renewing so
		// the lease holds while it winds down.
		if cancelRequested && !hb.cancelled.Load() {
			hb.cancelled.Store(true)
			hb.cancelHandler()
		}
		// Bound the winddown. It begins when the handler should have stopped — a cancel
		// was observed, OR its per-attempt deadline fired (execCtx). A ctx-aware handler
		// returns promptly and the heartbeat exits via ctx.Done() long before this fires;
		// the deadline only catches a handler that IGNORES ctx — past it abandon, so the
		// lease lapses for reclaim (and the poison-reclaim cap) rather than renewing
		// forever.
		if hb.cancelled.Load() || hb.execCtx.Err() != nil {
			switch {
			case graceDeadline.IsZero():
				graceDeadline = time.Now().Add(winddownGraceLeaseMultiple * lease)
			case time.Now().After(graceDeadline):
				w.logger.Warn(
					"run worker: handler exceeded winddown grace, abandoning for reclaim",
					logKeyRunID,
					id,
				)
				hb.abandon.Store(true)
				hb.cancelHandler()
				return
			}
		}
	}
}
