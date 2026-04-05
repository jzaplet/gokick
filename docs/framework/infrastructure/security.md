---
layout: 'page'
uri: '/framework/infrastructure/security'
position: 4
slug: 'framework-infrastructure-security'
parent: 'framework-infrastructure'
navTitle: 'Security'
title: 'Security'
description: 'Balíček infrastructure/security/ – JwtService, PasswordService, PermissionChecker.'
---

# Security

Balíček `infrastructure/security/`. Implementuje doménové interfaces.


## JwtService

```go
// infrastructure/security/security_jwt.go

type JwtService struct { ... }
func (s *JwtService) GenerateAccessToken(user *user.User) (string, int, error)
func (s *JwtService) ValidateAccessToken(tokenString string) (*shared.AuthClaims, error)
func (s *JwtService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
```

Refresh token interně generován přes `crypto/rand.Text()` (Go 1.24+).


## PasswordService

Implementuje `shared.PasswordHasher`:

```go
// infrastructure/security/security_password.go

type PasswordService struct{}
func (s *PasswordService) Hash(password string) (string, error)     // bcrypt
func (s *PasswordService) Verify(password, hash string) error       // bcrypt compare
```


## PermissionChecker

Implementuje `shared.PermissionChecker`:

```go
// infrastructure/security/security_permission.go

type PermissionChecker struct{}
func (c *PermissionChecker) Check(ctx context.Context, permission string) error {
    claims := shared.ClaimsFromContext(ctx)
    if claims == nil {
        return &shared.AuthError{Message: "nepřihlášen"}
    }
    // Ověření role/permission proti claims
}
```
