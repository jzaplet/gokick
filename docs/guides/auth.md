---
layout: 'page'
uri: '/guides/auth'
position: 1
slug: 'guides-auth'
parent: 'guides'
navTitle: 'Authentication'
title: 'Authentication'
description: 'JWT access + refresh token, session lifecycle, role & permissions.'
---

# Authentication

Dvoutokenový systém s konfigurovatelnou expirací.


## Access token (JWT)

- HS256, životnost `APP_JWT_ACCESS_EXPIRATION` (default `15m`)
- Přenos: `Authorization: Bearer` hlavička
- Uložení: v paměti (JS proměnná přes `setAccessToken()`)
- Claims: `sub` (user ID), `role`, `nickname`, `exp`, `iat`


## Refresh token

- Náhodný řetězec přes `crypto/rand.Text()` (Go 1.24+)
- Životnost `APP_JWT_REFRESH_EXPIRATION` (default `168h`)
- Přenos: `httpOnly` + `Secure` + `SameSite=Strict` cookie
- Uložení: SHA256 hash v DB se sloupcem `used_at`


## Rotace a theft detection

Při každém refresh se starý token **neodmaže**, jen se označí jako použitý (`used_at = NOW()`). Pokud se ten samý raw token objeví podruhé (`used_at != NULL`), backend to vyhodnotí jako krádež tokenu a zavolá **force logout** — smaže všechny refresh tokeny daného usera. Útočník i legitimní klient jsou okamžitě odhlášeni.

| Situace | Akce |
|---|---|
| Token neznámý | `AuthError` (401) |
| Token použitý (reuse) | **DeleteByUserID** + `AuthError` (theft) |
| Token expirovaný | DeleteByUserID + `AuthError` |
| Token platný | `MarkUsed` + vydat novou dvojici |


## Endpointy

| Metoda | Route | Auth | Popis |
|---|---|---|---|
| POST | `/api/v1/auth/login` | Ne | Přihlášení |
| POST | `/api/v1/auth/refresh` | Cookie | Obnovení tokenu |
| POST | `/api/v1/auth/logout` | Bearer | Odhlášení |

Response: `{ access_token, access_expiration, user: { id, nickname, role, permissions } }`


## Role & Permissions

Backend používá permission stringy (`admin:users:create`, `profile:read`, ...). Každý command/query handler deklaruje svůj požadavek přes `shared.Permissioned` interface.

- **Admin** role má přístup ke všemu
- **User** role má přístup jen k permissions, které nejsou `admin:*`
- Login response vrací `permissions: string[]` — seznam povolených permission stringů pro danou roli

Frontend:

```typescript
const { hasRole, isAdmin, hasPermission, hasAllPermissions, hasAnyPermission } = useAuth();

// Role
hasRole('admin');                                       // true/false
isAdmin();                                              // shortcut pro hasRole('admin')

// Permissions
hasPermission('admin:users:create');                    // admin: vždy true
hasAllPermissions(['profile:read', 'profile:update']);  // všechny musí platit
hasAnyPermission(['admin:users:read', 'admin:users:create']);  // stačí jedna
```

Kompletní přehled všech `useAuth()` metod viz [Frontend Utils – useAuth](/guides/frontend-utils#useauth).


## Session lifecycle

1. **Otevření** → route guard → `refresh()` → tiché přihlášení nebo redirect na login _(plánováno — Task 14)_
2. **Přihlášení** → access token + refresh cookie + `scheduleRefresh()`
3. **Auto-refresh** → 30s před expirací → nový access token + rotace refresh tokenu
4. **401 response** → auto-refresh v useFetch → retry nebo logout _(plánováno — Task 13)_
5. **Odhlášení** → smaže token z DB + cookie + paměť
