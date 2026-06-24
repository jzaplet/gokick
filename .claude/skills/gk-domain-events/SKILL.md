---
layout: 'page'
uri: '/skills/gk-domain-events'
position: 20
slug: 'skills-gk-domain-events'
parent: 'skills-domain'
navTitle: 'gk-domain-events'
title: 'GK — Domain events'
description: 'Vyhlášení "stalo se X" tak, aby na to reagoval kdokoli další, aniž by command handler musel vědět kdo — per-request sběrač, primitivní payloady, synchronní rozeslání až po commitu. Use when chceš po úspěšném commandu spustit vedlejší efekt (notifikace, indexace, follow-up) bez toho, aby ho command handler znal.'
name: 'gk-domain-events'
---

# GK — Domain events

Command handler vyhlásí, že se něco stalo (`UserCreated`), a neřeší kdo na to
reaguje. Eventy se sesbírají v rámci jednoho commandu a rozešlou se **až po
úspěšném commitu** — když command spadne, zahodí se.

## What & when

- Sáhni sem, když po nějakém commandu (`CreateUser`, …) chceš spustit vedlejší
  efekt — poslat welcome mail, zaindexovat, upozornit jiný kontext — a nechceš,
  aby command handler znal mailer/indexer/notifier.
- NEtýká se: práce, co musí přežít restart/crash nebo trvá dlouho (externí API,
  mail) → patří do job queue, ne do synchronního event handleru (viz `/gk-jobs`).
  Audit logu — ten má vlastní cestu (`AuditCollector`), ne eventy.

## For non-tech / juniors

Představ si event jako **veřejné oznámení na nástěnce**: „byl založen uživatel".
Ten, kdo oznámení vyvěsil (command handler), neřeší, kdo si ho přečte. Kdokoli
další (odesílač mailů, …) se může samostatně přihlásit „když uvidíš tohle
oznámení, udělej tamto". Klíčové: oznámení se rozešle **až když je změna
opravdu uložená v databázi**. Když uložení selže, oznámení se zahodí — nikdy
nepošleme welcome mail uživateli, který ve skutečnosti nevznikl.

## How it works

**`DomainEvent`** je jednoduchý interface (`app/domain/shared/event.go`):
`EventName() string` + `OccurredAt() time.Time`. Event je čistá data struct
**jen z primitivů** (`string`, `time.Time`), aby šel serializovat a aby ho mohl
přečíst i cizí kontext bez importu.

Reálný (a zatím jediný) event — `app/domain/user/user_created.go`:

```go
type UserCreated struct {
    UserID, Nickname, Email, Role string
    Timestamp                     time.Time
}
func (e UserCreated) EventName() string     { return "user.created" }
func (e UserCreated) OccurredAt() time.Time { return e.Timestamp }
```

**`EventCollector`** (`app/domain/shared/event.go`) sbírá eventy v rámci jednoho
commandu. Je thread-safe (mutex) a má příznak `forbidden`. Instance je
**per-request** — žádný singleton, takže se paralelní requesty nepřelévají.

**Tok přes `CommandBus`** (chain v `container_provider.go`, `provideCommandBus`):

```
… → DispatchEventsMiddleware → TransactionMiddleware → handler
```

`DispatchEventsMiddleware` (`app/application/bus/middleware/events.go`) **obaluje**
`TransactionMiddleware` (je vně). Pořadí dělá tu záruku:

1. `DispatchEventsMiddleware` vytvoří per-request collector a uloží ho do `ctx`
   (`shared.ContextWithEventCollector`).
2. `TransactionMiddleware` otevře transakci.
3. Handler volá `shared.EventCollectorFromContext(ctx).Collect(event)` — typicky
   hned po úspěšném `Save` (viz `create_user.go:77`).
4. Transakce se potvrdí (commit), nebo se při chybě vrátí zpět (rollback).
5. Když `next` vrátí chybu, middleware ji jen propaguje a **flush přeskočí**
   (eventy zmizí). Když je commit OK, `collector.Flush()` vrátí eventy a každý se
   pošle přes `eventBus.Dispatch(ctx, event)` — **synchronně, v request goroutině**.

**`EventBus.Dispatch`** (`app/application/bus/event.go`) najde handlery podle
`EventName()` a volá je **sériově, v pořadí registrace**. Před voláním nainstaluje
„forbidden" collector (`ContextWithoutEventCollector`), takže když event handler
zkusí `Collect`, runtime **panicne** s jasnou hláškou — kaskáda eventů není
podporovaná.

**Registrace** je jediné místo — `provideEventHandlers()` v
`app/infrastructure/di/container_provider.go` (stejný slice-list pattern jako
permissions / scheduler joby / job handlery). Dnes vrací prázdný slice; složka
`app/application/user/event/` je zatím prázdná (`.gitkeep`) — **žádný event
handler ještě není nasazený**, jen se eventy sbírají a logují.

## Recipe

Cíl: po `CreateUser` spustit nový vedlejší efekt na event `user.created`.

1. **Event** už existuje (`app/domain/user/user_created.go`) a emituje se
   (`create_user.go`). Pro nový event přidej struct jen z primitivů + `EventName()`
   + `OccurredAt()` do `domain/<kontext>/` a `Collect` ho v handleru **až po
   úspěšném zápisu**.
2. **Handler** napiš do `app/application/<kontext>/event/`. Signatura:
   `func (h *XHandler) Handle(ctx context.Context, event shared.DomainEvent) error`.
   Uvnitř si event přetypuj: `e := event.(user.UserCreated)`.
3. **Zaregistruj** ho v `provideEventHandlers()`:
   ```go
   {Event: "user.created", Handler: welcome.Handle},
   ```
   (přidej i `wire.Build` provider pro samotný handler, pokud má závislosti).
4. `make di` → `make arch-check` → `make test`.

## Invariants & pitfalls

- **Eventy = jen primitivy.** Nikdy nedávej do eventu entity ani value objects —
  musí jít serializovat a číst z cizího kontextu bez importu.
- **`Collect` až po úspěšném zápisu**, ne před. Když handler vrátí chybu / commit
  selže, eventy se zahodí — ale logika musí být „nejdřív ulož, pak vyhlas".
- **Dispatch je synchronní** v request goroutině → pomalý handler prodlouží HTTP
  response. Pro mail / externí API použij job queue (`/gk-jobs`), ne event handler.
- **Žádná kaskáda.** `Collect` z event handleru **panicne** (forbidden collector
  nastavený v `EventBus.Dispatch`). Pro follow-up async práci sáhni po
  `shared.JobDispatcherFromContext(ctx).Enqueue(...)`.
- **Handler nemůže odvolat command.** Když selže, command už commitnul; chyba se
  jen zaloguje (`RecoveryMiddleware` na EventBusu), uživatel dostal 2xx.
- **Mimo bus** (přímé volání handleru v testech) vrátí `EventCollectorFromContext`
  *jednorázový* sběrač — `Collect` projde, ale nikam to nejde. CLI commandy teď
  jedou přes `SystemCommandBus` (má DispatchEvents), takže event z `create-user` se
  po commitu rozešle; seeder ale eventy nesbírá (staví entity přímo) — žádný welcome
  mail pro seedovaného admina.
- **Registruj jen v `provideEventHandlers()`** během DI initu (single-goroutine).
  `EventBus.Dispatch` čte mapu handlerů bez zámku — registrace po prvním dispatchu
  by byl data race.

## Related

- Sousední skills: `/gk-jobs` (perzistentní async fronta — sem patří mail/externí
  volání), `/gk-config` (Wire DI registrace handlerů)
- Kód: `app/domain/shared/event.go` (interface + collector),
  `app/application/bus/middleware/events.go` (dispatch po commitu),
  `app/application/bus/event.go` (`EventBus`, `Register`, `Dispatch`),
  `app/domain/user/user_created.go` (vzorový event),
  `app/application/user/command/create_user.go` (emit site),
  `app/infrastructure/di/container_provider.go` (`provideEventHandlers` / `provideEventBus`)
