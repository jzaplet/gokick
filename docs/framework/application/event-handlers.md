---
layout: 'page'
uri: '/framework/application/event-handlers'
position: 4
slug: 'framework-application-event-handlers'
parent: 'framework-application'
navTitle: 'Event Handlers'
title: 'Event Handlers'
description: 'Balíček application/event/ – zpracování domain eventů, async side-effects.'
---

# Event Handlers

Balíček `application/event/`. Zpracovávají domain eventy dispatched přes EventBus. Závisí jen na `domain/`.


## Příklad

```go
// application/event/event_send_welcome_email.go

type SendWelcomeEmailHandler struct {
    mailer Mailer
}

func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event user.UserCreated) error {
    return h.mailer.Send(event.Email, /* ... */)
}
```


## Registrace

```go
// infrastructure/di/container_provider.go

func provideEventHandlerRegistry() map[string][]EventHandler {
    return map[string][]EventHandler{
        "user.created": {sendWelcomeEmailHandler},
    }
}
```


## Použití v command handleru

```go
// application/command/command_create_user.go

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business logika ...

    u := user.NewUser(nickname, hash, cmd.Email, role)

    h.events.Collect(user.UserCreated{
        UserID:    u.ID,
        Nickname:  u.Nickname,
        Email:     u.Email,
        Role:      u.Role,
        Timestamp: time.Now(),
    })

    return h.repo.Save(ctx, u)
}
```

Event se dispatchuje až **po úspěšném commitu** (DispatchEventsMiddleware). Při rollbacku se zahodí.
