---
layout: 'page'
uri: '/framework/presentation/http-handlers'
position: 2
slug: 'framework-presentation-http-handlers'
parent: 'framework-presentation'
navTitle: 'Handlers & Middleware'
title: 'Handlers & Middleware'
description: 'HTTP handlery s bus dispatchem a middleware chain (trace, security headers, CORS, CSRF, logging, JWT).'
---

# Handlers & Middleware

## Proč

Handlery jsou tenká vrstva mezi HTTP a doménou -- deserializují vstup, zavolají command/query přes bus a vrátí odpověď. Middleware chain řeší průřezy (trace, auth, CORS) mimo handlery, takže každý handler zůstává jednoduchý.

## Jak

### Handler pattern

Handler přijme request, dekóduje JSON body a dispatchne přes bus. Neimportuje `infrastructure/` -- autorizace a další průřezy probíhá v bus middleware.

**Konvence pojmenování:** struct odpovídá zdroji/oblasti (`AdminUsersHandler`, `AuthHandler`, `HealthHandler`), metoda odpovídá akci (`Create`, `List`, `Login`, `Check`). Žádný `Handle*` prefix -- struct už říká, že jde o handler, metoda nese význam (akci).

```go
// presentation/http/handler/admin_users.go

type AdminUsersHandler struct {
    commandBus *bus.CommandBus
    queryBus   *bus.QueryBus
    createUser *command.CreateUserHandler
    listUsers  *query.ListUsersHandler
}

func (h *AdminUsersHandler) Create(w http.ResponseWriter, r *http.Request) {
    var cmd command.CreateUserCommand
    if err := request.DecodeJSON(w, r, &cmd); err != nil {
        response.Error(w, http.StatusBadRequest, err)
        return
    }

    err := bus.ExecVoid(r.Context(), h.commandBus.Bus, "CreateUser", cmd, func(ctx context.Context) error {
        return h.createUser.Handle(ctx, cmd)
    })
    if err != nil {
        response.HandleError(w, err)
        return
    }
    response.JSON(w, http.StatusCreated, nil)
}

func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
    q := query.ListUsersQuery{}
    users, err := bus.Exec[[]user.User](r.Context(), h.queryBus.Bus, "ListUsers", q, func(ctx context.Context) ([]user.User, error) {
        return h.listUsers.Handle(ctx, q)
    })
    if err != nil {
        response.HandleError(w, err)
        return
    }
    response.JSON(w, http.StatusOK, users)
}
```

Registrace rout čitelně odráží akci:

```go
mux.HandleFunc("POST /api/v1/admin/users", adminUsers.Create)
mux.HandleFunc("GET /api/v1/admin/users", adminUsers.List)
mux.HandleFunc("POST /api/v1/auth/login", auth.Login)
mux.HandleFunc("GET /health", health.Check)
```

- **Command (bez výsledku):** `bus.ExecVoid()` -- použít pro create, update, delete.
- **Query (s výsledkem):** `bus.Exec[R]()` -- typovaný generický návrat.

Chyby z bus dispatche jsou centralizované přes `response.HandleError(w, err)` -- ten mapuje doménové typy na HTTP status (handler se o mapování nestará). Výjimkou je dekódování vstupu: selhání `request.DecodeJSON` handler mapuje explicitně přes `response.Error(w, http.StatusBadRequest, err)`, protože "špatný JSON" je vždy 400 a nemá procházet doménovým mapováním. Viz [Error typy](/framework/domain/errors-events).

### Middleware chain

Balíček `presentation/http/middleware/`. Každý middleware je `func(http.Handler) http.Handler`.

| Middleware | Soubor | Popis |
|---|---|---|
| Trace | `trace.go` | Generování/propagace X-Trace-Id |
| IP | `ip.go` | Resoluce klientské IP do contextu (sdíleno s rate limitem a auditem) |
| Security headers | `security.go` | HSTS (gateováno na `APP_COOKIE_SECURE`), CSP, X-Frame-Options, Permissions-Policy a další |
| CORS | `cors.go` | Povolení cross-origin (Vite dev) |
| CSRF | -- | `http.CrossOriginProtection` (Go 1.25 stdlib) |
| Logging | `logging.go` | Request/response logging |
| JWT Auth | `auth.go` | Validace Bearer tokenu, claims do contextu |

Pořadí chain podle typu routy:

```
Request
  /health, /api/v1/auth/{login,refresh}
      -> Trace -> IP -> Recovery -> Security headers -> CORS -> CSRF -> Logging -> Handler

  /api/v1/... (chráněné)
      -> Trace -> IP -> Recovery -> Security headers -> CORS -> CSRF -> Logging -> JWT Auth -> Handler

  /{path...} (SPA)
      -> Trace -> IP -> Recovery -> Security headers -> CORS -> CSRF -> Logging -> SPA Fallback
```

SPA catch-all (`GET /{path...}`) je registrovaný do **téhož** muxu, který obaluje globální chain -- **neobchází** middleware (žádný static-file bypass). Jen běží jako poslední route, takže explicitní cesty vyhrávají.

Oddělení admin / uživatel routes řeší bus `AuthorizeMiddleware` skrze `Permissioned.RequiredPermission()` -- žádný role-guard HTTP middleware není potřeba, protože pravidlo "admin má všechno, ostatní role jsou zamítnuti pro `admin:*`" platí pro každý command i query.

### Trace middleware

Generuje unikátní trace ID pro každý request a ukládá ho do contextu:

```go
ctx = shared.ContextWithTraceID(r.Context(), traceID)
```

Trace ID je dostupný ve všech dalších vrstvách přes `shared.TraceIDFromContext(ctx)`.

### JWT Auth middleware

Extrahuje Bearer token z `Authorization` hlavičky, validuje přes `security.JwtService` a uloží claims do contextu:

```go
claims, err := jwtService.ValidateAccessToken(token)
ctx = shared.ContextWithClaims(r.Context(), claims)
```

Při nevalidním tokenu vrací `401 Unauthorized`.

## Detaily

- Handlery nikdy neimportují infrastructure balíčky. Všechna business logika (validace, autorizace, persistence) se děje v bus middleware a application layer.
- CSRF ochrana používá `http.CrossOriginProtection` ze stdlib Go 1.25 -- není třeba externí knihovna.
- Context propagace: trace ID i auth claims putuje celým request lifecycle od middleware až po repository vrstvu.
