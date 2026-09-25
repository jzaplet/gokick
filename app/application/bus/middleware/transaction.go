package middleware

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// maxTxAttempts bounds how often TransactionMiddleware runs a command whose
// transaction lost a race (shared.Transactor.IsRetryable). It bounds the attempts,
// not the time: lock_timeout (APP_DB_LOCK_TIMEOUT) caps each single lock wait, and
// an attempt may wait on several locks, so a command behind a long lock holder
// can take up to three times as long as without the retry. The HTTP write timeout
// does not end it either — it drops the response, not the request's context.
const maxTxAttempts = 3

// txRetryBackoff is the base pause before the second attempt; each further one
// doubles it. The pause is jittered to [base/2, base), so two commands that
// deadlocked each other do not retry in lockstep and collide again.
const txRetryBackoff = 20 * time.Millisecond

// logKeyAttempt is the number of the attempt that lost the race.
const logKeyAttempt = "attempt"

// TransactionMiddleware runs the command in a transaction: commit when the
// handler returns nil, roll back when it fails.
//
// A transaction that lost a race with a concurrent one — a deadlock, a
// serialization failure, a lock wait past lock_timeout (shared.Transactor
// decides; on SQLite, which serializes every write, never) — is rolled back and
// the whole handler runs again in a fresh transaction, up to maxTxAttempts times.
// That is safe because nothing the handler did survives a rolled-back attempt:
//   - its writes roll back — a run it enqueued included, the INSERT joins the tx;
//   - it collects events and audit records into collectors of the attempt's own:
//     only the last attempt's reach the outer DispatchEvents / Audit middleware
//     (events only when it committed, audit records always — audit survives a
//     failure, as it would without the retry). A retried command therefore
//     dispatches its events and writes its audit trail once, not per attempt.
//
// The one write that would escape the rollback is a raw-pool write (the login
// counters, the audit log itself); the commands that make one are
// SkipsTransaction, which also skips the retry.
//
// The opt-out marker for commands that MUST run outside a bus-managed
// transaction is shared.SkipsTransaction (co-located with the other command
// declaration markers). It is required for handlers that touch raw-pool
// repositories (e.g. user.RecordFailedLogin / ResetFailedLogin) while inside
// their own command — wrapping such a handler in tx self-deadlocks under SQLite,
// because the raw-pool write blocks waiting for the very tx the handler hasn't
// returned from yet.
//
// Use sparingly. The commands that need it are the ones where "consistency across
// multiple writes" isn't actually buying anything (Login: a failed token Save
// just returns an error to the user).
func TransactionMiddleware(logger *slog.Logger, tx shared.Transactor) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		if _, skip := cmd.(shared.SkipsTransaction); skip {
			return next(ctx)
		}

		for attempt := 1; ; attempt++ {
			attemptCtx, records := collectAttempt(ctx)
			result, err := inTransaction(attemptCtx, tx, next)
			if err == nil || attempt == maxTxAttempts || !tx.IsRetryable(err) ||
				!awaitRetry(ctx, logger, name, attempt, err) {
				records.publish(ctx, err == nil)
				return result, err
			}
		}
	}
}

// awaitRetry logs the lost race and pauses before the next attempt, reporting
// false when ctx ends first — the command then ends with the lost race.
func awaitRetry(
	ctx context.Context,
	logger *slog.Logger,
	name string,
	attempt int,
	err error,
) bool {
	pause := retryPause(attempt)
	logger.LogAttrs(ctx, slog.LevelWarn, "bus: transaction lost a race, retrying",
		append(shared.LogAttrs(ctx),
			slog.String(shared.LogKeyCommand, name),
			slog.Int(logKeyAttempt, attempt),
			shared.MillisAttr(shared.LogKeyRetryInMs, pause),
			slog.Any(shared.LogKeyError, err),
		)...)
	return sleepCtx(ctx, pause)
}

// attemptRecords are the collectors of one attempt: the events and audit records
// its handler produced, held back until the attempt proves to be the last.
type attemptRecords struct {
	events *shared.EventCollector
	audit  *shared.AuditCollector
}

// collectAttempt gives an attempt collectors of its own in ctx.
func collectAttempt(ctx context.Context) (context.Context, attemptRecords) {
	ctx, events := shared.ContextWithEventCollector(ctx)
	ctx, audit := shared.ContextWithAuditCollector(ctx)
	return ctx, attemptRecords{events: events, audit: audit}
}

// publish hands the last attempt's records to the collectors in ctx — the outer
// Audit and DispatchEvents middleware's: the audit records always, the events
// only when the attempt committed.
func (r attemptRecords) publish(ctx context.Context, committed bool) {
	for _, evt := range r.audit.Flush() {
		shared.AuditCollectorFromContext(ctx).Record(evt)
	}
	if !committed {
		return
	}
	for _, evt := range r.events.Flush() {
		shared.EventCollectorFromContext(ctx).Collect(evt)
	}
}

// inTransaction runs next in one transaction: commit on success, roll back on a
// handler error.
func inTransaction(
	ctx context.Context,
	tx shared.Transactor,
	next func(ctx context.Context) (any, error),
) (any, error) {
	ctxWithTx, err := tx.BeginTx(ctx)
	if err != nil {
		return nil, err
	}

	result, err := next(ctxWithTx)

	if err != nil {
		_ = tx.Rollback(ctxWithTx)
		return nil, err
	}
	if commitErr := tx.Commit(ctxWithTx); commitErr != nil {
		return nil, commitErr
	}
	return result, nil
}

// retryPause is the jittered pause after the given failed attempt:
// txRetryBackoff doubled per attempt, drawn from [base/2, base).
func retryPause(attempt int) time.Duration {
	base := txRetryBackoff << (attempt - 1)
	return base/2 + rand.N(base/2)
}

// sleepCtx pauses for d, reporting false when ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
