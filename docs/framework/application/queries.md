---
layout: 'page'
uri: '/framework/application/queries'
position: 2
slug: 'framework-application-queries'
parent: 'framework-application'
navTitle: 'Queries'
title: 'Queries'
description: 'Balíček application/query/ – read operace, permission deklarace.'
---

# Queries

Balíček `application/query/`. Read operace – čtou stav systému, nemění ho. Závisí jen na `domain/`.


## Struktura

Stejná jako commands: `XxxQuery` (filtry) + `XxxHandler` (logika).


## Příklad

```go
// application/query/query_list_users.go

type ListUsersQuery struct{}

func (q ListUsersQuery) RequiredPermission() string { return "admin.users.read" }

type ListUsersHandler struct {
    repo user.Repository
}

func (h *ListUsersHandler) Handle(ctx context.Context, q ListUsersQuery) ([]user.User, error) {
    return h.repo.FindAll(ctx)
}
```


## Bez permission (veřejné)

Veřejné queries implementují `SkipPermission` – explicitní deklarace, že permission check není potřeba:

```go
// application/query/query_get_public_info.go – veřejný endpoint

type GetPublicInfoQuery struct{}

func (q GetPublicInfoQuery) SkipPermissionCheck() {}  // explicitní skip

type GetPublicInfoHandler struct {
    // ...
}

func (h *GetPublicInfoHandler) Handle(ctx context.Context, q GetPublicInfoQuery) (*PublicInfo, error) {
    // ...
}
```

Pokud command/query neimplementuje ani `Permissioned`, ani `SkipPermission`, bus middleware vrátí error – chrání proti zapomenutému permission deklaraci.
