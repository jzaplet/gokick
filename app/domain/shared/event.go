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
	mu        sync.Mutex
	events    []DomainEvent
	forbidden bool
}

func NewEventCollector() *EventCollector {
	return &EventCollector{}
}

func (c *EventCollector) Collect(event DomainEvent) {
	if c.forbidden {
		panic("shared.EventCollector: Collect called from an event/job handler — " +
			"cascading events is not supported. Use shared.JobDispatcherFromContext(ctx).Enqueue(...) " +
			"for follow-up async work.")
	}
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

// forbiddenCollector marker — set by EventBus.Dispatch and worker before
// invoking a handler. Cascading Collect from inside an event/job handler
// is dropped silently otherwise; with the marker we fail fast.
var forbiddenCollector = &EventCollector{forbidden: true}

func ContextWithEventCollector(ctx context.Context) (context.Context, *EventCollector) {
	c := NewEventCollector()
	return context.WithValue(ctx, eventCollectorKey, c), c
}

// ContextWithoutEventCollector installs a forbidden-marker collector that
// panics on Collect. Call it before invoking any event handler or job
// handler — they must use JobDispatcher for follow-up async work, not
// re-emit events through a collector that no one will flush.
func ContextWithoutEventCollector(ctx context.Context) context.Context {
	return context.WithValue(ctx, eventCollectorKey, forbiddenCollector)
}

// EventCollectorFromContext returns the per-request EventCollector.
//   - Command flow (CommandBus dispatch) → real collector that gets flushed.
//   - Event / job handler → forbidden collector; calling Collect panics.
//   - Outside both (CLI bypass) → throwaway collector; Collect silently drops.
func EventCollectorFromContext(ctx context.Context) *EventCollector {
	if c, ok := ctx.Value(eventCollectorKey).(*EventCollector); ok {
		return c
	}
	return NewEventCollector()
}
