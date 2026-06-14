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
| GET | `/api/v1/dashboard/user` | `GetUserDashboardQuery` | `dashboard:read` |

**Admin:**

| Metoda | Route | Command/Query | Permission |
|---|---|---|---|
| GET | `/api/v1/dashboard/admin` | `GetAdminDashboardQuery` | `admin:dashboard:read` |
| GET | `/api/v1/admin/users` | `ListUsersQuery` | `admin:users:read` |
| POST | `/api/v1/admin/users` | `CreateUserCommand` | `admin:users:create` |
| PUT | `/api/v1/admin/users/{id}` | `UpdateUserCommand` | `admin:users:update` |
| DELETE | `/api/v1/admin/users/{id}` | `DeleteUserCommand` | `admin:users:delete` |

Žádné samostatné role-guard middleware není potřeba -- bus `AuthorizeMiddleware` v kombinaci s `IsPermissionAllowedForRole` (`admin:*` permissions povolí jen admin role) pokrývá oddělení uživatel / admin.

### SPA fallback

Catch-all `GET /{path...}` vrací soubory z embedovaného `public.FS`. Neexistující cesty vrátí `index.html` -- o routing rozhoduje Vue Router na klientu.

Při servírování `index.html` do něj `SPAHandler` **injektuje runtime config** jako `<meta name="gokick:…">` tagy (Sentry DSN, environment, debug flag) -- jeden buildnutý image tak může běžet ve více prostředích bez rebuildu. SPA je čte přes `runtimeConfig.ts`. Detail viz [Sentry guide](/guides/sentry) + [Config → Sentry](/framework/infrastructure/config#sentry).

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
{ "nickname": "nickname is required" }

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

## Graceful shutdown

`server.Start(ctx context.Context) error` poslouchá na cancellation z ctx. `cmd/main.go` vytvoří root ctx přes `signal.NotifyContext(ctx, SIGINT, SIGTERM)` a propaguje ho přes `Application.Run` → Cobra → `ServeCommand`. Když přijde signál:

1. `ctx.Done()` se odpálí.
2. `Start` zavolá `http.Server.Shutdown(shutdownCtx)` s 30s timeoutem.
3. `Shutdown` přestane přijímat nová spojení a počká na dokončení inflight requestů.
4. Po dokončení (nebo timeoutu) se vrátí; `Run` ukončí proces s exit code 0.

Pokud handler trvá déle než 30s, `Shutdown` vrátí `context.DeadlineExceeded` a `Run` exitne s nenulovým kódem. Pro long-running endpointy (uploads, exports) je 30s konzervativní default — zvedněte `shutdownGracePeriod` v `server.go` podle potřeby.


## Detaily

- Domain error typy (`ValidationError` 400, `AuthError` 401, `PermissionError` 403) implementují `HTTPError` implicitně (duck typing). Žádný import mezi `response/` a `domain/`. Detaily viz [Errors & Events](/framework/domain/errors-events).
- Server struct drží konfiguraci, logger, JWT service, rate-limitery, IP extractor a HTTP handlery -- **ne** `*http.ServeMux` ani middleware chain. Ty se staví per-call uvnitř `Start`: `registerRoutes()` vrátí lokální `*http.ServeMux` a `buildMiddlewareChain()` ho obalí. `Start(ctx)` pak spustí `http.Server.ListenAndServe` v goroutině a čeká na `ctx.Done()` nebo server error (viz [Graceful shutdown](#graceful-shutdown)).
- **Pořadí v `buildMiddlewareChain` je invariant:** `Trace → IP → Recovery → Security → CORS → CSRF → Logging`. `Trace` a `IP` jsou čisté context-settery (jen razítkují ctx, nikdy neselžou), takže běží **před** `Recovery` schválně — každý panic report tím nese `trace_id` i klientskou IP (Sentry capture čte IP z ctx). Přehození `IP` za `Recovery` by tichounce shodilo `user.ip_address` na zachycených panikách; hlídá to regresní test proti `buildMiddlewareChain`.
- Response balíček nemá žádné závislosti kromě stdlib.
