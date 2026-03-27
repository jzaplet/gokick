---
layout: 'page'
uri: '/framework/wire'
position: 7
slug: 'framework-wire'
parent: 'framework'
navTitle: 'Wire DI'
title: 'Wire DI'
description: 'Dependency Injection přes Google Wire – compile-time generování, workflow.'
---

# Wire DI

Compile-time DI framework. Definujeme provider funkce a Wire generuje inicializační kód.

```
app/di_container/
├── container_provider.go   # Definice injektoru (build tag: wireinject)
└── wire_gen.go             # Generovaný kód (neupravovat)
```


## Workflow

1. Přidej nový provider (konstruktor) do `container_provider.go`
2. Spusť `make di` → Wire vygeneruje `wire_gen.go`
3. Hlavní vstup v `main.go` volá `di_container.CreateApplication()`


## Klíčové komponenty

- **Config** – načtení `.env`, validace, konfigurace JWT expirací
- **Database** – `SqliteManager`, `MigrationManager`
- **Security** – `JwtService`, `PasswordService`
- **Bus** – `CommandBus` (s TransactionMiddleware), `QueryBus` (bez transakce). Pro typové rozlišení lze použít generické type aliasy (Go 1.24+), např. `type CommandBus = Bus[Command]`
- **Repositories** – `SqliteUserRepository`, `SqliteTokenRepository`
- **Commands/Queries** – všechny CQRS handlery
- **Handlers** – všechny HTTP handlery
- **Server** – HTTP server, middleware stack
- **CLI** – root command + serve command
