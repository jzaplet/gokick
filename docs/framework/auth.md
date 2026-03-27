---
layout: 'page'
uri: '/framework/auth'
position: 8
slug: 'framework-auth'
parent: 'framework'
navTitle: 'Autentizace'
title: 'Autentizace'
description: 'JWT autentizace – access + refresh token, session lifecycle, frontend integrace.'
---

# Autentizace

Dvoutokenový systém s konfigurovatelnou expirací.

```
Login → access_token (body) + refresh_token (httpOnly cookie)
  ↓
API call → Authorization: Bearer <access_token>
  ↓
401 Expired → POST /api/v1/auth/refresh (cookie automaticky)
  ↓
Nový access_token + nový refresh_token (rotace)
```


## Access token (JWT)

- **Typ**: JWT, HS256
- **Životnost**: konfigurovatelná (`APP_JWT_ACCESS_EXPIRATION`, default `15m`)
- **Přenos**: `Authorization: Bearer` hlavička
- **Uložení na klientu**: v paměti (JS proměnná)

**Claims:**

```json
{
  "sub": "uuid-uživatele",
  "role": "admin",
  "nickname": "Přezdívka",
  "exp": 1234567890,
  "iat": 1234567890
}
```


## Refresh token

- **Typ**: náhodný řetězec generovaný přes `crypto/rand.Text()` (Go 1.24+)
- **Životnost**: konfigurovatelná (`APP_JWT_REFRESH_EXPIRATION`, default `168h`)
- **Přenos**: `httpOnly` + `Secure` + `SameSite=Strict` cookie
- **Uložení na serveru**: SHA256 hash v tabulce `refresh_tokens`
- **Rotace**: při každém refresh se starý token smaže a vydá nový


## Auth endpointy

| Metoda | Route | Popis | Auth |
|---|---|---|---|
| POST | `/api/v1/auth/login` | Přihlášení | Ne |
| POST | `/api/v1/auth/refresh` | Obnovení tokenu | Cookie |
| POST | `/api/v1/auth/logout` | Odhlášení | Bearer |

**Login/Refresh response:**

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 900,
  "user": { "id": "uuid", "nickname": "Přezdívka", "role": "admin" }
}
```


## CQRS commands

```go
// command/login.go
type LoginCommand struct { UserID, Password string }
// → Ověří heslo, vygeneruje tokeny, uloží refresh hash do DB.

// command/refresh_token.go
type RefreshTokenCommand struct { RawRefreshToken string }
// → Validuje refresh token, rotuje, vydá nový access token.

// command/logout.go
type LogoutCommand struct { UserID string }
// → Smaže všechny refresh tokeny uživatele z DB.
```


## Frontend – useAuth composable

```typescript
// assets/vue/composables/useAuth.ts
const accessToken = ref<string | null>(null)
const currentUser = ref<AuthUser | null>(null)
const isAuthenticated = computed(() => !!accessToken.value)

async function login(userId: string, password: string): Promise<void>
async function refresh(): Promise<boolean>
async function logout(): Promise<void>
function scheduleRefresh(expiresIn: number): void
```


## Frontend – API klient

```typescript
// assets/vue/services/api.ts
async function apiFetch(url: string, options?: RequestInit): Promise<Response> {
    // Přidá Authorization header
    // Při 401 automaticky zavolá refresh()
    // Při selhání refresh → redirect na /login
}
```


## Frontend – route guards

```typescript
router.beforeEach(async (to) => {
    if (to.meta.requiresAuth && !auth.isAuthenticated.value) {
        const refreshed = await auth.refresh()
        if (!refreshed) return { path: '/login' }
    }
    if (to.meta.requiredRole && auth.currentUser.value?.role !== to.meta.requiredRole) {
        return { path: '/' }
    }
})
```


## Životní cyklus session

1. **Otevření aplikace** → route guard → `refresh()` → tiché přihlášení nebo redirect na login
2. **Přihlášení** → access token v paměti + refresh cookie + `scheduleRefresh`
3. **Access token expiruje** → 401 → `apiFetch` automaticky refreshne → opakuje request
4. **Refresh token expiruje** → `refresh()` selže → redirect na login
5. **Odhlášení** → smaže refresh token z DB + cookie + paměť
