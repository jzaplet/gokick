---
layout: 'page'
uri: '/framework/domain/interfaces'
position: 2
slug: 'framework-domain-interfaces'
parent: 'framework-domain'
navTitle: 'Interfaces'
title: 'Interfaces'
description: 'Repository a service interfaces -- user.Repository, PasswordHasher, PermissionChecker, Transactor.'
---

# Interfaces


## Proč

Domain definuje _co_ systém umí, ne _jak_. Interfaces žijí v doméně, implementace v infrastructure (`sqlite/user/`, `sqlite/token/`, `security/`). Propojení přes Wire DI. Díky tomu domain nemá žádné závislosti na infrastruktuře.


## Jak

### Repository interfaces

#### user.Repository

Žije v `domain/user/user_repository.go`:

```go
package user

type Repository interface {
    Save(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*User, error)
    FindByNickname(ctx context.Context, nickname string) (*User, error)
    FindAllActive(ctx context.Context) ([]User, error)
    FindAll(ctx context.Context) ([]User, error)
}
```

Implementace: `infrastructure/sqlite/user/user_repository.go`


#### token.TokenRepository

Žije v `domain/token/token_repository.go`:

```go
package token

type TokenRepository interface {
    Save(ctx context.Context, token *RefreshToken) error
    FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
    DeleteByUserID(ctx context.Context, userID string) error
    DeleteExpired(ctx context.Context) error
}
```

Implementace: `infrastructure/sqlite/token/token_repository.go`


### Service interfaces

#### PasswordHasher

Žije v `domain/shared/password.go`:

```go
package shared

type PasswordHasher interface {
    Hash(password string) (string, error)
    Verify(password, hash string) error
}
```

Implementace: `infrastructure/security/security_password.go` (`security.PasswordHasher`). Používá SHA-256 prehash před bcrypt -- bcrypt truncuje vstup na 72 bytes, prehash zajistí že celý password je vždy zohledněn.


#### PermissionChecker

Žije v `domain/shared/permission.go`:

```go
package shared

type Permissioned interface {
    RequiredPermission() string
}

type SkipPermission interface {
    SkipPermissionCheck()
}

type PermissionChecker interface {
    Check(ctx context.Context, permission string) error
}
```

Každý command/query MUSÍ implementovat buď `Permissioned`, nebo `SkipPermission`. Bus `AuthorizeMiddleware` to vynucuje -- pokud command neimplementuje ani jeden, vrátí error.

Implementace checkeru: `infrastructure/security/security_permission.go`


#### Transactor

Žije v `domain/shared/transactor.go`:

```go
package shared

type Transactor interface {
    BeginTx(ctx context.Context) (context.Context, error)
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
}
```

Používá se v bus `TransactionMiddleware`. Implementace: `infrastructure/database/` (SqliteManager).


### AuthClaims context helpers

Žijí v `domain/shared/auth_context.go`. Identita uživatele je doménový koncept -- `AuthClaims` žijí v doméně, ne v security.

```go
package shared

type AuthClaims struct {
    UserID   string
    Role     string
    Nickname string
}

func ClaimsFromContext(ctx context.Context) *AuthClaims
func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context
```

JWT middleware v presentation vrstvě dekóduje token a vloží `AuthClaims` do contextu přes `ContextWithClaims`. Command/query handlery a bus middleware čtou claims přes `ClaimsFromContext`.

Stejný pattern používá `TraceID` (`domain/shared/trace_context.go`):

```go
func TraceIDFromContext(ctx context.Context) string
func ContextWithTraceID(ctx context.Context, traceID string) context.Context
```


## Detaily

- Všechny interfaces používají `context.Context` jako první parametr -- umožňuje předávat transakci, claims, trace ID.
- Repository interfaces vrací pointery na entity (`*User`, `*RefreshToken`), ne hodnoty.
- `FindByNickname` vrací `nil, nil` když uživatel neexistuje (ne error) -- umožňuje kontrolu existence bez error handlingu.
- `FindByID` vrací `*shared.ValidationError` když uživatel neexistuje -- protože volání s neexistujícím ID je vstupní chyba.
- Seeder v `infrastructure/sqlite/seeder.go` závisí na `user.Repository` (domain interface), ne na konkrétní implementaci.
