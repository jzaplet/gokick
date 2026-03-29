---
layout: 'page'
uri: '/framework/adapters/http-middleware'
position: 3
slug: 'framework-adapters-http-middleware'
parent: 'framework-adapters'
navTitle: 'HTTP Middleware'
title: 'HTTP Middleware'
description: 'Balíček middleware/ – CORS, CSRF, logging, JWT auth, role guard.'
---

# HTTP Middleware

Balíček `middleware/`. HTTP middleware chain před handlery.

| Middleware | Soubor | Popis |
|---|---|---|
| CORS | `cors.go` | Povolení cross-origin (Vite dev) |
| CSRF | — | `http.CrossOriginProtection` (Go 1.25, stdlib) |
| Logging | `logging.go` | Request/response logging |
| JWT Auth | `auth.go` | Validace Bearer tokenu, claims do contextu |
| Role Guard | `role.go` | Kontrola role (admin/user) |


## Middleware chain

```
Request
  ├─ /health, /api/v1/auth/login → CORS → Logging → Handler
  ├─ /api/v1/... (chráněné)      → CORS → Logging → JWT Auth → Handler
  ├─ /api/v1/admin/...            → CORS → Logging → JWT Auth → Role Guard → Handler
  └─ /{path...} (SPA)            → Static File / SPA Fallback
```


## JWT Auth middleware

Extrahuje Bearer token z `Authorization` hlavičky, validuje přes `security.JwtService`, uloží `domain.AuthClaims` do contextu:

```go
claims, err := jwtService.ValidateAccessToken(token)
ctx = domain.ContextWithClaims(r.Context(), claims)
```

Při nevalidním tokenu vrací `401 Unauthorized`.
