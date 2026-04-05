---
layout: 'page'
uri: '/framework/presentation/http-middleware'
position: 3
slug: 'framework-presentation-http-middleware'
parent: 'framework-presentation'
navTitle: 'HTTP Middleware'
title: 'HTTP Middleware'
description: 'Balíček presentation/http/middleware/ – CORS, CSRF, logging, JWT auth, role guard.'
---

# HTTP Middleware

Balíček `presentation/http/middleware/`. HTTP middleware chain před handlery.

| Middleware | Soubor | Popis |
|---|---|---|
| Trace | `trace.go` | Generování/propagace X-Trace-Id |
| CORS | `cors.go` | Povolení cross-origin (Vite dev) |
| CSRF | — | `http.CrossOriginProtection` (Go 1.25, stdlib) |
| Logging | `logging.go` | Request/response logging |
| JWT Auth | `auth.go` | Validace Bearer tokenu, claims do contextu |
| Role Guard | `role.go` | Kontrola role (admin/user) |


## Middleware chain

```
Request
  ├─ /health, /api/v1/auth/login → Trace → CORS → CSRF → Logging → Handler
  ├─ /api/v1/... (chráněné)      → Trace → CORS → CSRF → Logging → JWT Auth → Handler
  ├─ /api/v1/admin/...            → Trace → CORS → CSRF → Logging → JWT Auth → Role Guard → Handler
  └─ /{path...} (SPA)            → Static File / SPA Fallback
```


## Trace middleware

Generuje unikátní trace ID pro každý request a ukládá ho do contextu přes `shared.ContextWithTraceID()`. Trace ID je dostupný ve všech dalších vrstvách (bus middleware, handlery) přes `shared.TraceIDFromContext()`.


## JWT Auth middleware

Extrahuje Bearer token z `Authorization` hlavičky, validuje přes `security.JwtService`, uloží `shared.AuthClaims` do contextu:

```go
claims, err := jwtService.ValidateAccessToken(token)
ctx = shared.ContextWithClaims(r.Context(), claims)
```

Při nevalidním tokenu vrací `401 Unauthorized`.
