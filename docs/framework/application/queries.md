---
layout: 'page'
uri: '/framework/application/queries'
position: 3
slug: 'framework-application-queries'
parent: 'framework-application'
navTitle: 'Queries'
title: 'Queries'
description: 'Balíček application/query/ -- CQRS read operace.'
---

# Queries


## Proč

Queries čtou stav systému bez jeho změny. Oddělení od commands umožňuje nezávislou optimalizaci čtení (jiné modely, cache, projekce). Query závisí pouze na `domain/`.


## Jak

Stejná struktura jako command: `XxxQuery` (filtry) + `XxxHandler` (logika). Query prochází `QueryBus` (Recovery → Logging → Authorize).

```go
// application/user/query/list_users.go

type ListUsersQuery struct{}

func (q ListUsersQuery) RequiredPermission() string { return "admin:users:read" }

type ListUsersHandler struct {
    repo user.Repository
}

func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) ([]user.User, error) {
    return h.repo.FindAll(ctx)
}
```

### Veřejné query (bez permission)

Veřejná query implementují `SkipPermission` -- explicitní deklarace, že permission check není potřeba:

```go
type GetPublicInfoQuery struct{}

func (q GetPublicInfoQuery) SkipPermissionCheck() {}  // explicitní skip
```

Pokud query neimplementuje ani `Permissioned`, ani `SkipPermission`, `AuthorizeMiddleware` vrátí error.


## Detaily

- Query handler nemá side-effects -- jen čte data.
- Query může vracet libovolný typ (entitu, slice, DTO).
