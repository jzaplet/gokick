---
layout: 'page'
uri: '/framework/overview/architecture'
position: 2
slug: 'framework-overview-architecture'
parent: 'framework-overview'
navTitle: 'Architecture'
title: 'Architecture'
description: 'DDD vrstvy s CQRS, pravidla závislostí, lifecycle, cross-domain izolace.'
---

# Architecture


## Proč

DDD s CQRS a bus pattern. Čtyři vrstvy s přísnými pravidly závislostí. Komunikace přes CommandBus/QueryBus/EventBus zajišťuje loose coupling -- command handlery neznají HTTP, handlery neznají databázi.


## Jak

### Čtyři vrstvy

| Vrstva | Složka | Balíčky | Popis |
|---|---|---|---|
| **Domain** | `domain/` | `shared/`, `user/`, `token/` | Entity, value objects, interfaces, errors, events. Žádné závislosti. |
| **Application** | `application/` | `bus/`, `command/`, `query/`, `event/` | CQRS handlery, bus middleware. Závisí jen na domain. |
| **Infrastructure** | `infrastructure/` | `config/`, `database/`, `sqlite/`, `security/`, `di/` | Implementace domain interfaces, databáze, security. |
| **Presentation** | `presentation/` | `http/handler/`, `http/middleware/`, `http/response/`, `http/server/`, `console/` | HTTP a CLI vrstva. |

```
presentation --> application --> domain <-- infrastructure
     |                                        ^
     +----------------------------------------+
```



### Startup sequence

```
cmd/main.go
  -> di.CreateApplication()                Wire DI vytvoří vše
    -> config.LoadConfig()                 Načtení .env
    -> database.NewSqliteManager()         Připojení k SQLite
    -> database.MigrationManager.RunUp()   Automatické migrace
    -> bus.NewCommandBus/NewQueryBus/NewEventBus  CQRS busy s middleware chain
    -> server.New(handlers, middlewares)    HTTP server
    -> console.NewRootCommand()            Cobra CLI
  -> application.Run()
    -> rootCmd.Execute()                   Cobra parsuje "serve"
      -> server.Start()                   Naslouchá na portu
```


### Request flow (command)

`POST /api/v1/admin/users` -- vytvoření uživatele:

```
1. HTTP Request -> net/http ServeMux

2. HTTP Middleware (presentation/http/middleware/):
   Trace -> CORS -> CSRF -> Logging -> JWT Auth (claims do context)

3. HTTP Handler (presentation/http/handler/):
   json.Decode -> CreateUserCommand
   bus.ExecVoid(ctx, commandBus, "CreateUser", cmd, fn)

4. Bus Middleware (application/bus/middleware/):
   Recovery -> Logging -> Authorize -> Transaction -> DispatchEvents
   |
   |- Authorize: cmd.(Permissioned) -> PermissionChecker.Check()
   |- Transaction: BEGIN
   +-> handler:

5. Command Handler (application/command/):
   NewNickname() -> NewRole() -> repo.FindByNickname() -> password.Hash()
   -> NewUser() -> repo.Save()

6. Bus post-handler:
   Transaction -> COMMIT
   DispatchEvents -> flush EventCollector -> async goroutiny

7. HTTP Handler: response.JSON(w, 201, nil)
```


### Request flow (query)

`GET /api/v1/admin/users`:

```
HTTP Request -> Trace -> CORS -> CSRF -> Logging -> JWT Auth
  -> Handler -> bus.Exec[[]user.User](ctx, queryBus, "ListUsers", q, fn)
    -> Recovery -> Logging -> Authorize -> Query Handler -> repo.FindAll()
  -> response.JSON(w, 200, users)
```


### Error flow

```
Command Handler vrátí error
  |
Bus: Transaction -> ROLLBACK, DispatchEvents -> eventy zahozeny
  |
HTTP Handler: response.HandleError(w, err)
  -> ValidationError -> 400
  -> AuthError -> 403
  -> jiný error -> 500
```


## Detaily

### go-arch-lint konfigurace

Soubor `.go-arch-lint.yml` v kořeni projektu. Spuštění:

```bash
make arch-check    # go-arch-lint se instaluje automaticky přes make install
```

Hlavní body konfigurace:
- `workdir: app` -- všechny cesty relativně k `app/`
- `commonComponents: [domain]` -- domain je automaticky dostupná všem
- `exclude: [infrastructure/di/**]` -- DI balíček nemá omezení
- `excludeFiles: [infrastructure/database/migration_manager.go]` -- lifecycle soubor mimo kontrolu
- Každá komponenta má `mayDependOn` seznam povolených závislostí

### Cross-domain izolace

Každý doménový kontext (`domain/user/`, `domain/token/`, ...) je izolovaný balíček. `domain/shared/` obsahuje sdílené typy (errors, interfaces, auth context). Pravidla:

- **Bounded context nesmí importovat jiný bounded context.** `domain/user/` nesmí importovat `domain/token/` a naopak.
- Komunikace mezi kontexty: **QueryBus** (synchronní) nebo **Domain Events** (asynchronní).
- Eventy používají jen primitivy (string ID, ne celé entity).
- go-arch-lint zachytí cross-domain import při `make arch-check`.

Nový kontext (např. `domain/order/`) nevyžaduje žádnou změnu v `.go-arch-lint.yml` -- wildcard `domain/**` pokrývá všechny subbalíčky automaticky. Totéž platí pro `application/command/**`, `infrastructure/sqlite/**` atd.

### Přidání nové feature (checklist)

1. `domain/` -- entity, value objects, interfaces
2. `infrastructure/sqlite/` -- repository implementace
3. `application/command/` nebo `application/query/` -- CQRS handler s `Permissioned` nebo `SkipPermission`
4. `presentation/http/handler/` -- HTTP handler přes bus
5. `presentation/http/server/` -- registrace route
6. `infrastructure/di/` -- Wire provider
7. `make di && make arch-check`
