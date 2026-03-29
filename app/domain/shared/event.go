package shared

import "time"

type DomainEvent interface {
	EventName() string
	OccurredAt() time.Time
}

type EventCollector struct {
	events []DomainEvent
}

func NewEventCollector() *EventCollector {
	return &EventCollector{}
}

func (c *EventCollector) Collect(event DomainEvent) {
	c.events = append(c.events, event)
}

func (c *EventCollector) Flush() []DomainEvent {
	events := c.events
	c.events = nil
	return events
}
