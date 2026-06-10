package middleware

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

// testEvent is a minimal DomainEvent implementation that carries a per-dispatch
// ID so the regression test can verify which command emitted which event.
type testEvent struct {
	dispatchID int
	at         time.Time
}

func (e testEvent) EventName() string     { return "test.event" }
func (e testEvent) OccurredAt() time.Time { return e.at }

// noopCommand bypasses authorization in tests — it implements SkipPermission.
type noopCommand struct{ dispatchID int }

func (noopCommand) SkipPermissionCheck() {}

// Concurrent dispatches must NOT share an EventCollector — each gets exactly
// its own event. Catches a regression to the singleton-collector pattern.
func TestDispatchEventsMiddleware_PerRequestIsolation(t *testing.T) {
	t.Parallel()

	const dispatches = 200

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := bus.NewEventBus(RecoveryMiddleware(logger, shared.NopReporter{}))

	// Handler records (sourceDispatchID, eventDispatchID) pairs. Without
	// per-request isolation we'd see pairs where sourceDispatchID != eventDispatchID.
	var (
		mu       sync.Mutex
		mismatch int32
		seen     = make(map[int]int) // dispatchID → number of events received
	)
	eventBus.Register("test.event", func(_ context.Context, e shared.DomainEvent) error {
		te := e.(testEvent)
		mu.Lock()
		seen[te.dispatchID]++
		mu.Unlock()
		return nil
	})

	commandBus := bus.NewCommandBus(
		RecoveryMiddleware(logger, shared.NopReporter{}),
		DispatchEventsMiddleware(logger, eventBus),
		// No TransactionMiddleware — we're isolating the event flow itself.
	)

	var wg sync.WaitGroup
	wg.Add(dispatches)
	for i := 0; i < dispatches; i++ {
		i := i
		go func() {
			defer wg.Done()
			cmd := noopCommand{dispatchID: i}
			err := bus.ExecVoid(
				context.Background(),
				commandBus.Bus,
				"Noop",
				cmd,
				func(ctx context.Context) error {
					// Tiny stagger to maximise interleaving across dispatches.
					time.Sleep(time.Duration(i%5) * time.Microsecond)
					shared.EventCollectorFromContext(ctx).Collect(testEvent{
						dispatchID: cmd.dispatchID,
						at:         time.Now(),
					})
					return nil
				},
			)
			if err != nil {
				atomic.AddInt32(&mismatch, 1)
			}
		}()
	}
	wg.Wait()

	if mismatch != 0 {
		t.Fatalf("dispatches that returned errors: %d", mismatch)
	}

	mu.Lock()
	defer mu.Unlock()
	if got := len(seen); got != dispatches {
		t.Fatalf(
			"seen distinct dispatchIDs: got %d want %d (cross-contamination dropped some events)",
			got,
			dispatches,
		)
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf(
				"dispatchID=%d delivered %d times (want exactly 1) — collector leaked between dispatches",
				id,
				count,
			)
		}
	}
}

// TestEventCollector_Collect_ConcurrentWriters covers the mutex on
// EventCollector itself — handlers may spawn goroutines that all Collect
// against the same per-request collector, so writes must be safe.
func TestEventCollector_Collect_ConcurrentWriters(t *testing.T) {
	t.Parallel()

	c := shared.NewEventCollector()
	var wg sync.WaitGroup
	const goroutines = 50
	const each = 100

	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				c.Collect(testEvent{dispatchID: g*each + i, at: time.Now()})
			}
		}()
	}
	wg.Wait()

	events := c.Flush()
	if len(events) != goroutines*each {
		t.Fatalf("collected events: got %d want %d", len(events), goroutines*each)
	}
}

// Event handler that calls Collect must panic (cascading events is not
// supported — handlers must use JobDispatcher for follow-up async work).
func TestEventBus_Dispatch_CollectFromHandlerPanics(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := bus.NewEventBus(RecoveryMiddleware(logger, shared.NopReporter{}))

	var panicked atomic.Bool
	eventBus.Register("test.event", func(ctx context.Context, _ shared.DomainEvent) error {
		defer func() {
			if r := recover(); r != nil {
				panicked.Store(true)
			}
		}()
		shared.EventCollectorFromContext(ctx).Collect(testEvent{dispatchID: 999})
		return nil
	})

	eventBus.Dispatch(context.Background(), testEvent{dispatchID: 1})

	if !panicked.Load() {
		t.Fatal("expected Collect from event handler to panic")
	}
}

// Verify the noopCommand satisfies SkipPermission at compile time so the
// AuthorizeMiddleware-less chain in the test above stays correct intent.
var _ shared.SkipPermission = noopCommand{}
