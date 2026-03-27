---
layout: 'page'
uri: '/framework/adapters'
position: 6
slug: 'framework-adapters'
parent: 'framework'
navTitle: 'Adaptery'
title: 'Adaptery'
description: 'Adapter vrstva – HTTP server, handlery, middleware, SQLite repozitáře, security služby.'
---

# Adaptery

Implementace doménových interfaces a HTTP vrstva. Závisí na domain a application vrstvě.


## HTTP server

`net/http` stdlib s Go 1.22+ routing (method patterns). Balíček `server/` registruje handlery a middleware na `http.ServeMux`. Od Go 1.26 trailing slash redirecty používají HTTP 307 (zachovává HTTP metodu).


### Routing (skeleton)

**Veřejné (bez auth):**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/api/v1/auth/users` | Seznam aktivních uživatelů (login screen) |
| POST | `/api/v1/auth/login` | Přihlášení |
| POST | `/api/v1/auth/refresh` | Obnovení access tokenu |
| GET | `/{path...}` | SPA fallback |

**Chráněné (Bearer token):**

| Metoda | Route | Popis |
|---|---|---|
| POST | `/api/v1/auth/logout` | Odhlášení |
| GET | `/api/v1/profile` | Profil přihlášeného uživatele |
| PUT | `/api/v1/profile/password` | Změna hesla |

**Admin only (Bearer + role guard):**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/api/v1/admin/users` | Seznam všech uživatelů |
| POST | `/api/v1/admin/users` | Vytvoření uživatele |
| PUT | `/api/v1/admin/users/{id}` | Editace uživatele |
| DELETE | `/api/v1/admin/users/{id}` | Smazání uživatele |


### SPA fallback

Catch-all route vrací `index.html` pro cesty, které neodpovídají API endpointům. Vue Router pak rozhodne na klientu.


### Vite dev proxy

Vite dev server (5173) proxuje API requesty na Go backend (3000):

```typescript
// vite.config.ts
server: {
    proxy: {
        '/api': 'http://localhost:3000',
        '/health': 'http://localhost:3000'
    }
}
```


## HTTP handlery

Balíček `handler/`. Každý handler přijme HTTP request, deserializuje vstup a zavolá command/query přes bus. Při chybě volá `response.HandleError(w, err)` – mapování error→status je centralizované v `response/` balíčku.

Handler nikdy neimportuje `sqlite/` – pracuje jen s domain interfaces přes CQRS handlery. Validace probíhá v command handleru přes domain value objects – HTTP handler se o ni nestará.


## HTTP middleware

Balíček `middleware/`:

| Middleware | Soubor | Popis |
|---|---|---|
| CORS | `cors.go` | Povolení cross-origin (Vite dev server) |
| CSRF | — | `http.CrossOriginProtection` (Go 1.25) – vestavěná CSRF ochrana ze stdlib |
| Logging | `logging.go` | Request/response logging |
| JWT Auth | `auth.go` | Validace Bearer tokenu, claims do contextu |
| Role Guard | `role.go` | Kontrola role (admin/user) |

```
Request
  ├─ /health, /api/v1/auth/login → CORS → Logging → Handler
  ├─ /api/v1/... (chráněné)      → CORS → Logging → JWT Auth → Handler
  ├─ /api/v1/admin/...            → CORS → Logging → JWT Auth → Role Guard → Handler
  └─ /{path...} (SPA)            → Static File / SPA Fallback
```


## SQLite repozitáře

Balíček `sqlite/`. Implementuje doménové repository interfaces:

- `user_repository.go` → `domain.UserRepository`
- `token_repository.go` → `domain.TokenRepository`

Repozitáře používají `database.TxFromContext` pro práci s transakcemi z bus middleware.


## Security služby

Balíček `security/`:

```go
// security/jwt_service.go
type JwtService struct { ... }
func (s *JwtService) GenerateAccessToken(user *domain.User) (string, int, error)
func (s *JwtService) ValidateAccessToken(tokenString string) (*AuthClaims, error)
func (s *JwtService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
// Interně používá crypto/rand.Text() pro generování náhodného tokenu (Go 1.24+).
```

```go
// security/password_service.go
type PasswordService struct{}
func (s *PasswordService) Hash(password string) (string, error)
func (s *PasswordService) Verify(password, hash string) error
```

```go
// security/auth_context.go
type AuthClaims struct { UserID, Role, Nickname string }
func ClaimsFromContext(ctx context.Context) *AuthClaims
func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context
```


## Response helpery

Balíček `response/` – JSON response a centralizovaný error handling. Žádné závislosti na domain (duck typing přes interface):

```go
// response/json.go

// HTTPError – error s HTTP statusem. Domain typy ho implementují implicitně.
type HTTPError interface {
    error
    HTTPStatus() int
}

func JSON(w http.ResponseWriter, status int, data any)
func Error(w http.ResponseWriter, status int, err error)

// HandleError rozliší error typ přes HTTPError interface.
// Implementuje ho např. domain.ValidationError (400).
// Neznámé chyby → 500.
func HandleError(w http.ResponseWriter, err error) {
    var httpErr HTTPError
    if errors.As(err, &httpErr) {
        Error(w, httpErr.HTTPStatus(), err)
    } else {
        Error(w, http.StatusInternalServerError, err)
    }
}
```

`response/` definuje `HTTPError` interface, `domain.ValidationError` ho implicitně implementuje metodou `HTTPStatus() int`. Žádný import mezi nimi – Go duck typing.
