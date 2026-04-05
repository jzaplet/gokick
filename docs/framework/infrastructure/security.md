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
// infrastructure/security/security_jwt.go

type JwtService struct { /* secret, accessExpiration, refreshExpiration */ }

func NewJwtService(cfg *config.Config) (*JwtService, error)

func (s *JwtService) GenerateAccessToken(claims *shared.AuthClaims) (string, time.Duration, error)
func (s *JwtService) ValidateAccessToken(tokenString string) (*shared.AuthClaims, error)
func (s *JwtService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
```

- **Access token**: HS256-signed JWT, obsahuje `sub` (UserID), `role`, `nickname`. Vrací podepsaný string a dobu platnosti.
- **Refresh token**: `crypto/rand.Text()` (Go 1.24+) generuje náhodný raw token. Do DB se ukládá SHA-256 hash, klientovi se posílá raw hodnota.
- `ValidateAccessToken` vrací `*shared.AuthClaims` nebo `*shared.AuthError`.

### PasswordHasher

```go
// infrastructure/security/security_password.go

type PasswordHasher struct{}

func NewPasswordHasher() *PasswordHasher

func (h *PasswordHasher) Hash(password string) (string, error)
func (h *PasswordHasher) Verify(password, hash string) error
```

Implementuje `shared.PasswordHasher`. Před bcrypt (cost 12) provádí **SHA-256 prehash** -- bcrypt ořízne vstup na 72 bytů, prehash zajistí, že se vždy uvažuje celé heslo bez ohledu na délku.

### PermissionChecker

```go
// infrastructure/security/security_permission.go

type PermissionChecker struct{}

func NewPermissionChecker() *PermissionChecker

func (c *PermissionChecker) Check(ctx context.Context, permission string) error
```

Implementuje `shared.PermissionChecker`. Logika:

1. Pokud v contextu nejsou `AuthClaims` -- vrátí `AuthError` ("authentication required").
2. Role `admin` -- plný přístup, všechny permissions povoleny.
3. Role `user` -- permissions s prefixem `admin:` jsou zamítnuty.

Používá ho `AuthorizeMiddleware` v busu.

## Detaily

- `JwtService` vyžaduje `APP_JWT_SECRET` -- `NewJwtService` vrací error pokud chybí.
- Refresh token hash funkce `HashToken(raw)` je exportovaná -- používá se i při validaci refresh tokenu.
- Wire binduje `PasswordHasher` a `PermissionChecker` na doménové interfaces přes provider funkce v `container_provider.go`:
  ```go
  func providePasswordHasher() shared.PasswordHasher { return security.NewPasswordHasher() }
  func providePermissionChecker() shared.PermissionChecker { return security.NewPermissionChecker() }
  ```
