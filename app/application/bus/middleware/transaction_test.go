package middleware

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gokick/app/domain/shared"
)

// stubTx records BeginTx / Commit / Rollback calls so we can assert
// SkipsTransaction actually bypasses the middleware. commitErr lets a test
// simulate a failing Commit (used by the DispatchEvents+Transaction integration
// test to prove events are discarded on commit failure); commitErrs fails the
// first commits one by one before commitErr applies. It classifies errLostRace
// as retryable, the way the Postgres adapter classifies a deadlock.
type stubTx struct {
	beginCalls     int
	commitCalls    int
	rollbackCalls  int
	readBeginCalls int
	readEndCalls   int
	beginErr       error
	commitErr      error
	commitErrs     []error
}

// errLostRace stands for a transaction that lost a race (a deadlock, a
// serialization failure): stubTx.IsRetryable reports it, wrapped or not.
var errLostRace = errors.New("lost a race with a concurrent transaction")

func (s *stubTx) BeginTx(ctx context.Context) (context.Context, error) {
	s.beginCalls++
	if s.beginErr != nil {
		return ctx, s.beginErr
	}
	return ctx, nil
}

func (s *stubTx) BeginReadTx(ctx context.Context) (context.Context, func(), error) {
	s.readBeginCalls++
	if s.beginErr != nil {
		return ctx, nil, s.beginErr
	}
	return ctx, func() { s.readEndCalls++ }, nil
}

func (s *stubTx) Commit(context.Context) error {
	s.commitCalls++
	if len(s.commitErrs) > 0 {
		err := s.commitErrs[0]
		s.commitErrs = s.commitErrs[1:]
		return err
	}
	return s.commitErr
}

func (s *stubTx) Rollback(context.Context) error {
	s.rollbackCalls++
	return nil
}

func (s *stubTx) IsRetryable(err error) bool { return errors.Is(err, errLostRace) }

type normalCmd struct{}

type skipCmd struct{}

func (skipCmd) SkipTransaction() {}

var _ shared.SkipsTransaction = skipCmd{}

func TestTransactionMiddleware_WrapsByDefault(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(silent(), tx)

	_, err := mw(t.Context(), "Normal", normalCmd{}, func(context.Context) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if tx.beginCalls != 1 || tx.commitCalls != 1 || tx.rollbackCalls != 0 {
		t.Fatalf("expected begin=commit=1, rollback=0; got %+v", tx)
	}
}

func TestTransactionMiddleware_RollsBackOnHandlerError(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(silent(), tx)

	_, err := mw(t.Context(), "Normal", normalCmd{}, func(context.Context) (any, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("expected rollback=1, commit=0; got %+v", tx)
	}
}

// Commands implementing SkipsTransaction must skip BeginTx entirely.
// Regression guard: without this skip, LoginHandler self-deadlocks
// under SQLite (its raw-pool writes block on the wrapping tx).
func TestTransactionMiddleware_SkipsForOptOutCommands(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(silent(), tx)

	var ran bool
	_, err := mw(t.Context(), "Skip", skipCmd{}, func(context.Context) (any, error) {
		ran = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !ran {
		t.Fatal("handler must still run when tx is skipped")
	}
	if tx.beginCalls != 0 || tx.commitCalls != 0 || tx.rollbackCalls != 0 {
		t.Fatalf("opt-out command must touch no tx methods; got %+v", tx)
	}
}

// NOTE: the guarantee that the real LoginCommand / RefreshTokenCommand stay
// opted out of the tx is enforced by the behavioral test
// TestLoginHandler_DoesNotDeadlockUnderCommandBus (auth/command) — it dispatches
// the real command through the real bus and deadlocks if SkipTransaction() is
// removed. A compile-time assertion can't live here: arch-lint forbids the
// bus_middleware package from importing application/auth/command, so any
// assertion in this file could only reference a local dummy and would prove
// nothing about the production commands.

// Sanity: a SkipsTransaction-implementing command still composes cleanly with
// the middleware and the handler result is returned untouched.
func TestTransactionMiddleware_StillExecutesNextOnSkip(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	mw := TransactionMiddleware(silent(), tx)

	got, err := mw(t.Context(), "Skip", skipCmd{}, func(_ context.Context) (any, error) {
		return "value", nil
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got != "value" {
		t.Fatalf("result lost: %v", got)
	}
}

// ─── Retry of a transaction that lost a race ──────────────────────────────────

// runRetrying runs a TransactionMiddleware over stub tx around handler, inside
// parent event and audit collectors — the ones DispatchEvents and Audit install
// in the live chain — and returns what reached them.
func runRetrying(
	ctx context.Context,
	tx *stubTx,
	handler func(ctx context.Context) (any, error),
) (any, []shared.DomainEvent, []shared.AuditEvent, error) {
	ctx, events := shared.ContextWithEventCollector(ctx)
	ctx, audit := shared.ContextWithAuditCollector(ctx)
	mw := TransactionMiddleware(silent(), tx)
	result, err := mw(ctx, "Normal", normalCmd{}, handler)
	return result, events.Flush(), audit.Flush(), err
}

// collectingHandler records one event and one audit record per attempt (tagged
// with the attempt number), then returns the attempt's scripted error.
func collectingHandler(attempts *int, errs ...error) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		*attempts++
		shared.EventCollectorFromContext(ctx).Collect(testEvent{dispatchID: *attempts})
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
			Action: fmt.Sprintf("attempt.%d", *attempts),
		})
		if *attempts <= len(errs) {
			return nil, errs[*attempts-1]
		}
		return "ok", nil
	}
}

// A handler whose transaction lost a race runs again in a fresh transaction, and
// only the attempt that committed reaches the event and audit collectors: the
// command dispatches its events and writes its audit trail once.
func TestTransactionMiddleware_RetriesALostRace(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	attempts := 0
	lost := fmt.Errorf("update user: %w", errLostRace)

	result, events, audit, err := runRetrying(t.Context(), tx,
		collectingHandler(&attempts, lost, lost))
	if err != nil || result != "ok" {
		t.Fatalf("result=%v err=%v, want the third attempt's ok", result, err)
	}
	if attempts != 3 || tx.beginCalls != 3 || tx.rollbackCalls != 2 || tx.commitCalls != 1 {
		t.Fatalf("attempts=%d, tx %+v; want 3 attempts: 2 rolled back, 1 committed", attempts, tx)
	}
	if len(events) != 1 || events[0].(testEvent).dispatchID != 3 {
		t.Fatalf("events %v, want only the committed attempt's", events)
	}
	if len(audit) != 1 || audit[0].Action != "attempt.3" {
		t.Fatalf("audit %v, want only the committed attempt's", audit)
	}
}

// A COMMIT that loses the race (a serialization failure surfaces there) is
// retried the same way.
func TestTransactionMiddleware_RetriesACommitThatLostARace(t *testing.T) {
	t.Parallel()
	tx := &stubTx{commitErrs: []error{errLostRace}}
	attempts := 0

	_, events, _, err := runRetrying(t.Context(), tx, collectingHandler(&attempts))
	if err != nil {
		t.Fatalf("err=%v, want the retry to commit", err)
	}
	if attempts != 2 || tx.commitCalls != 2 {
		t.Fatalf("attempts=%d commits=%d, want 2 of each", attempts, tx.commitCalls)
	}
	if len(events) != 1 || events[0].(testEvent).dispatchID != 2 {
		t.Fatalf("events %v, want only the committed attempt's", events)
	}
}

// Three lost races in a row give up with the last error. No event is
// dispatched, and the audit trail holds the last attempt's records — what a
// failed command without the retry would have left.
func TestTransactionMiddleware_GivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	attempts := 0

	_, events, audit, err := runRetrying(t.Context(), tx,
		collectingHandler(&attempts, errLostRace, errLostRace, errLostRace, errLostRace))
	if !errors.Is(err, errLostRace) {
		t.Fatalf("err=%v, want the lost race", err)
	}
	if attempts != maxTxAttempts || tx.rollbackCalls != maxTxAttempts || tx.commitCalls != 0 {
		t.Fatalf("attempts=%d tx %+v, want %d rolled-back attempts", attempts, tx, maxTxAttempts)
	}
	if len(events) != 0 {
		t.Fatalf("events %v, want none from a failed command", events)
	}
	want := fmt.Sprintf("attempt.%d", maxTxAttempts)
	if len(audit) != 1 || audit[0].Action != want {
		t.Fatalf("audit %v, want only %s", audit, want)
	}
}

// Any other error is the handler's answer, not a race: no second attempt, and
// its audit records still reach the audit trail.
func TestTransactionMiddleware_DoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()
	tx := &stubTx{}
	attempts := 0
	boom := errors.New("boom")

	_, _, audit, err := runRetrying(t.Context(), tx, collectingHandler(&attempts, boom))
	if !errors.Is(err, boom) || attempts != 1 || tx.beginCalls != 1 {
		t.Fatalf("err=%v attempts=%d begins=%d, want boom after one attempt",
			err, attempts, tx.beginCalls)
	}
	if len(audit) != 1 {
		t.Fatalf("audit %v, want the failed attempt's record", audit)
	}
}

// A request that ends while the retry waits returns the lost race rather than
// running the handler again — and its audit records still reach the trail, as
// they would from a last attempt.
func TestTransactionMiddleware_StopsRetryingWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	tx := &stubTx{}
	attempts := 0

	_, _, audit, err := runRetrying(ctx, tx, func(ctx context.Context) (any, error) {
		attempts++
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{Action: "attempt.1"})
		cancel()
		return nil, errLostRace
	})
	if !errors.Is(err, errLostRace) || attempts != 1 {
		t.Fatalf("err=%v attempts=%d, want the lost race after one attempt", err, attempts)
	}
	if len(audit) != 1 || audit[0].Action != "attempt.1" {
		t.Fatalf("audit %v, want the ended attempt's record", audit)
	}
}

// The pause before a retry is jittered inside [base/2, base), the base doubling
// per attempt.
func TestRetryPause_JitteredAndDoubling(t *testing.T) {
	t.Parallel()
	for attempt := 1; attempt < maxTxAttempts; attempt++ {
		base := txRetryBackoff << (attempt - 1)
		for range 100 {
			if p := retryPause(attempt); p < base/2 || p >= base {
				t.Fatalf("attempt %d: pause %s outside [%s, %s)", attempt, p, base/2, base)
			}
		}
	}
}
