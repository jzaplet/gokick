---
layout: 'page'
uri: '/framework/domain/events'
position: 5
slug: 'framework-domain-events'
parent: 'framework-domain'
navTitle: 'Domain Events'
title: 'Domain Events'
description: 'Domain eventy a EventCollector – asynchronní side-effects.'
---

# Domain Events

Asynchronní side-effects po úspěšném command handleru. Eventy jsou čisté data structs s primitivy (serializovatelné).


## DomainEvent interface

```go
// domain/event.go

type DomainEvent interface {
    EventName() string
    OccurredAt() time.Time
}
```


## Příklad eventu

```go
// domain/events/user_created.go

type UserCreated struct {
    UserID    string
    Nickname  string
    Email     string
    Role      string
    Timestamp time.Time
}

func (e UserCreated) EventName() string      { return "user.created" }
func (e UserCreated) OccurredAt() time.Time  { return e.Timestamp }
```


## EventCollector

Sbírá eventy v rámci jednoho command handleru:

```go
// domain/event_collector.go

type EventCollector struct {
    events []DomainEvent
}

func (c *EventCollector) Collect(event DomainEvent) {
    c.events = append(c.events, event)
}

func (c *EventCollector) Flush() []DomainEvent {
    events := c.events
    c.events = nil
    return events
}
```


## Životní cyklus

1. Command handler volá `EventCollector.Collect(event)`
2. `TransactionMiddleware` commitne transakci
3. `DispatchEventsMiddleware` flushne eventy → async goroutiny přes EventBus
4. Event handler zpracuje side-effect (email, notifikace)

Pokud command selže (rollback), eventy se zahodí.
