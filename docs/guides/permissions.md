---
layout: 'page'
uri: '/guides/permissions'
position: 2
slug: 'guides-permissions'
parent: 'guides'
navTitle: 'Permissions'
title: 'Permissions'
description: 'Jak fungují role a permissions. Deklarace v handlerech, kontrola přes bus, seznam pro frontend.'
---

# Permissions

Permissions se nikde neudržují v listu — vznikají **automaticky** z command/query handlerů. Každý handler deklaruje svou permission a tím se sám registruje.


## Princip

1. Každý command/query handler implementuje buď `shared.Permissioned` (vyžaduje permission) nebo `shared.SkipPermission` (explicitní opt-out).
2. `PermissionsRegistry` posbírá permissions ze všech handlerů při startu.
3. `PermissionChecker` ji používá v bus middleware — zablokuje request, pokud role usera nesedí.
4. Login/profile response obsahuje `permissions: []string` — frontend podle nich skryje UI.


## Formát permission

```
<doména>:<akce>         např. profile:read, auth:logout, admin:users:create
```

**Pravidlo:** role `admin` má přístup ke všemu. Ostatní role mají **všechno kromě `admin:*`**.


## Deklarace v handleru

### Command s permission

```go
type DeleteUserCommand struct {
    UserID string
}

func (DeleteUserCommand) RequiredPermission() string { return "admin:users:delete" }
```

### Command bez permission (veřejný)

```go
type LoginCommand struct {
    Nickname string
    Password string
}

func (LoginCommand) SkipPermissionCheck() {}
```

Každý command/query **MUSÍ** implementovat jedno z těch dvou. Jinak `AuthorizeMiddleware` vrátí error — ochrana proti zapomenuté deklaraci.


## Kontrola (backend)

Middleware `AuthorizeMiddleware` se spouští automaticky v command/query bus. Pro každý dispatch:

1. Pokud command implementuje `SkipPermission` → propustí.
2. Pokud implementuje `Permissioned` → zavolá `PermissionChecker.Check(ctx, permission)`.
3. Checker přečte `AuthClaims` z kontextu a volá `shared.IsPermissionAllowedForRole(permission, role)`.
4. Když role nesedí → `*shared.PermissionError` (HTTP 403). Když nejsou claims → `*shared.AuthError` (HTTP 401).


## Seznam pro frontend

Login a `/profile` response obsahují seznam permissions pro konkrétní roli:

```json
{
    "access_token": "...",
    "access_expiration": 900,
    "user": {
        "id": "...",
        "nickname": "alice",
        "role": "user",
        "permissions": ["auth:logout", "profile:read", "profile:update"]
    }
}
```

Tento seznam sestavuje `PermissionsRegistry`:

```go
reg := shared.NewPermissionsRegistry([]shared.Permissioned{
    authcmd.LogoutCommand{},
    profilecmd.ChangePasswordCommand{},
    profileqry.GetProfileQuery{},
    usercmd.CreateUserCommand{},
    usercmd.UpdateUserCommand{},
    usercmd.DeleteUserCommand{},
    userqry.ListUsersQuery{},
    dashboardqry.GetUserDashboardQuery{},
    dashboardqry.GetAdminDashboardQuery{},
})

reg.ForRole("admin")  // vše
reg.ForRole("user")   // bez admin:*
```

Wire registruje všechny handlery ve `container_provider.go` (provider `providePermissionsRegistry`) — kdykoliv přidáš nový `Permissioned` handler, přidáš ho i do tohoto seznamu. Žádná druhá konfigurace neexistuje — permission registry je jediný zdroj pravdy a derived přímo z kódu.


## Použití ve frontendu

```typescript
const { hasPermission, hasAnyPermission, isAdmin } = useAuth();

hasPermission('admin:users:create')
hasAnyPermission(['profile:read', 'profile:update'])
isAdmin()
```

Detaily [useAuth API](/guides/frontend-utils#useauth).


## Přidání nové permission

1. Vytvoř command/query s `RequiredPermission()` vracející novou hodnotu (např. `"reports:export"`).
2. V `container_provider.go` přidej instanci commandu do slice pro `NewPermissionsRegistry`.
3. `make di && make test` — registry o ní ví, handler je chráněný, frontend ji dostane v login response.

Žádný seznam permissions k ručnímu udržování neexistuje — tím je zajištěno, že **permission == existující handler**.
