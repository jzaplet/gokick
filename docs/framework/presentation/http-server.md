---
layout: 'page'
uri: '/framework/presentation/http-server'
position: 1
slug: 'framework-presentation-http-server'
parent: 'framework-presentation'
navTitle: 'HTTP Server'
title: 'HTTP Server'
description: 'Routing, SPA fallback, Vite proxy, response helpery, HTTPError interface.'
---

# HTTP Server

## Proč

Server je jediný vstupní bod pro HTTP provoz. Centralizuje routing, middleware chain a response format na jednom místě, takže handlery a doména zůstávají čisté.

## Jak

### Routing

Server používá stdlib `net/http.ServeMux` s Go 1.22+ pattern routingem. Routy se registrují v `presentation/http/server/`.

**Veřejné:**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/login` | Přihlášení (nickname + heslo) |
| POST | `/api/v1/auth/refresh` | Obnovení tokenu |
| GET | `/{path...}` | SPA fallback |

**Chráněné (JWT Bearer + `AuthMiddleware` wrap):**

| Metoda | Route | Command/Query | Permission |
|---|---|---|---|
| POST | `/api/v1/auth/logout` | `LogoutCommand` | `auth:logout` |
| GET | `/api/v1/profile` | `GetProfileQuery` | `profile:read` |
| PUT | `/api/v1/profile/password` | `ChangePasswordCommand` | `profile:update` |

**Admin (plánováno — Fáze 7):**

| Metoda | Route | Command/Query | Permission |
|---|---|---|---|
| GET | `/api/v1/admin/users` | `ListUsersQuery` | `admin:users:read` |
| POST | `/api/v1/admin/users` | `CreateUserCommand` | `admin:users:create` |
| PUT | `/api/v1/admin/users/{id}` | `UpdateUserCommand` | `admin:users:update` |
| DELETE | `/api/v1/admin/users/{id}` | `DeleteUserCommand` | `admin:users:delete` |

### SPA fallback

Catch-all `GET /{path...}` vrací soubory z embedovaného `public.FS`. Neexistující cesty vrátí `index.html` -- o routing rozhoduje Vue Router na klientu.

### Vite dev proxy

Při vývoji frontend běží na Vite dev serveru (`yarn dev`). Proxy směruje API cesty, health check a favicon na Go backend:

```typescript
// vite.config.ts
// Port se čte z APP_HTTP_PORT v .env (default: 3000)
server: {
    proxy: {
        '^/(api|health|favicon\\.ico)': {
            target: `http://localhost:${backendPort}`,
            changeOrigin: true,
        },
    },
}
```

### Response helpery

Balíček `presentation/http/response/` poskytuje tři funkce pro jednotný JSON output:

```go
// presentation/http/response/response.go

func JSON(w http.ResponseWriter, status int, data any)
func Error(w http.ResponseWriter, status int, err error)
func HandleError(w http.ResponseWriter, err error)
```

- `JSON()` -- serializuje `data` do JSON a nastaví `Content-Type` + status code.
- `Error()` -- zapíše chybovou odpověď. Pokud error implementuje `FieldError` (např. `*shared.ValidationError` s vyplněným polem), použije jeho název jako klíč v JSON; jinak "general".
- `HandleError()` -- automaticky mapuje error na správný HTTP status + volá `Error()`.

**Error response shape** — key-based, každý error má jeden klíč:

```json
// ValidationError{Field: "nickname", Message: "..."}
{ "nickname": "nickname je povinný" }

// AuthError / PermissionError / systémové chyby
{ "general": "invalid credentials" }
```

Frontend definuje vlastní typ a přiřazuje celé tělo přímo do reactive errors:

```typescript
type LoginErrors = { general?: string; nickname?: string; password?: string };
const errors = ref<LoginErrors>({});

// on failure:
errors.value = result.data;  // server key (general / nickname / …) mapuje na formulář
```

Detaily viz [Errors & Events](/framework/domain/errors-events) a [Frontend Utils](/guides/frontend-utils).

### HTTPError interface

```go
type HTTPError interface {
    error
    HTTPStatus() int
}
```

`HandleError` kontroluje, zda error implementuje `HTTPError`:

- **Ano** -- použije `HTTPStatus()` (např. 400, 401, 403).
- **Ne** -- vrátí 500 Internal Server Error.

## Detaily

- Domain error typy (`ValidationError` 400, `AuthError` 401, `PermissionError` 403) implementují `HTTPError` implicitně (duck typing). Žádný import mezi `response/` a `domain/`. Detaily viz [Errors & Events](/framework/domain/errors-events).
- Server struct drží `*http.ServeMux`, middleware chain a `Start()` metodu, která spustí `http.ListenAndServe`.
- Response balíček nemá žádné závislosti kromě stdlib.
