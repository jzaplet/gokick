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
| **Config** | `config.LoadConfig`, `provideCookieSecure` (-> `handler.CookieSecure`) |
| **Database** | `database.NewSqliteManager`, `database.NewMigrationManager` |
| **Scheduler** | `provideScheduler` (-> `*scheduler.Scheduler`, registruje cleanup jobs) |
| **Job Queue** | `provideJobHandlerRegistry`, `provideJobDispatcher` (-> `shared.JobDispatcher`), `provideWorker` (-> `*worker.Worker`) |
| **Security** | `security.NewJwtService`, `providePasswordHasher` (-> `shared.PasswordHasher`), `providePermissionChecker` (-> `shared.PermissionChecker`), `providePermissionsRegistry` (-> `*shared.PermissionsRegistry`) |
| **Bus** | `provideCommandBus`, `provideQueryBus`, `provideEventBus` |
| **Repositories** | `sqliteuser.NewRepository`, `sqlitetoken.NewRepository`, `sqlitejob.NewRepository`, `sqlite.NewSeeder` |
| **Auth handlery** | `authcmd.NewLoginHandler`, `authcmd.NewRefreshTokenHandler`, `authcmd.NewLogoutHandler` |
| **Profile handlery** | `profilecmd.NewChangePasswordHandler`, `profileqry.NewGetProfileHandler` |
| **User handlery (admin CRUD)** | `usercmd.NewCreateUserHandler`, `usercmd.NewUpdateUserHandler`, `usercmd.NewDeleteUserHandler`, `userqry.NewListUsersHandler` |
| **Dashboard handlery** | `dashboardqry.NewGetUserDashboardHandler`, `dashboardqry.NewGetAdminDashboardHandler` |
| **HTTP** | `handler.NewHealthHandler`, `handler.NewSPAHandler`, `handler.NewAuthHandler`, `handler.NewProfileHandler`, `handler.NewAdminUsersHandler`, `handler.NewDashboardHandler`, `server.NewServer`, `providePublicFS` |
| **CLI** | `console.NewServeCommand`, `console.NewSeedCommand`, `console.NewCreateUserCommand`, `console.NewRootCommand` |

### Interface binding

Wire nepropojí interface automaticky -- je potřeba explicitní `wire.Bind`:

```go
wire.Bind(new(shared.JwtService), new(*security.JwtService))
wire.Bind(new(user.Repository), new(*sqliteuser.Repository))
wire.Bind(new(token.TokenRepository), new(*sqlitetoken.Repository))
wire.Bind(new(shared.Seeder), new(*sqlite.Seeder))
```

Provider funkce pro doménové interfaces (kde constructor vrací konkrétní typ):

```go
func providePasswordHasher() shared.PasswordHasher { return security.NewPasswordHasher() }
func providePermissionChecker() shared.PermissionChecker { return security.NewPermissionChecker() }
```

## Detaily

- `container_provider.go` má build tag `//go:build wireinject` -- kompiluje se jen pro Wire, ne pro produkční build.
- `wire_gen.go` se nikdy neupravuje ručně -- vždy přes `make di`.
- `CreateApplication` přijímá `*slog.Logger` jako externí závislost (vytvořenou v `main.go`).
- Přidání nové služby: vytvoř constructor, přidej ho do `wire.Build(...)`, spusť `make di`.
