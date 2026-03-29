---
layout: 'page'
uri: '/framework/domain/interfaces'
position: 3
slug: 'framework-domain-interfaces'
parent: 'framework-domain'
navTitle: 'Interfaces'
title: 'Interfaces'
description: 'Repository a service interfaces – UserRepository, PasswordHasher, PermissionChecker.'
---

# Interfaces

Domain definuje _co_ systém umí. Implementace žijí v adapter vrstvě (`sqlite/`, `security/`), propojení přes Wire DI.


## Repository interfaces

```go
// domain/user.go

type UserRepository interface {
    Save(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*User, error)
    FindByNickname(ctx context.Context, nickname string) (*User, error)
    FindAllActive(ctx context.Context) ([]User, error)
    FindAll(ctx context.Context) ([]User, error)
}
```

```go
// domain/auth.go

type TokenRepository interface {
    Save(ctx context.Context, token *RefreshToken) error
    FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
    DeleteByUserID(ctx context.Context, userID string) error
    DeleteExpired(ctx context.Context) error
}
```


## Service interfaces

```go
// domain/password.go

type PasswordHasher interface {
    Hash(password string) (string, error)
    Verify(password, hash string) error
}
```

Implementace: `security.PasswordService` (bcrypt).


## Permission interfaces

```go
// domain/permission.go

// Command/Query deklaruje požadovanou permission.
type Permissioned interface {
    RequiredPermission() string
}

// Command/Query explicitně přeskakuje permission check.
type SkipPermission interface {
    SkipPermissionCheck()
}

// Ověří, zda aktuální uživatel má permission.
type PermissionChecker interface {
    Check(ctx context.Context, permission string) error
}
```

Každý command/query MUSÍ implementovat buď `Permissioned`, nebo `SkipPermission`. Bus `AuthorizeMiddleware` to vynucuje – pokud command neimplementuje ani jeden, vrátí error.

Implementace checkeru: `security.PermissionChecker`. Použití: `bus.AuthorizeMiddleware`.


## Auth context

```go
// domain/auth_context.go

type AuthClaims struct {
    UserID   string
    Role     string
    Nickname string
}

func ClaimsFromContext(ctx context.Context) *AuthClaims
func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context
```

Identita uživatele je doménový koncept – `AuthClaims` žijí v doméně, ne v security.
