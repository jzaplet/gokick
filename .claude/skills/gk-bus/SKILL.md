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
  domain eventů (→ `/gk-domain-events`), ani DI registrace busů (→ `/gk-di`).

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

**Řetězce middleware** jsou single-sourced v `middleware/base.go` (`BaseChain`, `CommandChain`, `QueryChain`, `SystemChain`) — DI (`app/infrastructure/di/container_provider.go`) je jen volá, a testfx staví bus ze stejného zdroje, takže pořadí nemůže driftovat. `busmw.BaseChain(...)` (`middleware/base.go`) je sdílený základ
**Recovery → Logging → Authorize → Plane → Tenant**:

| Bus | Chain (pořadí) |
|---|---|
| `CommandBus` | Recovery → Logging → Authorize → Plane → Tenant → **Audit → RunDispatcher → DispatchEvents → Transaction** |
| `SystemCommandBus` | Recovery → Logging → SystemPlane → **Audit → RunDispatcher → DispatchEvents → Transaction** (bez Authorize/Tenant, vše v systémové rovině — operator-trusted CLI, tenant injectovaný explicitně; RunDispatcher zůstává, aby i CLI command mohl durably enqueue run) |
| `QueryBus` | Recovery → Logging → Authorize → Plane → Tenant → ReadTx |
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
- **Plane** (`plane.go`) — označí v `ctx` rovinu (`shared.Plane`) podle deklarované
  permission: `platform:*` → platformní, cokoli jiného → tenantová (nulová hodnota,
  fail closed). `SystemPlaneMiddleware` v `SystemChain` dává všem CLI commandům
  systémovou rovinu. Adaptér podle ní otevře transakci: na Postgresu tenantová
  rovina běží jako role `gokick_app` (Row-Level Security), platformní a systémová
  jako `gokick_system` (BYPASSRLS); na SQLite rovina nic nemění.
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
  `shared.Transactor` (duck typing, `sqlite.Manager` ho implementuje). Command
  může opt-outnout přes marker `shared.SkipsTransaction` — ze dvou různých
  důvodů: raw-pool zápisy (Login, jinak SQLite self-deadlock) a cleanup,
  který musí přežít vrácený error (RefreshToken — theft/expiry smaže tokeny
  a vrátí `AuthError`; uvnitř tx by rollback force-logout zrušil).
  **Opakování:** na Postgresu může transakce prohrát souboj se souběžnou
  transakcí (deadlock 40P01, serializační chyba 40001, čekání na zámek delší než
  `lock_timeout` 55P03). Taková transakce se vrátí a celý handler běží znovu
  v nové transakci.
  Pokusy jsou nejvýš tři, mezi nimi je krátká náhodná pauza. Co je taková chyba,
  rozhodne adaptér (`shared.Transactor.IsRetryable`). SQLite zápisy serializuje,
  takže souběh neprohraje nikdy. Každý pokus sbírá eventy a audit záznamy do
  vlastních sběračů a ven se dostanou jen záznamy posledního pokusu: eventy po
  commitu, audit vždy. Opakovaný command proto rozešle eventy a zapíše audit
  jednou. Pevný časový strop opakování nemá: `APP_DB_LOCK_TIMEOUT` omezuje každé
  jednotlivé čekání na zámek, ne celý pokus. Command za dlouhým držitelem zámku
  tak může trvat až třikrát déle. Write timeout HTTP serveru ho neukončí, jen
  zahodí odpověď; kontext requestu běží dál.

`EventBus.Register(name, handler)` se volá **jen při DI wiringu** (single-goroutine
init) — `event.go` čte mapu bez zámku, což je safe jen díky tomu. Dispatch navíc
blokuje kaskádový `Collect` z event handlerů (`ContextWithoutEventCollector`).

## Recipe
### Recipe: dispatch query z HTTP handleru (čtení)
```go
page, err := bus.Query(
    r.Context(),
    h.queryBus,                // *QueryBus — párování hlídá kompilátor
    "ListUsers",               // štítek do logu
    q,                         // query value (musí mít Permissioned/SkipPermission)
    func(ctx context.Context) (user.ListPage, error) {
        return h.listUsers.Handle(ctx, q)
    },
)
if err != nil { h.resp.HandleError(r.Context(), w, err); return }
```
(reálný vzor: `app/presentation/http/handler/admin_users.go:139`)

### Recipe: dispatch command z HTTP handleru (zápis)
```go
err := bus.DispatchVoid(
    r.Context(), h.commandBus, "CreateUser", cmd,
    func(ctx context.Context) error { return h.createUser.Handle(ctx, cmd) },
)
```
(reálný vzor: `app/presentation/http/handler/admin_users.go:205`)

Command, který něco vrací (bulk operace vrací počet dotčených řádků), jde přes
`bus.Dispatch[R]` místo `DispatchVoid` — zbytek chainu je identický (vzor:
`app/presentation/http/handler/admin_users.go:297`).

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
- **Handler musí snést opakování.** Na Postgresu se může spustit víckrát (viz
  Transaction). Zápisy přes `Conn(ctx)` se s pokusem vrátí, ale práce mimo
  transakci (raw pool, volání cizího API) by se zopakovala. Commandy s raw-pool
  zápisy jsou `SkipsTransaction`, a tím i bez opakování. Pomalou nebo externí
  práci dej do runu (`/gk-runs`).
- **`SkipsTransaction` jen výjimečně** — dnes jen dva legitimní důvody:
  raw-pool zápisy (Login, jinak SQLite self-deadlock) a cleanup přeživší
  vrácený error (RefreshToken, force-logout po theft/expiry). Ne jako
  pohodlný útěk z transakce.

## Related
- Skills: `/gk-commands`, `/gk-queries` (psaní handlerů), `/gk-domain-events`
  (domain eventy), `/gk-audit` (audit trail), `/gk-runs` (durable runs),
  `/gk-di` (DI registrace busů)
- Kód: `app/application/bus/` (`bus.go`, `command.go`, `system_command.go`, `query.go`, `event.go`, `dispatch.go`, `exec.go`, `void.go`), `app/application/bus/middleware/` (`base.go`,
  `recovery.go`, `logging.go`, `authorize.go`, `tenant.go`, `audit.go`, `run_dispatcher.go`,
  `events.go`, `transaction.go`), `app/infrastructure/di/container_provider.go`,
  `app/domain/shared/permission.go`
