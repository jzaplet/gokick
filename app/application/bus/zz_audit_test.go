package bus

import (
	"context"
	"testing"
	"time"

	"gokick/app/domain/shared"
)

// auditTestEvent is a minimal shared.DomainEvent implementation used by the
// EventBus dispatch tests in this file. It carries no payload — the tests only
// need a stable EventName() to route handlers.
type auditTestEvent struct{ at time.Time }

func (auditTestEvent) EventName() string       { return "audit.test.event" }
func (e auditTestEvent) OccurredAt() time.Time { return e.at }

// Closes app-events-audit-12: when several handlers are registered for the SAME
// event name, EventBus.Dispatch must invoke them serially, in registration
// order. Register appends to handlers[eventName] (event.go:31) and Dispatch
// ranges over that slice (event.go:40), so each handler records its registration
// index into a shared slice; after a single Dispatch the recorded order must be
// exactly [0,1,2]. A regression to map-keyed storage (random iteration), a
// prepend, or any reordering would break the expected sequence. No middleware is
// wired (NewEventBus with zero middlewares runs the bare handler chain), so the
// assertion isolates the ordering guarantee of Dispatch itself.
func TestEventBus_Dispatch_MultipleHandlersRunSeriallyInRegistrationOrder(t *testing.T) {
	t.Parallel()

	eventBus := NewEventBus()

	var order []int
	for i := 0; i < 3; i++ {
		i := i
		eventBus.Register("audit.test.event", func(_ context.Context, _ shared.DomainEvent) error {
			// Serial execution means no locking is required around this append;
			// if Dispatch ever ran handlers concurrently this would also race
			// (caught under `go test -race`).
			order = append(order, i)
			return nil
		})
	}

	eventBus.Dispatch(context.Background(), auditTestEvent{at: time.Now()})

	if len(order) != 3 {
		t.Fatalf("expected all 3 handlers to run, got %d invocations: %v", len(order), order)
	}
	for idx, got := range order {
		if got != idx {
			t.Fatalf("handlers ran out of registration order: got %v want [0 1 2]", order)
		}
	}
}
