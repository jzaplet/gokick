---
layout: 'page'
uri: '/framework/infrastructure/wire'
position: 5
slug: 'framework-infrastructure-wire'
parent: 'framework-infrastructure'
navTitle: 'Wire DI'
title: 'Wire DI'
description: 'Balíček infrastructure/di/ – compile-time DI, workflow.'
---

# Wire DI

Balíček `infrastructure/di/`. Compile-time DI – definujeme providers, Wire generuje kód.

```
app/infrastructure/di/
├── container_provider.go   # Definice (build tag: wireinject)
└── wire_gen.go             # Generovaný (neupravovat)
```


## Workflow

1. Přidej provider do `container_provider.go`
2. `make di` → Wire vygeneruje `wire_gen.go`
3. `cmd/main.go` volá `di.CreateApplication()`


## Komponenty

- **Config** – `config.Config`
- **Database** – `SqliteManager`, `MigrationManager`
- **Security** – `JwtService`, `PasswordService` (→ `shared.PasswordHasher`), `PermissionChecker` (→ `shared.PermissionChecker`)
- **Bus** – CommandBus (Authorize + Transaction + DispatchEvents), QueryBus (Authorize), EventBus
- **Repositories** – `SqliteUserRepository`, `SqliteTokenRepository`
- **CQRS** – command/query handlery, event handlery
- **HTTP** – handlery, middleware, server
- **CLI** – root + serve command
