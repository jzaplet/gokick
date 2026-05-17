---
layout: 'page'
uri: '/framework/application/events'
position: 4
slug: 'framework-application-events'
parent: 'framework-application'
navTitle: 'Events'
title: 'Events'
description: 'Domain events -- jak vyhlásit "stalo se X" tak, aby na to reagoval kdokoli další, aniž by o tom command handler musel vědět.'
---

# Events


## K čemu ti to je

Command handler má dělat jednu věc. Když po vytvoření uživatele přijde požadavek "a taky pošli welcome email", nechceš, aby `CreateUserHandler` znal mailer -- jinak ho bude za měsíc znát i Slack notifier, audit a indexer.

Lepší: handler **vyhlásí event** `UserCreated` a neřeší, kdo na to reaguje. Mailer (nebo cokoli dalšího) se na ten event zaregistruje samostatně.

Tři garance:

1. **Loose coupling.** Command ví, co se stalo, ne co se má teď stát.
2. **Atomicita s commitem.** Event se rozešle handlerům až **po úspěšném commitu**. Když commit selže, event se zahodí -- žádný welcome mail pro uživatele, který v DB nevznikl.
3. **Izolace mezi requesty.** Paralelní requesty mají vlastní sběrače, nepřelévá se to.


## Krok za krokem

Scénář: po `CreateUser` poslat welcome email.

### 1. Event struct v doméně (jen primitivy)

`domain/user/user_created.go`:

```go
type UserCreated struct {
    UserID, Nickname, Email string
    Timestamp               time.Time
}

func (e UserCreated) EventName() string     { return "user.created" }
func (e UserCreated) OccurredAt() time.Time { return e.Timestamp }
```

### 2. Vyhlas event v command handleru -- **až po úspěšném zápisu**

```go
if err := h.users.Save(ctx, u); err != nil {
    return err
}
shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{...})
return nil
```

### 3. Napiš handler

`application/user/event/send_welcome_email.go`:

```go
func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event shared.DomainEvent) error {
    e := event.(user.UserCreated)
    return h.mailer.Send(e.Email, "Welcome!", /* ... */)
}
```

### 4. Zaregistruj v `provideEventHandlers`

`infrastructure/di/container_provider.go` -- jediné místo, stejný pattern jako [permissions](/guides/permissions), scheduler joby, job handlery:

```go
func provideEventHandlers(welcome *eventcmd.SendWelcomeEmailHandler) []bus.EventHandlerEntry {
    return []bus.EventHandlerEntry{
        {Event: "user.created", Handler: welcome.Handle},
    }
}
```

`make di` a hotovo.


## Co se ti hodí vědět

- **Dispatch je synchronní v request goroutině.** Pomalý handler prodlouží HTTP response. Pro odeslání mailu, externí API a podobné věci použij [Job Queue](/framework/infrastructure/job-queue) -- handler tam jen `JobDispatcherFromContext(ctx).Enqueue(...)` a vrátí se hned.
- **Když handler selže**, command už commitnul -- error se zaloguje, uživatel dostane 200/201. Z handleru nemůžeš odvolat command.
- **Eventy = primitivy** (`string`, `time.Time`), žádné entity ani VOs. Aby je mohl konzumovat cizí kontext bez importu a aby šly serializovat pro job queue.
- **Bez kaskády (strojově vynuceno)**: když event handler zavolá `Collect(...)`, runtime panic s jasnou hláškou. Pro další asynchronní práci použij `JobDispatcher`.
- **Mimo bus** (CLI `create-user`) vrátí `EventCollectorFromContext` *throwaway* sběrač -- `Collect` projde, ale eventy nikam nejdou. Žádný welcome mail pro seedovaného admina.


## Co lze nastavit

Vše v `infrastructure/di/container_provider.go`, žádné env proměnné.

| Co | Kde | Default |
|---|---|---|
| Které eventy aplikace zná | `provideEventHandlers()` | prázdný slice |
| Více handlerů na stejný event | Více entries se stejným `Event` | volají se v pořadí registrace, sériově |
| Middleware kolem dispatchu | `provideEventBus()` -- `bus.NewEventBus(...)` | `Recovery + Logging` |
| Sync → async dispatch | n/a | Není konfigurovatelné. Pro async přesuň handler do [Job Queue](/framework/infrastructure/job-queue) |
