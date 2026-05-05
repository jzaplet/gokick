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

Event handlery zpracovávají domain eventy dispatched přes `EventBus` (Recovery → Logging). Registrují se v DI kontejneru. Boilerplate sám žádný event handler aktuálně nemá -- následující šablona ukazuje, jak by handler vypadal (např. pro odeslání welcome emailu po vytvoření uživatele):

```go
// application/user/event/send_welcome_email.go (placeholder)

type SendWelcomeEmailHandler struct {
    mailer Mailer
}

func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event user.UserCreated) error {
    return h.mailer.Send(event.Email, /* ... */)
}
```

### Sběr eventů v command handleru

Eventy se sbírají do `EventCollector` až **po úspěšném zápisu**, aby se v případě DB chyby nedispatchoval event s daty, která neuvízla:

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business logika + Save ...
    if err := h.users.Save(ctx, u); err != nil {
        return err
    }

    h.events.Collect(user.UserCreated{
        UserID:    u.ID,
        Nickname:  u.Nickname,
        Email:     u.Email,
        Role:      u.Role,
        Timestamp: time.Now(),
    })
    return nil
}
```

Bus `DispatchEventsMiddleware` flushne sbírku až po commitu transakce -- pokud Save uspěl, ale commit selže, eventy se zahodí.


## Detaily

- Eventy se dispatchují až **po úspěšném commitu** transakce (`DispatchEventsMiddleware`). Při rollbacku se zahodí.
- Event handler může selhat bez vlivu na původní command -- chyba se zaloguje, ale command už commitnul.
