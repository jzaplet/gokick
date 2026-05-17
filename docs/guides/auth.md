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


## Rate limiting

Per-IP token bucket na auth endpointech. Defaultně `10/min` na `/login` a `60/min` na `/refresh`. Při překročení vrátí backend `429 Too Many Requests` s `Retry-After` headerem.

| Env | Default | Význam |
|---|---|---|
| `APP_RATE_LIMIT_LOGIN` | `10/min` | Per-IP bucket pro `/api/v1/auth/login`. Formát: `N/sec`, `N/min`, `N/hour` nebo `N/Xs|Xm|Xh`. Prázdná hodnota = vypnuto. |
| `APP_RATE_LIMIT_REFRESH` | `60/min` | Stejný formát pro `/api/v1/auth/refresh`. |
| `APP_TRUST_PROXY_HEADERS` | `false` | Když `true`, IP se čte z `X-Real-IP` (nutné za reverse proxy, která hlavičku přepisuje). Defaultně `RemoteAddr`. **Pozor:** zapni jen tehdy, pokud proxy hlavičku skutečně přepisuje — jinak ji může spoofnout libovolný klient a obejít limit. |

Buckety jsou per-IP a per-endpoint. Janitor v Go goroutině uklízí idle buckety (≥ 5 min) aby paměť nerostla pod stuffing útokem.


## Brute-force ochrana

Doplněk k rate limitingu — chrání i tehdy, když útočník rotuje přes mnoho IP adres. Po **5 failed login attempts uvnitř 10minutového okna** se účet zamkne na **15 minut**. Locked účet s **správným** heslem vrátí stejnou neutrální chybu `invalid credentials` — response nikde neprozradí, zda byl problém v hesle nebo v locku.

Stav žije na `users` řádku:

| Sloupec | Význam |
|---|---|
| `failed_login_attempts` | Aktuální počítadlo. Reset na 0 po úspěšném loginu **nebo** po dosažení threshold (lock se aktivuje, počítadlo se vynuluje). |
| `last_failed_login_at` | Poslední neúspěch — slouží k window check. |
| `locked_until` | Pokud `!= NULL` a v budoucnosti, accept attempts jsou no-op + audit `auth.login.blocked_while_locked` (počítadlo se nezvyšuje, lock se neprodlužuje). |

Implementační detail: counter update běží **mimo** business transakci (přes raw connection pool), aby přežil rollback způsobený AuthError, který handler na konci vrátí. Jediný SQL `UPDATE ... CASE` rozhoduje atomicky o resetu / inkrementu / locku — žádný read-modify-write race.


## Audit log

Každý security-relevantní krok (úspěšný/neúspěšný login, lock, theft, change password, CRUD na uživatelích) padá do append-only tabulky `audit_log`. Detaily a integration pattern: viz [Audit log](/framework/application/audit).


## Session lifecycle

1. **Otevření / hard refresh stránky** → `assets/app.ts:bootstrap()` zavolá `refresh()` ještě před mountem routeru. Pokud je refresh cookie platná, session se obnoví seamless (nový access token + populace `user` state). Když cookie chybí nebo je neplatná, `refresh()` tiše selže a route guard pošle chráněné routy na `/login`.
2. **Přihlášení** → access token + refresh cookie + `scheduleRefresh()` (timer na auto-refresh).
3. **Auto-refresh** → 30s před expirací → nový access token + rotace refresh tokenu.
4. **401 response** → `authFetch` zavolá single-flight `refresh()` → retry s novým tokenem, jinak vrátí 401 a vyčistí stav.
5. **Odhlášení** → smaže token z DB + cookie + paměť.
