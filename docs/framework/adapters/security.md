---
layout: 'page'
uri: '/framework/adapters/security'
position: 5
slug: 'framework-adapters-security'
parent: 'framework-adapters'
navTitle: 'Security'
title: 'Security'
description: 'Balíček security/ – JwtService, PasswordService, PermissionChecker.'
---

# Security

Balíček `security/`. Implementuje doménové interfaces.


## JwtService

```go
// security/jwt_service.go

type JwtService struct { ... }
func (s *JwtService) GenerateAccessToken(user *domain.User) (string, int, error)
func (s *JwtService) ValidateAccessToken(tokenString string) (*domain.AuthClaims, error)
func (s *JwtService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
```

Refresh token interně generován přes `crypto/rand.Text()` (Go 1.24+).


## PasswordService

Implementuje `domain.PasswordHasher`:

```go
// security/password_service.go

type PasswordService struct{}
func (s *PasswordService) Hash(password string) (string, error)     // bcrypt
func (s *PasswordService) Verify(password, hash string) error       // bcrypt compare
```


## PermissionChecker

Implementuje `domain.PermissionChecker`:

```go
// security/permission_checker.go

type PermissionChecker struct{}
func (c *PermissionChecker) Check(ctx context.Context, permission string) error {
    claims := domain.ClaimsFromContext(ctx)
    if claims == nil {
        return &domain.AuthError{Message: "nepřihlášen"}
    }
    // Ověření role/permission proti claims
}
```
