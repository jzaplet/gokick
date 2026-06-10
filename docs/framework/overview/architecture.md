---
layout: 'page'
uri: '/framework/overview/architecture'
position: 3
slug: 'framework-overview-architecture'
parent: 'framework-overview'
navTitle: 'Architecture'
title: 'Architecture'
description: 'DDD vrstvy s CQRS, pravidla závislostí, lifecycle, cross-domain izolace.'
---

# Architecture

DDD s CQRS a bus pattern. Čtyři vrstvy s přísnými pravidly závislostí. Komunikace přes CommandBus/QueryBus/EventBus zajišťuje loose coupling -- command handlery neznají HTTP, handlery neznají databázi.

## Čtyři vrstvy

| Vrstva | Složka | Balíčky | Popis |
|---|---|---|---|
| **Domain** | `domain/` | `shared/`, `user/`, `token/` | Entity, value objects, interfaces, errors, events. Žádné závislosti. |
| **Application** | `application/` | `bus/`, `<domain>/command/`, `<domain>/query/`, `<domain>/event/` | CQRS handlery organizované po doménách, bus middleware. Závisí jen na domain. |
| **Infrastructure** | `infrastructure/` | `config/`, `database/`, `sqlite/`, `security/`, `di/` | Implementace domain interfaces, databáze, security. |
| **Presentation** | `presentation/` | `http/handler/`, `http/middleware/`, `http/response/`, `http/server/`, `console/` | HTTP a CLI vrstva. |

```
presentation --> application --> domain <-- infrastructure
     |                                        ^
     +----------------------------------------+
```



## Startup sequence

```
cmd/main.go
  -> signal.NotifyContext(SIGINT, SIGTERM)  Root ctx s signal handlingem
  -> di.CreateApplication()                Wire DI vytvoří vše
    -> config.LoadConfig()                 Načtení .env
    -> database.NewSqliteManager()         Připojení k SQLite
    -> database.MigrationManager.RunUp()   Automatické migrace
    -> bus.NewCommandBus/NewQueryBus/NewEventBus  CQRS busy s middleware chain
    -> server.New(handlers, middlewares)    HTTP server
    -> console.NewRootCommand()            Cobra CLI
  -> application.Run(ctx)
    -> rootCmd.Execute(ctx)                Cobra parsuje "serve" (ExecuteContext)
      -> server.Start(cmd.Context())       Naslouchá na portu, drainuje při ctx.Done()
```


## Request flow (command)

`POST /api/v1/admin/users` -- vytvoření uživatele:

```
1. HTTP Request -> net/http ServeMux

2. HTTP Middleware (presentation/http/middleware/):
   Trace -> Security headers -> CORS -> CSRF -> Logging -> JWT Auth (claims do context)

3. HTTP Handler (presentation/http/handler/):
   json.Decode -> CreateUserCommand
   bus.ExecVoid(ctx, commandBus, "CreateUser", cmd, fn)

4. Bus Middleware (application/bus/middleware/):
   Recovery -> Logging -> Authorize -> DispatchEvents -> Transaction
   |
   |- Authorize: cmd.(Permissioned) -> PermissionChecker.Check()
   |- DispatchEvents: vytvoří per-request EventCollector v ctx
   |- Transaction: BEGIN
   +-> handler:

5. Command Handler (application/user/command/):
   NewNickname() -> NewRole() -> repo.FindByNickname() -> password.Hash()
   -> NewUser() -> repo.Save()
   -> shared.EventCollectorFromContext(ctx).Collect(UserCreated{...})

6. Bus post-handler:
   Transaction -> COMMIT (nebo ROLLBACK při chybě)
   DispatchEvents -> pokud commit OK, flush EventCollector -> EventBus.Dispatch (synchronně)

7. HTTP Handler: response.JSON(w, 201, nil)
```


## Request flow (query)

`GET /api/v1/admin/users`:

```
HTTP Request -> Trace -> Security headers -> CORS -> CSRF -> Logging -> JWT Auth
  -> Handler -> bus.Exec[[]user.User](ctx, queryBus, "ListUsers", q, fn)
    -> Recovery -> Logging -> Authorize -> Query Handler -> repo.FindAll()
  -> response.JSON(w, 200, users)
```


## Error flow

```
Command Handler vrátí error
  |
Bus: Transaction -> ROLLBACK -> err propaguje skrz DispatchEvents (eventy zahozeny)
  |
HTTP Handler: response.HandleError(w, err)
  -> ValidationError  -> 400
  -> AuthError        -> 401
  -> PermissionError  -> 403
  -> jiný error       -> 500
```


## Detaily

### go-arch-lint konfigurace

Soubor `.go-arch-lint.yml` v kořeni projektu. Spuštění:

```bash
make arch-check    # go-arch-lint se instaluje automaticky přes make install
```

Hlavní body konfigurace:
- `workdir: app` -- všechny cesty relativně k `app/`
- `commonComponents: [domain_shared]` -- pouze `domain/shared/` (sdílené typy a porty) je dostupná všem; bounded kontexty (`domain_user`, `domain_token`, ...) common **nejsou**
- `exclude: [infrastructure/di/**]` -- DI balíček nemá omezení
- `excludeFiles: [infrastructure/database/migration_manager.go]` -- lifecycle soubor mimo kontrolu
- Každá komponenta má `mayDependOn` seznam povolených závislostí

### Cross-domain izolace

Každý doménový kontext (`domain/user/`, `domain/token/`, ...) je izolovaný balíček. `domain/shared/` obsahuje sdílené typy (errors, interfaces, auth context). Pravidla:

- **Bounded context nesmí importovat jiný bounded context.** `domain/user/` nesmí importovat `domain/token/` a naopak.
- Komunikace mezi kontexty: **QueryBus** (synchronní) nebo **Domain Events** (asynchronní).
- Eventy používají jen primitivy (string ID, ne celé entity).
- go-arch-lint zachytí cross-domain import při `make arch-check`.

Nový kontext (např. `domain/order/`) **vyžaduje** vlastní komponentu v `.go-arch-lint.yml` -- právě to, že každý kontext je samostatná komponenta (a `domain/**` není jeden společný wildcard), je důvod, proč go-arch-lint cross-context import vůbec zachytí. Cenou za tu izolaci je explicitní zápis: přidat komponentu `domain_order`, povolit ji v `mayDependOn` u každého konzumenta (`application`, `sqlite_repos`, `testfx`, ...) a přidat `infrastructure/sqlite/order/**` do `sqlite_repos`. Naproti tomu broad-glob komponenty (`application/**`, `presentation/http/handler/**`) nové subbalíčky pokrývají automaticky.

### Přidání nové feature (checklist)

1. `domain/` -- entity, value objects, interfaces
2. `infrastructure/sqlite/` -- repository implementace
3. `application/<domain>/command/` nebo `application/<domain>/query/` -- CQRS handler s `Permissioned` nebo `SkipPermission`
4. `presentation/http/handler/` -- HTTP handler přes bus
5. `presentation/http/server/` -- registrace route
6. `infrastructure/di/` -- Wire provider
7. `make di && make arch-check`
