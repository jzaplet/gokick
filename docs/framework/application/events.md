---
layout: 'page'
uri: '/framework/application/events'
position: 4
slug: 'framework-application-events'
parent: 'framework-application'
navTitle: 'Event Handlers'
title: 'Event Handlers'
description: 'Balíček application/event/ -- domain event handlery pro side-effects.'
---

# Event Handlers


## Proč

Event handlery reagují na domain eventy -- side-effects po úspěšném commitu (notifikace, indexace, audit log). Selhání event handleru neovlivní původní command. Závisí pouze na `domain/`.


## Jak

### Event handler

Event handlery zpracovávají domain eventy dispatched přes `EventBus` (Recovery → Logging). Registrují se v DI kontejneru.

```go
// application/user/event/send_welcome_email.go

type SendWelcomeEmailHandler struct {
    mailer Mailer
}

func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event user.UserCreated) error {
    return h.mailer.Send(event.Email, /* ... */)
}
```

### Sběr eventů v command handleru

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business logika ...
    u := user.NewUser(nickname, hash, cmd.Email, role)

    h.events.Collect(user.UserCreated{
        UserID: u.ID, Nickname: u.Nickname, Email: u.Email,
    })

    return h.repo.Save(ctx, u)
}
```


## Detaily

- Eventy se dispatchují až **po úspěšném commitu** transakce (`DispatchEventsMiddleware`). Při rollbacku se zahodí.
- Event handler může selhat bez vlivu na původní command -- chyba se zaloguje, ale command už commitnul.
