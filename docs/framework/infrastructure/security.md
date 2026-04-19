---
layout: 'page'
uri: '/framework/infrastructure/security'
position: 3
slug: 'framework-infrastructure-security'
parent: 'framework-infrastructure'
navTitle: 'Security'
title: 'Security'
description: 'Balíček infrastructure/security/ -- JwtService, PasswordHasher, PermissionChecker.'
---

# Security

## Proč

Bezpečnostní vrstva implementuje tři služby: JWT tokeny pro autentizaci, hashování hesel, a kontrolu oprávnění. Všechny jsou bindované na doménové interfaces v `shared/` -- žádný jiný balíček neimportuje `security/` přímo.

## Jak

### JwtService

```go
// infrastructure/security/jwt.go

type JwtService struct { /* secret, accessExpiration, refreshExpiration */ }

func NewJwtService(cfg *config.Config) (*JwtService, error)

func (s *JwtService) GenerateAccessToken(claims *shared.AuthClaims) (string, time.Duration, error)
func (s *JwtService) ValidateAccessToken(tokenString string) (*shared.AuthClaims, error)
func (s *JwtService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
func (s *JwtService) HashRefreshToken(raw string) string
```

Implementuje `shared.JwtService` interface.


- **Access token**: HS256-signed JWT, obsahuje `sub` (UserID), `role`, `nickname`. Vrací podepsaný string a dobu platnosti.
- **Refresh token**: `crypto/rand.Text()` (Go 1.24+) generuje náhodný raw token. Do DB se ukládá SHA-256 hash, klientovi se posílá raw hodnota.
- `HashRefreshToken(raw)` zhashuje raw token — používá se při validaci příchozího tokenu z cookie (nalezení v DB přes hash).
- `ValidateAccessToken` vrací `*shared.AuthClaims` nebo `*shared.AuthError`.

### PasswordHasher

```go
// infrastructure/security/password.go

type PasswordHasher struct{}

func NewPasswordHasher() *PasswordHasher

func (h *PasswordHasher) Hash(password string) (string, error)
func (h *PasswordHasher) Verify(password, hash string) error
```

Implementuje `shared.PasswordHasher`. Před bcrypt (cost 12) provádí **SHA-256 prehash** -- bcrypt ořízne vstup na 72 bytů, prehash zajistí, že se vždy uvažuje celé heslo bez ohledu na délku.

### PermissionChecker

```go
// infrastructure/security/permission.go

type PermissionChecker struct{}

func NewPermissionChecker() *PermissionChecker

func (c *PermissionChecker) Check(ctx context.Context, permission string) error
```

Implementuje `shared.PermissionChecker`. Logika:

1. Pokud v contextu nejsou `AuthClaims` -- vrátí `AuthError` 401 ("authentication required").
2. Delegate na `shared.IsPermissionAllowedForRole(permission, role)`:
   - Role `admin` -- plný přístup, všechny permissions povoleny.
   - Ostatní role -- permissions s prefixem `admin:` jsou zamítnuty.
3. Když role nesedí -- vrátí `PermissionError` 403 ("insufficient permissions").

Používá ho `AuthorizeMiddleware` v busu. Stejný helper `IsPermissionAllowedForRole` používá i `PermissionsRegistry.ForRole` pro sestavení seznamu povolených permissions pro frontend.

## Detaily

- `JwtService` vyžaduje `APP_JWT_SECRET` -- `NewJwtService` vrací error pokud chybí.
- Wire binduje `*security.JwtService` na `shared.JwtService` interface přes `wire.Bind`. `PasswordHasher` a `PermissionChecker` se bindují přes provider funkce vracející interface typ:
  ```go
  wire.Bind(new(shared.JwtService), new(*security.JwtService))

  func providePasswordHasher() shared.PasswordHasher { return security.NewPasswordHasher() }
  func providePermissionChecker() shared.PermissionChecker { return security.NewPermissionChecker() }
  ```
