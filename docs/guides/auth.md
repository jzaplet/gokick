---
layout: 'page'
uri: '/guides/auth'
position: 1
slug: 'guides-auth'
parent: 'guides'
navTitle: 'Authentication'
title: 'Authentication'
description: 'JWT access + refresh token, session lifecycle.'
---

# Authentication

Dvoutokenový systém s konfigurovatelnou expirací.


## Access token (JWT)

- HS256, životnost `APP_JWT_ACCESS_EXPIRATION` (default `15m`)
- Přenos: `Authorization: Bearer` hlavička
- Uložení: v paměti (JS proměnná)
- Claims: `sub` (user ID), `role`, `nickname`, `exp`, `iat`


## Refresh token

- Náhodný řetězec přes `crypto/rand.Text()` (Go 1.24+)
- Životnost `APP_JWT_REFRESH_EXPIRATION` (default `168h`)
- Přenos: `httpOnly` + `Secure` + `SameSite=Strict` cookie
- Uložení: SHA256 hash v DB. Rotace při každém refresh.


## Endpointy

| Metoda | Route | Auth |
|---|---|---|
| POST | `/api/v1/auth/login` | Ne |
| POST | `/api/v1/auth/refresh` | Cookie |
| POST | `/api/v1/auth/logout` | Bearer |

Response: `{ access_token, expires_in, user: { id, nickname, role } }`


## Frontend

- **`useAuth()`** – login, logout, refresh, scheduleRefresh, isAuthenticated, currentUser
- **`apiFetch()`** – Authorization header, auto-refresh na 401
- **Route guards** – requiresAuth, requiredRole


## Session lifecycle

1. **Otevření** → route guard → `refresh()` → tiché přihlášení nebo login
2. **Přihlášení** → access token + refresh cookie + scheduleRefresh
3. **Expirace** → 401 → apiFetch refreshne → opakuje request
4. **Odhlášení** → smaže token z DB + cookie + paměť
