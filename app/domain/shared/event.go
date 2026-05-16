package shared

import (
	"context"
	"sync"
	"time"
)

type DomainEvent interface {
	EventName() string
	OccurredAt() time.Time
}

// EventCollector accumulates domain events emitted by a command handler.
// Methods are safe for concurrent use — handlers may spawn goroutines that
// all Collect against the same per-request instance.
type EventCollector struct {
	mu     sync.Mutex
	events []DomainEvent
}

func NewEventCollector() *EventCollector {
	return &EventCollector{}
}

func (c *EventCollector) Collect(event DomainEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *EventCollector) Flush() []DomainEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	events := c.events
	c.events = nil
	return events
}

type eventCollectorKeyType struct{}

var eventCollectorKey = eventCollectorKeyType{}

func ContextWithEventCollector(ctx context.Context) (context.Context, *EventCollector) {
	c := NewEventCollector()
	return context.WithValue(ctx, eventCollectorKey, c), c
}

// EventCollectorFromContext returns the per-request EventCollector. Outside
// the bus (CLI bypass) it returns a throwaway collector — handlers never
// nil-check, but emitted events go nowhere.
func EventCollectorFromContext(ctx context.Context) *EventCollector {
	if c, ok := ctx.Value(eventCollectorKey).(*EventCollector); ok {
		return c
	}
	return NewEventCollector()
}
