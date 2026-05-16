---
layout: 'page'
uri: '/framework/application/event-flow'
position: 5
slug: 'framework-application-event-flow'
parent: 'framework-application'
navTitle: 'Event Flow'
title: 'Event Flow'
description: 'End-to-end tok domain eventů -- per-request collector, middleware order, atomicita s commitem, synchronní dispatch.'
---

# Event Flow

End-to-end pohled na to, jak se v gokicku eventy sbírají, dispatchují a doručují k handlerům. Komplementární stránka k [Bus](/framework/application/bus) (mechanika middleware) a [Event Handlers](/framework/application/events) (jak psát handler).


## Proč

Domain event = "stalo se X" oznámení z command handleru bez znalosti, kdo na to reaguje. Tok musí splnit tři tvrdé garance:

1. **Atomicita s commitem.** Pokud transakce rollbackne nebo commit selže, eventy se zahodí. Žádný "uživatel vytvořen" mail pro uživatele, který v DB nikdy nevznikl.
2. **Izolace mezi requesty.** Paralelní commandy mají vlastní sběrače, žádné cross-contamination eventů mezi nezávislými dispatchi.
3. **Bez ztráty contextu.** Handler dostane `ctx` s `TraceID`, takže log řádek z odeslaného mailu se spojí s původním HTTP requestem.

První dvě garance byly v boilerplatu opraveny ve [fázi 1 roadmapy](/roadmap#fáze-1--stabilizace-event-flow--shutdown) — předtím byl collector singleton a dispatch běžel před commitem.


## Jak

### Tři komponenty

| Komponenta | Soubor | Role |
|---|---|---|
| `EventCollector` | `domain/shared/event.go` | Per-request sběrač s `sync.Mutex`. Žije v contextu, vytvořený middleware. |
| `DispatchEventsMiddleware` | `application/bus/middleware/events.go` | Vytvoří collector, po commitu flushne a dispatchne. |
| `EventBus` | `application/bus/event.go` | Registr handlerů + synchronní volání. |

### Middleware chain v CommandBus

```
Recovery → Logging → Authorize → DispatchEvents → Transaction → handler
                                  └── outer ─┘     └── inner ─┘
```

`DispatchEvents` **wrapuje** `Transaction`. Důsledek: middleware vidí výsledek commitu. Pokud commit selže, chyba propaguje skrz `DispatchEvents` a flush se přeskočí.

### Tok při úspěšném commandu

1. `DispatchEvents` pre — vytvoří `EventCollector`, uloží do `ctx`.
2. `Transaction` BeginTx.
3. Handler volá `shared.EventCollectorFromContext(ctx).Collect(event)`.
4. `Transaction` commit → vrátí `nil`.
5. `DispatchEvents` post — `collector.Flush()` → `eventBus.Dispatch(ctx, event)` synchronně pro každý event.

### Tok při chybě

Handler vrátí err **nebo** commit selže:

1. `DispatchEvents` pre — collector do `ctx`.
2. `Transaction` BeginTx → handler vrátí err → Rollback, nebo handler OK → commit selže.
3. Err propaguje do `DispatchEvents` → post-processing flush se přeskočí.

Eventy jsou zahozeny i v případě, že handler je již `Collect`oval — collector v ctx jednoduše zanikne s ctx.

### V handleru — pattern

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... validace, repo.Save(...) ...

    shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
        UserID:    u.ID,
        Nickname:  u.Nickname,
        Timestamp: time.Now(),
    })
    return nil
}
```

`EventCollectorFromContext` vrací non-nil collector **vždy**. Pokud handler běží mimo bus (CLI bypass — `./bin/app create-user`), dostane throwaway instanci → `Collect` proběhne, ale eventy se nikam neposílají.


## Detaily

### Synchronní dispatch

`EventBus.Dispatch` volá handlery sériově v request goroutině. To znamená:

- Pomalý handler **prodlužuje HTTP response time**.
- Selhání handleru se zaloguje (recovery middleware), ale původní command už commitnul — vrátí 200/201 i když handler selhal.

Pro long-running / I/O-heavy side-effects (mail, externí API, retry-prone práce) **nepoužívejte event handler** — patří do perzistentní [job queue (roadmap F3)](/roadmap#fáze-3--perzistentní-job-queue-sqlite).

### Bez kaskády eventů

Event handler **nesmí** volat `EventCollectorFromContext(ctx).Collect(...)`. `DispatchEvents` volá `Flush()` jen jednou — pokud handler během dispatch přidá nový event, middleware ho už nevyzvedne a tiše se ztratí. Pokud handler potřebuje další asynchronní práci, sáhne po `JobDispatcher` (F3).

### Eventy = primitiva, ne entity

Eventy přenáší jen primitivy (`string`, `time.Time`, `int`), nikdy `*User` ani VOs. Důvody:

- **Cross-domain handler v jiném bounded contextu** neimportuje entity — `domain/user/UserCreated` může konzumovat handler v jiném kontextu bez závislosti na `domain/user/`.
- **Forward compatibility s job queue (F3)** — eventy uvozené primitivy se snadno serializují do JSON payloadu.
- **Žádný lazy-load hazard** při deserializaci v jiném procesu.

### Throwaway collector pro CLI bypass

`./bin/app create-user` volá `*CreateUserHandler.Handle` přímo, mimo bus. V `ctx` žádný collector není; `EventCollectorFromContext` vrátí čerstvou throwaway instanci. Handler se chová identicky jako přes HTTP, jen emitované eventy nikam nejdou — pro one-shot CLI příkaz je to žádoucí default (žádný welcome mail pro seedovaného admina).

### Mušštex na `EventCollector`

`Collect` a `Flush` jsou pod `sync.Mutex`. Defenzivní opatření pro případ, že handler spustí goroutiny, které všechny `Collect`ují — middleware se vždy dočká korektního snapshot stavu při flushu.

### Regresní test

`app/application/bus/middleware/events_test.go::TestDispatchEventsMiddleware_PerRequestIsolation` — 200 paralelních dispatchů přes `CommandBus`, ověří, že každý dispatch dostane unikátní event přesně 1× (žádné cross-contamination). Chrání proti regresi zpět na singleton collector.


## Kam dál

| Téma | Odkaz |
|---|---|
| Jak napsat event handler | [Event Handlers](/framework/application/events) |
| Middleware chain mechanika | [Bus](/framework/application/bus) |
| Error typy + DomainEvent rozhraní | [Errors & Events](/framework/domain/errors-events) |
| Plán pro perzistentní job queue | [Roadmap F3](/roadmap#fáze-3--perzistentní-job-queue-sqlite) |
