---
layout: 'page'
uri: '/framework/application/queries'
position: 3
slug: 'framework-application-queries'
parent: 'framework-application'
navTitle: 'Queries & Events'
title: 'Queries & Event Handlers'
description: 'Balíčky application/query/ a application/event/ -- read operace a domain event handlery.'
---

# Queries & Event Handlers

## Proč

Queries čtou stav systému bez jeho změny. Event handlery reagují na domain eventy (side-effects po úspěšném commitu). Obojí závisí pouze na `domain/`.

## Jak

### Query

Stejná struktura jako command: `XxxQuery` (filtry) + `XxxHandler` (logika). Query prochází `QueryBus` (Recovery - Logging - Authorize).

```go
// application/query/query_list_users.go

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

Pokud command/query neimplementuje ani `Permissioned`, ani `SkipPermission`, `AuthorizeMiddleware` vrátí error.

### Event handler

Event handlery zpracovávají domain eventy dispatched přes `EventBus` (Recovery - Logging). Registrují se v DI kontejneru.

```go
// application/event/event_send_welcome_email.go

type SendWelcomeEmailHandler struct {
    mailer Mailer
}

func (h *SendWelcomeEmailHandler) Handle(ctx context.Context, event user.UserCreated) error {
    return h.mailer.Send(event.Email, /* ... */)
}
```

### Sběr eventů v command handleru

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    // ... business logika ...
    u := user.NewUser(nickname, hash, cmd.Email, role)

    h.events.Collect(user.UserCreated{
        UserID: u.ID, Nickname: u.Nickname, Email: u.Email,
    })

    return h.repo.Save(ctx, u)
}
```

## Detaily

- Query handler nemá side-effects -- jen čte data.
- Eventy se dispatchují až **po úspěšném commitu** transakce (`DispatchEventsMiddleware`). Při rollbacku se zahodí.
- Event handler může selhat bez vlivu na původní command -- chyba se zaloguje, ale command už commitnul.
