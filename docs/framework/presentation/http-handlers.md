---
layout: 'page'
uri: '/framework/presentation/http-handlers'
position: 2
slug: 'framework-presentation-http-handlers'
parent: 'framework-presentation'
navTitle: 'Handlers & Middleware'
title: 'Handlers & Middleware'
description: 'HTTP handlery s bus dispatchem a middleware chain (trace, CORS, CSRF, logging, JWT, role guard).'
---

# Handlers & Middleware

## Proč

Handlery jsou tenká vrstva mezi HTTP a doménou -- deserializují vstup, zavolají command/query přes bus a vrátí odpověď. Middleware chain řeší průřezy (trace, auth, CORS) mimo handlery, takže každý handler zůstává jednoduchý.

## Jak

### Handler pattern

Handler přijme request, dekóduje JSON body a dispatchne přes bus. Neimportuje `infrastructure/` -- autorizace a další průřezy probíhá v bus middleware.

```go
// presentation/http/handler/admin_users.go

type AdminUsersHandler struct {
    commandBus *bus.CommandBus
    queryBus   *bus.QueryBus
    createUser *command.CreateUserHandler
    listUsers  *query.ListUsersHandler
}

func (h *AdminUsersHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
    var cmd command.CreateUserCommand
    if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
        response.HandleError(w, err)
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

func (h *AdminUsersHandler) HandleList(w http.ResponseWriter, r *http.Request) {
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

- **Command (bez výsledku):** `bus.ExecVoid()` -- použít pro create, update, delete.
- **Query (s výsledkem):** `bus.Exec[R]()` -- typovaný generický návrat.

Error handling je centralizovaný přes `response.HandleError(w, err)` -- handler se nestará o mapování error na HTTP status. Viz [Error typy](/framework/domain/errors-events).

### Middleware chain

Balíček `presentation/http/middleware/`. Každý middleware je `func(http.Handler) http.Handler`.

| Middleware | Soubor | Popis |
|---|---|---|
| Trace | `trace.go` | Generování/propagace X-Trace-Id |
| CORS | `cors.go` | Povolení cross-origin (Vite dev) |
| CSRF | -- | `http.CrossOriginProtection` (Go 1.25 stdlib) |
| Logging | `logging.go` | Request/response logging |
| JWT Auth | `auth.go` | Validace Bearer tokenu, claims do contextu |
| Role Guard | `role.go` | Kontrola role (admin/user) |

Pořadí chain podle typu routy:

```
Request
  /health, /api/v1/auth/{login,refresh}
      -> Trace -> CORS -> CSRF -> Logging -> Handler

  /api/v1/... (chráněné)
      -> Trace -> CORS -> CSRF -> Logging -> JWT Auth -> Handler

  /api/v1/admin/...
      -> Trace -> CORS -> CSRF -> Logging -> JWT Auth -> Role Guard -> Handler

  /{path...} (SPA)
      -> Static File / SPA Fallback
```

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
