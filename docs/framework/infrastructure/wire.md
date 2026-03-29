---
layout: 'page'
uri: '/framework/infrastructure/wire'
position: 4
slug: 'framework-infrastructure-wire'
parent: 'framework-infrastructure'
navTitle: 'Wire DI'
title: 'Wire DI'
description: 'Balíček di_container/ – compile-time DI, workflow.'
---

# Wire DI

Balíček `di_container/`. Compile-time DI – definujeme providers, Wire generuje kód.

```
app/di_container/
├── container_provider.go   # Definice (build tag: wireinject)
└── wire_gen.go             # Generovaný (neupravovat)
```


## Workflow

1. Přidej provider do `container_provider.go`
2. `make di` → Wire vygeneruje `wire_gen.go`
3. `main.go` volá `di_container.CreateApplication()`


## Komponenty

- **Config** – `env.Config`
- **Database** – `SqliteManager`, `MigrationManager`
- **Security** – `JwtService`, `PasswordService` (→ `domain.PasswordHasher`), `PermissionChecker` (→ `domain.PermissionChecker`)
- **Bus** – CommandBus (Authorize + Transaction + DispatchEvents), QueryBus (Authorize), EventBus
- **Repositories** – `SqliteUserRepository`, `SqliteTokenRepository`
- **CQRS** – command/query handlery, event handlery
- **HTTP** – handlery, middleware, server
- **CLI** – root + serve command
