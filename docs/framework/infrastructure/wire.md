---
layout: 'page'
uri: '/framework/infrastructure/wire'
position: 4
slug: 'framework-infrastructure-wire'
parent: 'framework-infrastructure'
navTitle: 'Wire DI'
title: 'Wire DI'
description: 'Balíček infrastructure/di/ -- compile-time DI, workflow.'
---

# Wire DI

## Proč

Google Wire generuje DI kód v compile-time -- žádná reflexe, žádný runtime kontejner. Všechny závislosti jsou explicitně definované v `container_provider.go`, Wire z nich vygeneruje `wire_gen.go`.

## Jak

### Struktura

```
app/infrastructure/di/
├── container_provider.go   # Definice (build tag: wireinject)
└── wire_gen.go             # Generovaný kód (neupravovat)
```

### Workflow

1. Přidej provider do `container_provider.go` (funkce vracející novou závislost)
2. `make di` -- Wire vygeneruje `wire_gen.go`
3. `cmd/main.go` volá `di.CreateApplication(logger)`

### Registrované komponenty

| Kategorie | Providery |
|---|---|
| **Config** | `config.LoadConfig` |
| **Database** | `database.NewSqliteManager`, `database.NewMigrationManager` |
| **Security** | `security.NewJwtService`, `providePasswordHasher` (-> `shared.PasswordHasher`), `providePermissionChecker` (-> `shared.PermissionChecker`) |
| **Bus** | `provideCommandBus`, `provideQueryBus`, `provideEventBus` |
| **Repositories** | `sqliteuser.NewRepository`, `sqlitetoken.NewRepository`, `sqlite.NewSeeder` |
| **HTTP** | `handler.NewHealthHandler`, `server.NewServer` |
| **CLI** | `console.NewServeCommand`, `console.NewRootCommand` |

### Interface binding

Wire nepropojí interface automaticky -- je potřeba explicitní `wire.Bind`:

```go
wire.Bind(new(user.Repository), new(*sqliteuser.Repository))
```

Provider funkce pro doménové interfaces:

```go
func providePasswordHasher() shared.PasswordHasher { return security.NewPasswordHasher() }
func providePermissionChecker() shared.PermissionChecker { return security.NewPermissionChecker() }
```

## Detaily

- `container_provider.go` má build tag `//go:build wireinject` -- kompiluje se jen pro Wire, ne pro produkční build.
- `wire_gen.go` se nikdy neupravuje ručně -- vždy přes `make di`.
- `CreateApplication` přijímá `*slog.Logger` jako externí závislost (vytvořenou v `main.go`).
- Přidání nové služby: vytvoř constructor, přidej ho do `wire.Build(...)`, spusť `make di`.
