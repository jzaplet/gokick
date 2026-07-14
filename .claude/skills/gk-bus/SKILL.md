---
layout: 'page'
uri: '/skills/gk-bus'
position: 10
slug: 'skills-gk-bus'
parent: 'skills-cqrs'
navTitle: 'gk-bus'
title: 'GK — CQRS bus & middleware chain'
description: 'CQRS busy (Command/Query/Event), middleware chain a jeho pořadí, dispatch přes Dispatch/DispatchVoid/Query. Use when posíláš command/query/event z HTTP handleru nebo CLI, řešíš pořadí middleware (transakce, autorizace, audit, eventy), nebo přidáváš nový handler a nevíš, kudy teče.'
name: 'gk-bus'
---

# GK — CQRS bus & middleware chain

Každý command/query/event teče přes `*bus.Bus` — sdílený řetězec middleware, který
kolem business logiky obalí cross-cutting věci (recovery, logging, autorizaci,
transakci, audit, eventy). Handler tak řeší jen byznys, nic víc.

## What & when
- Sáhni sem, když z HTTP handleru nebo CLI posíláš command/query/event a chceš
  vědět, **jak** ho dispatchnout (`bus.Dispatch` / `bus.DispatchVoid` / `bus.Query`).
- Když řešíš **pořadí** middleware — proč audit přežije rollback, proč se eventy
  dispatchnou až po commitu, kdy běží autorizace.
- Když přidáváš nový handler a ladíš „proč mi to vrací error, že chybí permission".
- **NEtýká** se psaní samotných handlerů (→ `/gk-commands`, `/gk-queries`),
  domain eventů (→ `/gk-domain-events`), ani DI registrace busů (→ `/gk-di`). — a stejně opravit oba odkazy v Related (`/gk-domain-events`, `/gk-di`).

## For non-tech / juniors
Představ si bus jako **pásovou linku ve fabrice**. Na začátek položíš požadavek
(„vytvoř uživatele"). Než dojede na konec, projede několika **stanicemi** za
sebou: jedna zkontroluje, že na to máš oprávnění; další otevře „transakci"
(bezpečnou bublinu, kde se buď uloží všechno, nebo nic); další pásovou linku
hlídá, aby případný pád (panic) neshodil celý server. Tvůj kód (vlastní výroba)
sedí až **na konci** linky a řeší jen samotnou věc — o zbytek se postarají
stanice okolo. Tři linky existují proto, že **zápis** (Command) potřebuje jiné
stanice než **čtení** (Query) nebo **reakce po uložení** (Event).

## How it works
Jádro je `app/application/bus/bus.go`: typ `Middleware` a privátní `newBus(...)`.
Čtyři veřejné typy obalují stejný `*Bus`:

| Typ | Soubor | Použití |
|---|---|---|
| `CommandBus` | `command.go` | zápisy z HTTP (mění stav) |
| `SystemCommandBus` | `system_command.go` | zápisy z CLI (operator-trusted: bez Authorize/Tenant) |
| `QueryBus` | `query.go` | čtení (nic nemění) |
| `EventBus` | `event.go` | side-effects po commitu |

**Dispatch funkce** (typově bezpečné generiky, `dispatch.go`) — berou obalový typ busu, takže párování bus↔operace kontroluje kompilátor:
- `bus.Dispatch[R](ctx, commandBus, name, cmd, fn) (R, error)` — command s návratem.
- `bus.DispatchVoid(ctx, commandBus, name, cmd, fn) error` — command bez návratu.
- `bus.Query[R](ctx, queryBus, name, q, fn) (R, error)` — čtení.
- `bus.SystemDispatch[R]` / `bus.SystemDispatchVoid` — operator-trusted CLI přes `SystemCommandBus`.
(Interní jádro `exec`/`execVoid` v `exec.go`/`void.go` je neexportované — vnitřní `*Bus` se ven nedostane.)

Parametr `cmd any` slouží middleware k introspekci (např. type-assert na
`shared.Permissioned`). `name` je jen lidský štítek do logu.

**Řetězce middleware** jsou single-sourced v `middleware/base.go` (`BaseChain`, `CommandChain`, `SystemChain`) — DI (`app/infrastructure/di/container_provider.go`) je jen volá, a testfx staví bus ze stejného zdroje, takže pořadí nemůže driftovat. `busmw.BaseChain(...)` (`middleware/base.go`) je sdílený základ
**Recovery → Logging → Authorize → Tenant**:

| Bus | Chain (pořadí) |
|---|---|
| `CommandBus` | Recovery → Logging → Authorize → Tenant → **Audit → RunDispatcher → DispatchEvents → Transaction** |
| `SystemCommandBus` | Recovery → Logging → **Audit → RunDispatcher → DispatchEvents → Transaction** (bez Authorize/Tenant — operator-trusted CLI, tenant injectovaný explicitně; RunDispatcher zůstává, aby i CLI command mohl durably enqueue run) |
| `QueryBus` | Recovery → Logging → Authorize → Tenant |
| `EventBus` | Recovery → Logging |

Co která stanice dělá (`app/application/bus/middleware/`):

- **Recovery** (`recovery.go`) — zachytí panic, zaloguje stack, zabalí do
  `shared.PanicError` (→ 500) a nahlásí přes `shared.ErrorReporter`. Jediné
  místo, kde se panic reportuje — běžné errory ne.
- **Logging** (`logging.go`) — „bus: executing/completed/failed" + `duration_ms`.
- **Authorize** (`authorize.go`) — command/query **musí** implementovat
  `shared.Permissioned` (vrací permission string) nebo `shared.SkipPermission`
  (explicitní opt-out). Když ani jedno → middleware vrátí error. Volá
  `PermissionChecker.Check()`.
- **Tenant** (`tenant.go`) — resolvne aktivní tenant a uloží ho do `ctx`, takže
  každý downstream handler i repozitář vidí stejný tenant. Sedí v `BaseChain`,
  takže ho dostane CommandBus **i** QueryBus — čtení potřebuje tenant scoping
  stejně jako zápis. Chyba resolveru command/query zastaví (fail-closed).
  Detail → `/gk-multitenancy`.
- **Audit** (`audit.go`) — po handleru vydrénuje `AuditCollector` a zapíše přes
  `AuditLogger`. Sedí **vně** Transaction, takže security záznamy
  (`auth.login.failed`, …) přežijí i rollback. Selhání zápisu se jen zaloguje,
  nikdy nepropaguje volajícímu.
- **RunDispatcher** (`run_dispatcher.go`) — vloží `RunDispatcher` do `ctx`.
  Enqueue z handleru se pak přes `Conn(ctx)` připojí k business transakci (atomický
  zápis + enqueue runu; samotný handler pak běží mimo transakci).
- **DispatchEvents** (`events.go`) — vytvoří **per-request** `EventCollector`
  v `ctx`. Obaluje Transaction (je vně). Až po **úspěšném commitu** vyprázdní
  sebrané eventy a rozešle je přes `EventBus` synchronně. Při chybě/rollbacku
  se eventy zahodí.
- **Transaction** (`transaction.go`) — `BeginTx`/`Commit`/`Rollback` přes
  `shared.Transactor` (duck typing, `SqliteManager` ho implementuje). Command
  může opt-outnout přes marker `shared.SkipsTransaction` — ze dvou různých
  důvodů: raw-pool zápisy (Login, jinak SQLite self-deadlock) a cleanup,
  který musí přežít vrácený error (RefreshToken — theft/expiry smaže tokeny
  a vrátí `AuthError`; uvnitř tx by rollback force-logout zrušil).

`EventBus.Register(name, handler)` se volá **jen při DI wiringu** (single-goroutine
init) — `event.go` čte mapu bez zámku, což je safe jen díky tomu. Dispatch navíc
blokuje kaskádový `Collect` z event handlerů (`ContextWithoutEventCollector`).

## Recipe
### Recipe: dispatch query z HTTP handleru (čtení)
```go
users, err := bus.Query(
    r.Context(),
    h.queryBus,                // *QueryBus — párování hlídá kompilátor
    "ListUsers",               // štítek do logu
    q,                         // query value (musí mít Permissioned/SkipPermission)
    func(ctx context.Context) ([]user.User, error) {
        return h.listUsers.Handle(ctx, q)
    },
)
if err != nil { h.resp.HandleError(r.Context(), w, err); return }
```
(reálný vzor: `app/presentation/http/handler/admin_users.go:76`)

### Recipe: dispatch command z HTTP handleru (zápis)
```go
err := bus.DispatchVoid(
    r.Context(), h.commandBus, "CreateUser", cmd,
    func(ctx context.Context) error { return h.createUser.Handle(ctx, cmd) },
)
```
(reálný vzor: `app/presentation/http/handler/admin_users.go:139`)

### Recipe: nová stanice (middleware)
1. Napiš `func XxxMiddleware(deps...) bus.Middleware` v `middleware/`.
2. Zařaď ji do správného chainu v `middleware/base.go` (`BaseChain`/`CommandChain`/`SystemChain`) — pozor na pořadí
   (vnější obaluje vnitřní; `BaseChain` je vždy první).
3. `make di` (regeneruje `wire_gen.go`) → `make test`.

## Invariants & pitfalls
- **Vždy přes bus.** Nikdy nevolej handler přímo z HTTP handleru — přišel bys
  o recovery, autorizaci, transakci i eventy.
- **Permission deklarace povinná.** Zapomenuté `Permissioned`/`SkipPermission` =
  runtime error z AuthorizeMiddleware, ne tichý průchod.
- **Pořadí drží pravidla.** Audit je **vně** Transaction schválně (přežije
  rollback). DispatchEvents **obaluje** Transaction (eventy jen po commitu).
  Nepřehazuj je.
- **Query nemá transakci ani eventy.** `QueryBus` = Recovery/Logging/Authorize/Tenant.
  Transakci a eventy nepotřebuje — je read-only. **Tenant ale běží i pro query**:
  čtení musí být tenant-scoped stejně jako zápis.
- **`Register` jen při DI.** Registrace event handleru po prvním dispatchi je
  data race (mapa se čte bez zámku) — proto se dělá jen v `provideEventBus`.
- **`SkipsTransaction` jen výjimečně** — dnes jen dva legitimní důvody:
  raw-pool zápisy (Login, jinak SQLite self-deadlock) a cleanup přeživší
  vrácený error (RefreshToken, force-logout po theft/expiry). Ne jako
  pohodlný útěk z transakce.

## Related
- Skills: `/gk-commands`, `/gk-queries` (psaní handlerů), `/gk-events`
  (domain eventy), `/gk-audit` (audit trail), `/gk-runs` (durable runs),
  `/gk-wire` (DI registrace busů)
- Kód: `app/application/bus/` (`bus.go`, `command.go`, `system_command.go`, `query.go`, `event.go`, `dispatch.go`, `exec.go`, `void.go`), `app/application/bus/middleware/` (`base.go`,
  `recovery.go`, `logging.go`, `authorize.go`, `tenant.go`, `audit.go`, `run_dispatcher.go`,
  `events.go`, `transaction.go`), `app/infrastructure/di/container_provider.go`,
  `app/domain/shared/permission.go`
