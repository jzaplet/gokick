package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// Events collected during a command must be dispatched ONLY after the
// transaction commits. The architecture.md "Error flow" and roadmap-06 promise
// that a handler error (→ rollback) or a failed commit discards them. This
// composes DispatchEventsMiddleware OVER TransactionMiddleware — the live
// nesting order — around a handler that always Collects one event, and proves
// the event reaches the EventBus only on a clean commit.
func TestDispatchEventsMiddleware_DiscardsEventsUnlessCommitSucceeds(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// newChain wires DispatchEvents(Transaction(handler)) and returns a runner
	// plus a counter of how many events actually reached the bus.
	newChain := func(tx *stubTx) (func(handlerErr error) error, *int32) {
		var dispatched int32
		eventBus := bus.NewEventBus(RecoveryMiddleware(logger, shared.NopReporter{}))
		eventBus.Register("test.event", func(_ context.Context, _ shared.DomainEvent) error {
			atomic.AddInt32(&dispatched, 1)
			return nil
		})
		dispatch := DispatchEventsMiddleware(logger, eventBus)
		txmw := TransactionMiddleware(tx)
		run := func(handlerErr error) error {
			_, err := dispatch(
				context.Background(),
				"Cmd",
				normalCmd{},
				func(ctx context.Context) (any, error) {
					return txmw(ctx, "Cmd", normalCmd{}, func(ctx context.Context) (any, error) {
						shared.EventCollectorFromContext(ctx).
							Collect(testEvent{dispatchID: 1, at: time.Now()})
						return nil, handlerErr
					})
				},
			)
			return err
		}
		return run, &dispatched
	}

	t.Run("handler error rolls back and discards events", func(t *testing.T) {
		tx := &stubTx{}
		run, dispatched := newChain(tx)
		if err := run(errors.New("boom")); err == nil {
			t.Fatal("expected handler error to propagate")
		}
		if tx.rollbackCalls != 1 || tx.commitCalls != 0 {
			t.Fatalf("expected rollback=1 commit=0, got %+v", tx)
		}
		if got := atomic.LoadInt32(dispatched); got != 0 {
			t.Fatalf("events must NOT dispatch on rollback, got %d", got)
		}
	})

	t.Run("commit failure discards events", func(t *testing.T) {
		tx := &stubTx{commitErr: errors.New("commit boom")}
		run, dispatched := newChain(tx)
		if err := run(nil); err == nil {
			t.Fatal("expected commit error to propagate")
		}
		if tx.commitCalls != 1 {
			t.Fatalf("expected commit attempt, got %+v", tx)
		}
		if got := atomic.LoadInt32(dispatched); got != 0 {
			t.Fatalf("events must NOT dispatch when commit fails, got %d", got)
		}
	})

	t.Run("successful commit dispatches events", func(t *testing.T) {
		tx := &stubTx{}
		run, dispatched := newChain(tx)
		if err := run(nil); err != nil {
			t.Fatalf("clean command must succeed: %v", err)
		}
		if tx.commitCalls != 1 || tx.rollbackCalls != 0 {
			t.Fatalf("expected commit=1 rollback=0, got %+v", tx)
		}
		if got := atomic.LoadInt32(dispatched); got != 1 {
			t.Fatalf("event must dispatch after a clean commit, got %d", got)
		}
	})
}
