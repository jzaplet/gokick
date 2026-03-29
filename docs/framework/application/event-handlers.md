---
layout: 'page'
uri: '/framework/application/event-handlers'
position: 4
slug: 'framework-application-event-handlers'
parent: 'framework-application'
navTitle: 'Event Handlery'
title: 'Event Handlery'
description: 'Balíček event/ – zpracování domain eventů, async side-effects.'
---

# Event Handlery

Balíček `event/`. Zpracovávají domain eventy dispatched přes EventBus. Závisí jen na `domain/`.


## Příklad

```go
// event/send_welcome_email.go

type SendWelcomeEmailHandler struct {
    mailer Mailer
}

func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event domain.UserCreated) error {
    return h.mailer.Send(event.Email, /* ... */)
}
```


## Registrace

```go
// di_container/container_provider.go

func provideEventHandlerRegistry() map[string][]EventHandler {
    return map[string][]EventHandler{
        "user.created": {sendWelcomeEmailHandler},
    }
}
```


## Použití v command handleru

```go
// command/create_user.go

func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business logika ...

    user := domain.NewUser(nickname, hash, cmd.Email, role)

    h.events.Collect(domain.UserCreated{
        UserID:    user.ID,
        Nickname:  user.Nickname,
        Email:     user.Email,
        Role:      user.Role,
        Timestamp: time.Now(),
    })

    return h.repo.Save(ctx, user)
}
```

Event se dispatchuje až **po úspěšném commitu** (DispatchEventsMiddleware). Při rollbacku se zahodí.
