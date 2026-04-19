package bus

import (
	"context"
	"gokick/app/domain/shared"
)

type EventHandler func(ctx context.Context, event shared.DomainEvent) error

type EventBus struct {
	*Bus
	handlers map[string][]EventHandler
}

func NewEventBus(middlewares ...Middleware) *EventBus {
	return &EventBus{Bus: newBus(middlewares...), handlers: make(map[string][]EventHandler)}
}

func (eb *EventBus) Register(eventName string, handler EventHandler) {
	eb.handlers[eventName] = append(eb.handlers[eventName], handler)
}

func (eb *EventBus) Dispatch(ctx context.Context, event shared.DomainEvent) {
	handlers := eb.handlers[event.EventName()]
	for _, h := range handlers {
		_ = ExecVoid(ctx, eb.Bus, event.EventName(), event, func(ctx context.Context) error {
			return h(ctx, event)
		})
	}
}
