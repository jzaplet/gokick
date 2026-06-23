---
layout: 'page'
uri: '/framework/architecture'
position: 30
slug: 'framework-architecture'
parent: 'framework'
navTitle: 'Architecture'
title: 'Architecture'
description: 'DDD vrstvy s CQRS, pravidla závislostí, lifecycle, cross-domain izolace.'
---

# Architecture

Architektura stojí na DDD s CQRS a bus patternem: čtyři vrstvy s přísnými pravidly závislostí. Komunikace přes CommandBus/QueryBus/EventBus drží vrstvy volně provázané (loose coupling) — command handlery neznají HTTP ani databázi.

> Tahle stránka je **mentální model**. Detailní návody („jak přidat repo / command / handler / job …") jsou ve skillech `/gk-*` — napiš `/gk` pro přehled, nebo rovnou `/gk-architecture`, `/gk-feature`.

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
    -> database.NewMigrationManager()      Vytvoření migration manageru
    -> bus.NewCommandBus/NewQueryBus/NewEventBus  CQRS busy s middleware chain
    -> server.New(handlers, middlewares)    HTTP server
    -> console.NewRootCommand()            Cobra CLI
  -> application.Run(ctx)
    -> database.MigrationManager.RunUp()   Automatické migrace
    -> rootCmd.Execute(ctx)                Cobra parsuje "serve" (ExecuteContext)
      -> server.Start(cmd.Context())       Naslouchá na portu, drainuje při ctx.Done()
```


## Toky (flows)

Tahle stránka je mentální model. Konkrétní cesta requestu napříč vrstvami — middleware chain, transakce, autorizace, mapování chyb — žije na samostatných stránkách:

- [Request flow](/framework/request-flow) — společný HTTP middleware chain a kudy request vstupuje do busu.
- [Command flow](/framework/command-flow) — write operace: Recovery → Logging → Authorize → Tenant → Audit → JobDispatcher → DispatchEvents → Transaction, commit a rozeslání eventů.
- [Query flow](/framework/query-flow) — read operace: Recovery → Logging → Authorize → Tenant, typovaný návrat přes `bus.Exec`.
- [Event flow](/framework/event-flow) — domain eventy po commitu: per-request collector, synchronní dispatch přes EventBus.


## Pravidla a rozšiřování

### go-arch-lint konfigurace

Soubor `.go-arch-lint.yml` v kořeni projektu. Spuštění:

```bash
make arch-check    # go-arch-lint se instaluje automaticky přes make install
```

Hlavní body konfigurace:
- `workdir: app` — všechny cesty relativně k `app/`
- `commonComponents: [domain_shared]` — všem je dostupné pouze `domain/shared/` (sdílené typy a porty); bounded kontexty (`domain_user`, `domain_token`, …) common **nejsou**
- `exclude: [infrastructure/di/**]` — DI balíček nemá omezení
- `excludeFiles: [infrastructure/database/migration_manager.go]` — lifecycle soubor mimo kontrolu
- Každá komponenta má `mayDependOn` seznam povolených závislostí

### Cross-domain izolace

Každý doménový kontext (`domain/user/`, `domain/token/`, ...) je izolovaný balíček. `domain/shared/` obsahuje sdílené typy (errors, interfaces, auth context). Pravidla:

- **Bounded context nesmí importovat jiný bounded context.** `domain/user/` nesmí importovat `domain/token/` a naopak.
- Komunikace mezi kontexty: **QueryBus** (synchronní) nebo **Domain Events** (asynchronní).
- Eventy používají jen primitivy (string ID, ne celé entity).
- go-arch-lint zachytí cross-domain import při `make arch-check`.

Nový kontext (např. `domain/order/`) **vyžaduje** vlastní komponentu v `.go-arch-lint.yml` — právě to, že každý kontext je samostatná komponenta (a `domain/**` není jeden společný wildcard), je důvod, proč go-arch-lint cross-context import vůbec zachytí. Cenou za tu izolaci je explicitní zápis: přidat komponentu `domain_order`, povolit ji v `mayDependOn` u každého konzumenta (`application`, `sqlite_repos`, `testfx`, …) a přidat `infrastructure/sqlite/order/**` do `sqlite_repos`. Naproti tomu broad-glob komponenty (`application/**`, `presentation/http/handler/**`) nové podbalíčky pokrývají automaticky.

### Přidání nové feature (checklist)

1. `domain/` — entity, value objects, interfaces
2. `infrastructure/sqlite/` — repository implementace
3. `application/<domain>/command/` nebo `application/<domain>/query/` — CQRS handler s `Permissioned` nebo `SkipPermission`
4. `presentation/http/handler/` — HTTP handler přes bus
5. `presentation/http/server/` — registrace route
6. `infrastructure/di/` — Wire provider
7. `make di && make arch-check`

Krok za krokem (s permissions, route, DI bindingem) vede skill `/gk-feature`; pravidla vrstev a importů rozebírá `/gk-architecture`.
