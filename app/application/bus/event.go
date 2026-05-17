package bus

import (
	"context"
	"gokick/app/domain/shared"
)

type EventHandler func(ctx context.Context, event shared.DomainEvent) error

// EventHandlerEntry is one (event-name → handler) pair collected by the DI
// provider and applied during EventBus construction. Mirrors the slice-list
// pattern used by PermissionsRegistry and JobHandlerRegistry.
type EventHandlerEntry struct {
	Event   string
	Handler EventHandler
}

type EventBus struct {
	*Bus
	handlers map[string][]EventHandler
}

func NewEventBus(middlewares ...Middleware) *EventBus {
	return &EventBus{Bus: newBus(middlewares...), handlers: make(map[string][]EventHandler)}
}

// Register must only be called during DI wiring (single-goroutine init).
// Dispatch reads `handlers` without locking and is safe only because no
// registration happens after the first dispatch.
func (eb *EventBus) Register(eventName string, handler EventHandler) {
	eb.handlers[eventName] = append(eb.handlers[eventName], handler)
}

func (eb *EventBus) Dispatch(ctx context.Context, event shared.DomainEvent) {
	// Block cascading Collect from inside event handlers — they must use
	// JobDispatcher for follow-up async work.
	ctx = shared.ContextWithoutEventCollector(ctx)

	handlers := eb.handlers[event.EventName()]
	for _, h := range handlers {
		_ = ExecVoid(ctx, eb.Bus, event.EventName(), event, func(ctx context.Context) error {
			return h(ctx, event)
		})
	}
}
