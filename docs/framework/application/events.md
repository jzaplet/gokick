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

`EventCollector` je per-request — vytváří ho `DispatchEventsMiddleware` na začátku každého dispatch a ukládá do `ctx`. Handler ho čte přes `shared.EventCollectorFromContext(ctx)` a sbírá až **po úspěšném zápisu**, aby se v případě DB chyby nedispatchoval event s daty, která neuvízla:

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business logika + Save ...
    if err := h.users.Save(ctx, u); err != nil {
        return err
    }

    shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
        UserID:    u.ID,
        Nickname:  u.Nickname,
        Email:     u.Email,
        Role:      u.Role,
        Timestamp: time.Now(),
    })
    return nil
}
```

`DispatchEventsMiddleware` sedí v bus chainu vně `TransactionMiddleware`. Pokud commit selže, chyba propaguje zpět do middleware a eventy se nedispatchují.


## Detaily

- Collector žije v contextu, ne v DI — eliminuje race condition u paralelních commandů (každý dispatch má vlastní izolovanou sbírku).
- Eventy se dispatchují až **po úspěšném commitu** transakce. Při rollbacku se zahodí (chyba propaguje z `TransactionMiddleware` skrz `DispatchEventsMiddleware`, post-processing flush se přeskočí).
- Dispatch je **synchronní** — `EventBus.Dispatch` volá handlery sériově v request goroutině. Heavy I/O handlery (emaily, externí API) patří do perzistentní job queue (viz [Roadmap fáze 3](/roadmap#fáze-3--perzistentní-job-queue-sqlite)), ne do `EventBus`.
- `EventCollectorFromContext` u handleru spuštěného mimo bus (např. CLI `create-user`, který bus bypassuje) vrací prázdný throwaway collector — eventy se tiše zahodí.
- Event handler může selhat bez vlivu na původní command -- chyba se zaloguje, ale command už commitnul.
