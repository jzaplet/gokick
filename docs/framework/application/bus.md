---
layout: 'page'
uri: '/framework/application/bus'
position: 3
slug: 'framework-application-bus'
parent: 'framework-application'
navTitle: 'Bus'
title: 'Bus'
description: 'Balíček bus/ – middleware chain, Exec/ExecVoid, authorize, transaction, events.'
---

# Bus

Balíček `bus/`. Middleware chain kolem command/query handlerů.


## API

```go
// bus/bus.go
type Middleware func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error)

type Bus struct {
    middlewares []Middleware
}

func New(middlewares ...Middleware) *Bus
```

```go
// bus/command.go – typově bezpečný dispatch
func Exec[R any](ctx context.Context, b *Bus, name string, cmd any, fn func(ctx context.Context) (R, error)) (R, error)

// bus/void.go – pro commandy bez návratové hodnoty
func ExecVoid(ctx context.Context, b *Bus, name string, cmd any, fn func(ctx context.Context) error) error
```

`cmd any` parametr umožňuje middleware introspekci (type assert na `domain.Permissioned`).


## Middleware

| Middleware | Soubor | Popis |
|---|---|---|
| Recovery | `middleware_recovery.go` | Zachytí panic, zaloguje |
| Logging | `middleware_logging.go` | Název, trvání, error. `slog.NewMultiHandler()` (Go 1.26) |
| Authorize | `middleware_authorize.go` | `Permissioned` → `PermissionChecker.Check()` |
| Transaction | `middleware_transaction.go` | BEGIN/COMMIT/ROLLBACK (jen CommandBus) |
| DispatchEvents | `middleware_events.go` | Flush EventCollector po commitu (jen CommandBus) |


## AuthorizeMiddleware

Každý command/query MUSÍ implementovat `Permissioned` nebo `SkipPermission`. Pokud neimplementuje ani jeden, middleware vrátí error – chrání proti zapomenutému permission deklaraci.

```go
func AuthorizeMiddleware(checker domain.PermissionChecker) Middleware {
    return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
        switch c := cmd.(type) {
        case domain.Permissioned:
            if err := checker.Check(ctx, c.RequiredPermission()); err != nil {
                return nil, err // → AuthError → 403
            }
        case domain.SkipPermission:
            // Explicitně přeskočeno – OK
        default:
            // Ani Permissioned, ani SkipPermission → programátorská chyba
            return nil, fmt.Errorf("bus: command %q must implement Permissioned or SkipPermission", name)
        }
        return next(ctx)
    }
}
```


## Tři instance (Wire DI)

- **CommandBus** = Recovery → Logging → Authorize → Transaction → DispatchEvents
- **QueryBus** = Recovery → Logging → Authorize
- **EventBus** = Recovery → Logging
