---
layout: 'page'
uri: '/framework/application/bus'
position: 1
slug: 'framework-application-bus'
parent: 'framework-application'
navTitle: 'Bus'
title: 'Bus'
description: 'Balíček application/bus/ -- middleware chain, dispatch, tři instance.'
---

# Bus

## Proč

Každý command/query prochází middleware chainem -- recovery, logging, autorizace, transakce, eventy. Bus odděluje prezentační vrstvu od handlerů a umožňuje transparentně přidat cross-cutting concerns bez změny business logiky.

## Jak

### API

```go
// application/bus/bus.go
type Middleware func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error)

type Bus struct {
    middlewares []Middleware
}

func newBus(middlewares ...Middleware) *Bus  // unexported, používá se přes NewCommandBus/NewQueryBus/NewEventBus
```

```go
// Typově bezpečný dispatch
func Exec[R any](ctx context.Context, b *Bus, name string, cmd any, fn func(ctx context.Context) (R, error)) (R, error)

// Pro commandy bez návratové hodnoty
func ExecVoid(ctx context.Context, b *Bus, name string, cmd any, fn func(ctx context.Context) error) error
```

Parametr `cmd any` umožňuje middleware introspekci -- např. type assert na `shared.Permissioned`.

### Middleware

| Middleware | Soubor | Popis |
|---|---|---|
| Recovery | `recovery.go` | Zachytí panic, zaloguje stack trace |
| Logging | `logging.go` | Název handleru, trvání, trace ID |
| Authorize | `authorize.go` | Type assert na `Permissioned` / `SkipPermission`, volá `PermissionChecker.Check()` |
| Audit | `audit.go` | Drainuje `AuditCollector` po handleru a zapisuje přes `AuditLogger` — **vně** Transaction, takže přežije rollback ([Audit log](/framework/application/audit)) |
| JobDispatcher | `job_dispatcher.go` | Injektuje `JobDispatcher` do `ctx`; enqueue z handleru joinuje business tx |
| DispatchEvents | `events.go` | Vytvoří per-request `EventCollector` v `ctx`, po úspěšném commitu flushne a dispatchne přes `EventBus` |
| Transaction | `transaction.go` | BeginTx / Commit / Rollback přes `shared.Transactor` interface |

### Tři instance (Wire DI)

| Typ | Chain | Použití |
|---|---|---|
| `CommandBus` | Recovery - Logging - Authorize - Audit - JobDispatcher - DispatchEvents - Transaction | Write operace |
| `QueryBus` | Recovery - Logging - Authorize | Read operace |
| `EventBus` | Recovery - Logging | Side-effects po commitu |

Každý bus typ žije ve vlastním souboru (`command.go`, `query.go`, `event.go`):

```go
type CommandBus struct{ *Bus }
type QueryBus struct{ *Bus }
type EventBus struct{ *Bus }
```

Wire je konfiguruje v `container_provider.go` -- každý typ dostane svůj middleware chain.

## Detaily

- **AuthorizeMiddleware** vynucuje, že každý command/query implementuje buď `Permissioned` (deklaruje required permission), nebo `SkipPermission` (explicitní skip). Pokud neimplementuje ani jeden, middleware vrátí error -- chrání proti zapomenuté deklaraci.
- **DispatchEventsMiddleware** sedí **uvnitř** chainu, ale **vně** TransactionMiddleware. Při dispatchi vytvoří per-request `EventCollector` v contextu — handlery ho čtou přes `shared.EventCollectorFromContext(ctx)`. Po návratu z `TransactionMiddleware` (commit OK) flushne a dispatchne eventy přes `EventBus` (synchronně). Pokud commit selže, chyba propaguje, eventy se nedispatchují.
- **TransactionMiddleware** závisí na `shared.Transactor` interface, ne na konkrétní databázi. `SqliteManager` ho implementuje implicitně (duck typing).
- Per-request collector místo sdíleného singletonu eliminuje race condition u paralelních commandů a zaručuje, že každý dispatch dostane vlastní izolovanou sbírku eventů.
